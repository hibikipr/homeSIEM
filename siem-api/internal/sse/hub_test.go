package sse

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSubscribeAndPublish(t *testing.T) {
	h := NewHub()
	ch, cancel := h.Subscribe("alerts")
	defer cancel()

	h.Publish("alerts", []byte(`{"id":1}`))

	select {
	case msg := <-ch:
		if string(msg) != `{"id":1}` {
			t.Errorf("msg = %q", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for published message")
	}
}

func TestPublish_DifferentTopicNotReceived(t *testing.T) {
	h := NewHub()
	ch, cancel := h.Subscribe("alerts")
	defer cancel()

	h.Publish("tail", []byte("irrelevant"))

	select {
	case msg := <-ch:
		t.Fatalf("received message on wrong topic: %q", msg)
	case <-time.After(50 * time.Millisecond):
		// expected: nothing arrives
	}
}

func TestCancel_RemovesSubscriber(t *testing.T) {
	h := NewHub()
	_, cancel := h.Subscribe("alerts")
	if h.subscriberCount("alerts") != 1 {
		t.Fatalf("subscriberCount = %d, want 1", h.subscriberCount("alerts"))
	}
	cancel()
	if h.subscriberCount("alerts") != 0 {
		t.Fatalf("subscriberCount = %d, want 0 after cancel", h.subscriberCount("alerts"))
	}
}

func TestPublish_SlowSubscriberDoesNotBlock(t *testing.T) {
	h := NewHub()
	_, cancel := h.Subscribe("alerts") // never drained
	defer cancel()

	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			h.Publish("alerts", []byte("x"))
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Publish() blocked on a full subscriber channel")
	}
}

func TestServeHTTP_StreamsPublishedMessages(t *testing.T) {
	h := NewHub()
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/events/tail", nil).WithContext(ctx)
	rec := httptest.NewRecorder()

	handlerDone := make(chan struct{})
	go func() {
		h.ServeHTTP("tail", rec, req)
		close(handlerDone)
	}()

	// Give ServeHTTP a moment to subscribe before publishing.
	for h.subscriberCount("tail") == 0 {
		time.Sleep(time.Millisecond)
	}
	h.Publish("tail", []byte("hello"))
	time.Sleep(20 * time.Millisecond) // let the write land before we cancel

	cancel()
	select {
	case <-handlerDone:
	case <-time.After(time.Second):
		t.Fatal("ServeHTTP did not return after context cancellation")
	}

	if !strings.Contains(rec.Body.String(), "data: hello") {
		t.Errorf("body = %q, want it to contain %q", rec.Body.String(), "data: hello")
	}
}

func TestServeHTTP_SendsHeartbeatWhenIdle(t *testing.T) {
	original := heartbeatInterval
	heartbeatInterval = 10 * time.Millisecond
	defer func() { heartbeatInterval = original }()

	h := NewHub()
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/events/tail", nil).WithContext(ctx)
	rec := httptest.NewRecorder()

	handlerDone := make(chan struct{})
	go func() {
		h.ServeHTTP("tail", rec, req)
		close(handlerDone)
	}()

	// No publish at all - an idle stream with no heartbeat would sit
	// silent forever and never write another byte.
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-handlerDone:
	case <-time.After(time.Second):
		t.Fatal("ServeHTTP did not return after context cancellation")
	}

	if !strings.Contains(rec.Body.String(), ": heartbeat") {
		t.Errorf("body = %q, want it to contain a heartbeat comment", rec.Body.String())
	}
}
