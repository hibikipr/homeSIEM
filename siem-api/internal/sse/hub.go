package sse

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// streamPaddingBytes works around WebKit/Safari content-sniffing the start
// of a text/event-stream response and withholding it from EventSource
// until enough bytes have arrived, independent of flushing. It's an SSE
// comment (never dispatched to onmessage) so other clients ignore it.
const streamPaddingBytes = 2048

// heartbeatInterval is a var, not a const, so tests can shrink it rather
// than waiting out a real 15s idle period.
var heartbeatInterval = 15 * time.Second

type Hub struct {
	mu   sync.Mutex
	subs map[string]map[chan []byte]struct{}
}

func NewHub() *Hub {
	return &Hub{subs: make(map[string]map[chan []byte]struct{})}
}

func (h *Hub) Subscribe(topic string) (chan []byte, func()) {
	ch := make(chan []byte, 16)

	h.mu.Lock()
	if h.subs[topic] == nil {
		h.subs[topic] = make(map[chan []byte]struct{})
	}
	h.subs[topic][ch] = struct{}{}
	h.mu.Unlock()

	cancel := func() {
		h.mu.Lock()
		delete(h.subs[topic], ch)
		h.mu.Unlock()
	}
	return ch, cancel
}

func (h *Hub) Publish(topic string, data []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()

	for ch := range h.subs[topic] {
		select {
		case ch <- data:
		default:
			// Slow consumer: drop rather than block every other publish.
		}
	}
}

func (h *Hub) SubscriberCount(topic string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.subs[topic])
}

// subscriberCount is deprecated; use SubscriberCount instead.
func (h *Hub) subscriberCount(topic string) int {
	return h.SubscriberCount(topic)
}

func (h *Hub) ServeHTTP(topic string, w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, ": %s\n\n", strings.Repeat(" ", streamPaddingBytes))
	flusher.Flush()

	ch, cancel := h.Subscribe(topic)
	defer cancel()

	// Without this, a stream that goes quiet (normal for e.g. an idle SIEM
	// tail) never writes another byte, and any idle timeout downstream -
	// reverse proxy, NAT conntrack, the browser's own fetch - can silently
	// kill the connection. The client then has no way to distinguish "quiet
	// but alive" from "silently dead" and, in practice, doesn't reconnect
	// until something else prompts a full page reload.
	heartbeat := time.NewTicker(heartbeatInterval)
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case data := <-ch:
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		case <-heartbeat.C:
			fmt.Fprint(w, ": heartbeat\n\n")
			flusher.Flush()
		}
	}
}
