package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hibikipr/homeSIEM/siem-api/internal/vector"
)

func TestIngestHealth_ReturnsMetricsFromVector(t *testing.T) {
	fakeVector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":{
			"sources":{"nodes":[
				{"componentId":"unifi","metrics":{"receivedEventsTotal":{"receivedEventsTotal":1234}}},
				{"componentId":"hosts_tcp","metrics":{"receivedEventsTotal":{"receivedEventsTotal":56}}}
			]},
			"sinks":{"nodes":[
				{"componentId":"loki","metrics":{"sentEventsTotal":{"sentEventsTotal":1290}}},
				{"componentId":"siem_api","metrics":{"sentEventsTotal":null}}
			]},
			"transforms":{"nodes":[
				{"componentId":"enrich_geo","metrics":{"receivedEventsTotal":{"receivedEventsTotal":1300},"sentEventsTotal":{"sentEventsTotal":1300}}},
				{"componentId":"drop_blank_messages","metrics":{"receivedEventsTotal":{"receivedEventsTotal":1300},"sentEventsTotal":{"sentEventsTotal":1290}}}
			]}
		}}`))
	}))
	defer fakeVector.Close()

	s, st := newTestServer(t)
	s.deps.Vector = vector.New(fakeVector.URL, fakeVector.Client())

	token := authToken(t, st, "viewer", 100)
	req := httptest.NewRequest(http.MethodGet, "/sources/ingest-health", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	var got ingestHealthResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if got.Degraded {
		t.Fatal("Degraded = true, want false on a successful Vector response")
	}
	if got.ReceivedEventsPerSource["unifi"] != 1234 || got.ReceivedEventsPerSource["hosts_tcp"] != 56 {
		t.Fatalf("ReceivedEventsPerSource = %+v", got.ReceivedEventsPerSource)
	}
	if got.LokiSentEventsTotal != 1290 {
		t.Fatalf("LokiSentEventsTotal = %v, want 1290", got.LokiSentEventsTotal)
	}
	if got.BlankMessagesFilteredTotal != 10 {
		t.Fatalf("BlankMessagesFilteredTotal = %v, want 10 (drop_blank_messages' own received-1300 minus sent-1290)", got.BlankMessagesFilteredTotal)
	}
}

func TestIngestHealth_ZeroBlankMessagesFilteredWhenTransformAbsent(t *testing.T) {
	fakeVector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":{
			"sources":{"nodes":[]},
			"sinks":{"nodes":[]},
			"transforms":{"nodes":[]}
		}}`))
	}))
	defer fakeVector.Close()

	s, st := newTestServer(t)
	s.deps.Vector = vector.New(fakeVector.URL, fakeVector.Client())

	token := authToken(t, st, "viewer", 100)
	req := httptest.NewRequest(http.MethodGet, "/sources/ingest-health", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	var got ingestHealthResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if got.BlankMessagesFilteredTotal != 0 {
		t.Fatalf("BlankMessagesFilteredTotal = %v, want 0 when the transform doesn't appear in Vector's response", got.BlankMessagesFilteredTotal)
	}
}

func TestIngestHealth_DegradedWhenVectorUnset(t *testing.T) {
	s, st := newTestServer(t) // s.deps.Vector left nil

	token := authToken(t, st, "viewer", 100)
	req := httptest.NewRequest(http.MethodGet, "/sources/ingest-health", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (never 502 for a missing Vector client), body=%s", rec.Code, rec.Body.String())
	}
	var got ingestHealthResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if !got.Degraded {
		t.Fatal("Degraded = false, want true when Deps.Vector is nil")
	}
}

func TestIngestHealth_DegradedWhenVectorUnreachable(t *testing.T) {
	s, st := newTestServer(t)
	s.deps.Vector = vector.New("http://127.0.0.1:1", nil) // nothing listens here

	token := authToken(t, st, "viewer", 100)
	req := httptest.NewRequest(http.MethodGet, "/sources/ingest-health", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	var got ingestHealthResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if !got.Degraded {
		t.Fatal("Degraded = false, want true when the Vector request fails")
	}
}

func TestIngestHealth_RequiresViewerRole(t *testing.T) {
	s, st := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/sources/ingest-health", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 with no Authorization header, body=%s", rec.Code, rec.Body.String())
	}
	_ = st
}
