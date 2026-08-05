package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hibikipr/homeSIEM/siem-api/internal/loki"
)

func TestEventsSearch_ReturnsCompiledQueryAndEntries(t *testing.T) {
	fakeLoki := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"success","data":{"result":[
            {"stream":{"job":"siem","source":"udm-ultra"},"values":[["1700000000000000000","hello"]]}
        ]}}`))
	}))
	defer fakeLoki.Close()

	s, st := newTestServer(t)
	s.deps.Loki = loki.New(fakeLoki.URL, fakeLoki.Client())
	token := authToken(t, st, "viewer", 100)

	req := httptest.NewRequest(http.MethodGet, "/events/search?source=udm-ultra", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}

	var resp searchResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if resp.LogQL != `{job="siem",source="udm-ultra"}` {
		t.Errorf("LogQL = %q", resp.LogQL)
	}
	if resp.Count != 1 || len(resp.Entries) != 1 || resp.Entries[0].Line != "hello" {
		t.Errorf("resp = %+v", resp)
	}
}

func TestEventsSearch_IncludesVolumeBuckets(t *testing.T) {
	fakeLoki := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("query")
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(query, "count_over_time") {
			w.Write([]byte(`{"status":"success","data":{"resultType":"matrix","result":[
				{"metric":{},"values":[[1700000000,"3"],[1700000300,"7"]]}
			]}}`))
			return
		}
		w.Write([]byte(`{"status":"success","data":{"result":[]}}`))
	}))
	defer fakeLoki.Close()

	s, st := newTestServer(t)
	s.deps.Loki = loki.New(fakeLoki.URL, fakeLoki.Client())

	token := authToken(t, st, "viewer", 100)
	req := httptest.NewRequest(http.MethodGet, "/events/search?start=2023-11-14T00:00:00Z&end=2023-11-14T01:00:00Z", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	var resp searchResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if len(resp.Volume) != 2 || resp.Volume[0].Count != 3 || resp.Volume[1].Count != 7 {
		t.Fatalf("Volume = %+v", resp.Volume)
	}
}

func TestEventsSearch_SucceedsWhenVolumeQueryFails(t *testing.T) {
	fakeLoki := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("query")
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(query, "count_over_time") {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Write([]byte(`{"status":"success","data":{"result":[
			{"stream":{"job":"siem"},"values":[["1700000000000000000","hello"]]}
		]}}`))
	}))
	defer fakeLoki.Close()

	s, st := newTestServer(t)
	s.deps.Loki = loki.New(fakeLoki.URL, fakeLoki.Client())

	token := authToken(t, st, "viewer", 100)
	req := httptest.NewRequest(http.MethodGet, "/events/search", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (volume failure must not fail the whole request), body=%s", rec.Code, rec.Body.String())
	}
	var resp searchResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if resp.Count != 1 {
		t.Fatalf("Count = %d, want 1", resp.Count)
	}
	if resp.Volume == nil || len(resp.Volume) != 0 {
		t.Fatalf("Volume = %+v, want a non-nil empty slice", resp.Volume)
	}
}

func TestEventsSearch_RequiresAuth(t *testing.T) {
	s, _ := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/events/search", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestEventsTail_StreamsHubMessages(t *testing.T) {
	s, st := newTestServer(t)
	token := authToken(t, st, "viewer", 100)

	req := httptest.NewRequest(http.MethodGet, "/events/tail", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	// handleEventsTail blocks until the request context is cancelled, same
	// as Hub.ServeHTTP in Task 16 — reuse that task's pattern.
	ctx, cancel := context.WithCancel(context.Background())
	req = req.WithContext(ctx)

	done := make(chan struct{})
	go func() {
		s.Handler().ServeHTTP(rec, req)
		close(done)
	}()

	// Wait for the subscriber to connect with a reasonable timeout
	for i := 0; i < 100; i++ {
		if s.deps.Hub.SubscriberCount("tail") > 0 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if s.deps.Hub.SubscriberCount("tail") == 0 {
		t.Fatal("subscriber did not connect")
	}
	s.deps.Hub.Publish("tail", []byte(`{"line":"hi"}`))
	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("handler did not return after context cancellation")
	}

	if !strings.Contains(rec.Body.String(), `"line":"hi"`) {
		t.Errorf("body = %q, want it to contain the published message", rec.Body.String())
	}
}
