package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
