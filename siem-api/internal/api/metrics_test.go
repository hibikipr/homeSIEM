package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hibikipr/homeSIEM/siem-api/internal/store"
)

func TestHandleMetrics(t *testing.T) {
	s, st := newTestServer(t)
	ctx := context.Background()
	if _, err := st.UpsertSource(ctx, store.Source{
		Name: "udm-ultra", Address: "10.0.0.1", Transport: "udp/514", Parser: "unifi-os", HeartbeatSec: 900,
	}); err != nil {
		t.Fatalf("UpsertSource() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("Content-Type = %q, want text/plain prefix", ct)
	}
	body := rec.Body.String()
	// Never heartbeated, so sourceStatus() reads it as silent (0), and it
	// has no last_seen_at line at all - same shape handleListSources uses.
	if !strings.Contains(body, `siem_source_up{source="udm-ultra"} 0`) {
		t.Errorf("body missing siem_source_up line, got:\n%s", body)
	}
	if !strings.Contains(body, `siem_source_heartbeat_seconds{source="udm-ultra"} 900`) {
		t.Errorf("body missing siem_source_heartbeat_seconds line, got:\n%s", body)
	}
}

func TestHandleMetrics_Unauthenticated(t *testing.T) {
	s, _ := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 with no Authorization header, body=%s", rec.Code, rec.Body.String())
	}
}
