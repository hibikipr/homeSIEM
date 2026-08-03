package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hibikipr/homeSIEM/siem-api/internal/store"
)

func TestListSources(t *testing.T) {
	s, st := newTestServer(t)
	ctx := context.Background()
	if _, err := st.UpsertSource(ctx, store.Source{
		Name: "udm-ultra", Address: "10.0.0.1", Transport: "udp/514", Parser: "unifi-os", HeartbeatSec: 900,
	}); err != nil {
		t.Fatalf("UpsertSource() error = %v", err)
	}

	token := authToken(t, st, "viewer", 100)
	req := httptest.NewRequest(http.MethodGet, "/sources", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	var got []sourceResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if len(got) != 1 || got[0].Name != "udm-ultra" {
		t.Fatalf("got = %+v", got)
	}
}

func TestClaimSource_RequiresAdmin(t *testing.T) {
	s, st := newTestServer(t)
	ctx := context.Background()
	src, err := st.UpsertSource(ctx, store.Source{
		Name: "unclaimed-host", Address: "10.0.0.2", Transport: "tcp/601", Parser: "rfc5424", HeartbeatSec: 900,
	})
	if err != nil {
		t.Fatalf("UpsertSource() error = %v", err)
	}

	token := authToken(t, st, "analyst", 50)
	req := httptest.NewRequest(http.MethodPost, "/sources/"+itoa(src.ID)+"/claim", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestClaimSource_AdminSucceeds(t *testing.T) {
	s, st := newTestServer(t)
	ctx := context.Background()
	src, err := st.UpsertSource(ctx, store.Source{
		Name: "unclaimed-host", Address: "10.0.0.2", Transport: "tcp/601", Parser: "rfc5424", HeartbeatSec: 900,
	})
	if err != nil {
		t.Fatalf("UpsertSource() error = %v", err)
	}

	token := authToken(t, st, "admin", 10)
	req := httptest.NewRequest(http.MethodPost, "/sources/"+itoa(src.ID)+"/claim", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204, body=%s", rec.Code, rec.Body.String())
	}

	sources, err := st.ListSources(ctx)
	if err != nil {
		t.Fatalf("ListSources() error = %v", err)
	}
	if !sources[0].Claimed {
		t.Error("Claimed = false after claim")
	}
}

func TestSourceHeartbeat_InvalidToken(t *testing.T) {
	s, _ := newTestServer(t)
	body := strings.NewReader(`{"name":"udm-ultra","address":"10.0.0.1","transport":"udp/514","parser":"unifi-os"}`)
	req := httptest.NewRequest(http.MethodPost, "/sources/heartbeat", body)
	req.Header.Set("X-Fastpath-Token", "wrong-token")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestSourceHeartbeat_RegistersNewSourceAndBumpsLastSeen(t *testing.T) {
	s, st := newTestServer(t)
	ctx := context.Background()

	body := strings.NewReader(`{"name":"udm-ultra","address":"10.0.0.1","transport":"udp/514","parser":"unifi-os"}`)
	req := httptest.NewRequest(http.MethodPost, "/sources/heartbeat", body)
	req.Header.Set("X-Fastpath-Token", "test-fastpath-token")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202, body=%s", rec.Code, rec.Body.String())
	}

	sources, err := st.ListSources(ctx)
	if err != nil {
		t.Fatalf("ListSources() error = %v", err)
	}
	if len(sources) != 1 {
		t.Fatalf("len(sources) = %d, want 1", len(sources))
	}
	got := sources[0]
	if got.Name != "udm-ultra" || got.Address != "10.0.0.1" || got.Transport != "udp/514" || got.Parser != "unifi-os" {
		t.Errorf("source = %+v, want name/address/transport/parser to match the heartbeat body", got)
	}
	if got.Claimed {
		t.Error("Claimed = true, want a newly-registered source to start unclaimed")
	}
	if got.LastSeenAt == nil {
		t.Error("LastSeenAt = nil, want it set by the heartbeat")
	}
}

func TestSourceHeartbeat_ExistingSourceUpdatesLastSeen(t *testing.T) {
	s, st := newTestServer(t)
	ctx := context.Background()
	created, err := st.UpsertSource(ctx, store.Source{
		Name: "udm-ultra", Address: "10.0.0.1", Transport: "udp/514", Parser: "unifi-os", HeartbeatSec: 900,
	})
	if err != nil {
		t.Fatalf("UpsertSource() error = %v", err)
	}
	if created.LastSeenAt != nil {
		t.Fatal("test setup: expected LastSeenAt nil before any heartbeat")
	}

	body := strings.NewReader(`{"name":"udm-ultra","address":"10.0.0.1","transport":"udp/514","parser":"unifi-os"}`)
	req := httptest.NewRequest(http.MethodPost, "/sources/heartbeat", body)
	req.Header.Set("X-Fastpath-Token", "test-fastpath-token")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202, body=%s", rec.Code, rec.Body.String())
	}

	sources, err := st.ListSources(ctx)
	if err != nil {
		t.Fatalf("ListSources() error = %v", err)
	}
	if len(sources) != 1 {
		t.Fatalf("len(sources) = %d, want 1 (heartbeat must not duplicate an existing source)", len(sources))
	}
	if sources[0].LastSeenAt == nil {
		t.Error("LastSeenAt = nil, want the heartbeat to have bumped it")
	}
}

func TestSourceHeartbeat_InvalidJSON(t *testing.T) {
	s, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/sources/heartbeat", strings.NewReader("not json"))
	req.Header.Set("X-Fastpath-Token", "test-fastpath-token")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}
