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
		w.Write([]byte(`# TYPE vector_component_received_events_total counter
vector_component_received_events_total{component_id="unifi",component_kind="source",component_type="syslog",host="h"} 1234 1792786573500
vector_component_received_events_total{component_id="hosts_tcp",component_kind="source",component_type="syslog",host="h"} 56 1792786573500
vector_component_received_events_total{component_id="vector_metrics",component_kind="source",component_type="internal_metrics",host="h"} 999 1792786573500
vector_component_sent_events_total{component_id="loki",component_kind="sink",component_type="loki",host="h"} 1290 1792786573500
vector_component_sent_events_total{component_id="siem_api",component_kind="sink",component_type="http",host="h"} 40 1792786573500
vector_component_received_events_total{component_id="enrich_geo",component_kind="transform",component_type="remap",host="h"} 1300 1792786573500
vector_component_sent_events_total{component_id="enrich_geo",component_kind="transform",component_type="remap",host="h",output="_default"} 1300 1792786573500
vector_component_received_events_total{component_id="drop_blank_messages",component_kind="transform",component_type="filter",host="h"} 1300 1792786573500
vector_component_sent_events_total{component_id="drop_blank_messages",component_kind="transform",component_type="filter",host="h",output="_default"} 1290 1792786573500
`))
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
	if len(got.ReceivedEventsPerSource) != 2 || got.ReceivedEventsPerSource["unifi"] != 1234 || got.ReceivedEventsPerSource["hosts_tcp"] != 56 {
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
		w.Write([]byte("# no component metrics yet\n"))
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
