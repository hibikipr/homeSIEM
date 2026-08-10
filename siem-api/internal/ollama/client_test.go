package ollama

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestChat_SendsExpectedRequest(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody chatRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(chatResponse{
			Message: chatMessage{Role: "assistant", Content: "the response"},
			Done:    true,
		})
	}))
	defer srv.Close()

	c := New(srv.URL, "qwen3:27b", srv.Client())
	opts := ChatOptions{Temperature: 0.2, TopP: 0.9, NumPredict: 1024, NumCtx: 8192}
	got, err := c.Chat(context.Background(), "system prompt", "user prompt", opts)
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}

	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/api/chat" {
		t.Errorf("path = %q, want /api/chat", gotPath)
	}
	if gotBody.Model != "qwen3:27b" {
		t.Errorf("model = %q, want qwen3:27b", gotBody.Model)
	}
	if gotBody.Stream {
		t.Error("stream = true, want false")
	}
	if len(gotBody.Messages) != 2 {
		t.Fatalf("messages = %d, want 2", len(gotBody.Messages))
	}
	if gotBody.Messages[0].Role != "system" || gotBody.Messages[0].Content != "system prompt" {
		t.Errorf("messages[0] = %+v, want role=system content=%q", gotBody.Messages[0], "system prompt")
	}
	if gotBody.Messages[1].Role != "user" || gotBody.Messages[1].Content != "user prompt" {
		t.Errorf("messages[1] = %+v, want role=user content=%q", gotBody.Messages[1], "user prompt")
	}
	if got != "the response" {
		t.Errorf("Chat() = %q, want %q", got, "the response")
	}
	if gotBody.Options != opts {
		t.Errorf("options = %+v, want %+v", gotBody.Options, opts)
	}
}

func TestChat_NonOKStatus_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := New(srv.URL, "qwen3:27b", srv.Client())
	if _, err := c.Chat(context.Background(), "s", "u", ChatOptions{}); err == nil {
		t.Fatal("Chat() error = nil, want error for 500 response")
	}
}

func TestChat_MalformedResponseBody_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("not json"))
	}))
	defer srv.Close()

	c := New(srv.URL, "qwen3:27b", srv.Client())
	if _, err := c.Chat(context.Background(), "s", "u", ChatOptions{}); err == nil {
		t.Fatal("Chat() error = nil, want error for malformed response body")
	}
}

func TestChat_RequestTimesOut_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := New(srv.URL, "qwen3:27b", &http.Client{Timeout: 20 * time.Millisecond})
	if _, err := c.Chat(context.Background(), "s", "u", ChatOptions{}); err == nil {
		t.Fatal("Chat() error = nil, want a timeout error")
	}
}
