package sse

import (
	"fmt"
	"net/http"
	"sync"
)

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

func (h *Hub) subscriberCount(topic string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.subs[topic])
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
	flusher.Flush()

	ch, cancel := h.Subscribe(topic)
	defer cancel()

	for {
		select {
		case <-r.Context().Done():
			return
		case data := <-ch:
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}
