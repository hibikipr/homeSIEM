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

func TestRenameSource_RequiresAdmin(t *testing.T) {
	s, st := newTestServer(t)
	ctx := context.Background()
	src, err := st.UpsertSource(ctx, store.Source{
		Name: "192.168.3.223", Address: "192.168.3.223", Transport: "tcp/601", Parser: "rfc5424", HeartbeatSec: 900,
	})
	if err != nil {
		t.Fatalf("UpsertSource() error = %v", err)
	}

	token := authToken(t, st, "analyst", 50)
	req := httptest.NewRequest(http.MethodPut, "/sources/"+itoa(src.ID)+"/rename",
		strings.NewReader(`{"display_name":"Home Assistant"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestRenameSource_AdminSucceeds_AndShowsInListSources(t *testing.T) {
	s, st := newTestServer(t)
	ctx := context.Background()
	src, err := st.UpsertSource(ctx, store.Source{
		Name: "192.168.3.223", Address: "192.168.3.223", Transport: "tcp/601", Parser: "rfc5424", HeartbeatSec: 900,
	})
	if err != nil {
		t.Fatalf("UpsertSource() error = %v", err)
	}

	adminToken := authToken(t, st, "admin", 10)
	req := httptest.NewRequest(http.MethodPut, "/sources/"+itoa(src.ID)+"/rename",
		strings.NewReader(`{"display_name":"Home Assistant"}`))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204, body=%s", rec.Code, rec.Body.String())
	}

	viewerToken := authToken(t, st, "viewer", 100)
	listReq := httptest.NewRequest(http.MethodGet, "/sources", nil)
	listReq.Header.Set("Authorization", "Bearer "+viewerToken)
	listRec := httptest.NewRecorder()
	s.Handler().ServeHTTP(listRec, listReq)

	var got []sourceResponse
	if err := json.Unmarshal(listRec.Body.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if len(got) != 1 || got[0].DisplayName != "Home Assistant" || got[0].Name != "192.168.3.223" {
		t.Fatalf("got = %+v, want DisplayName=Home Assistant with Name unchanged", got)
	}
}

func TestRenameSource_UnknownID_Returns404(t *testing.T) {
	s, st := newTestServer(t)
	token := authToken(t, st, "admin", 10)
	req := httptest.NewRequest(http.MethodPut, "/sources/999/rename", strings.NewReader(`{"display_name":"x"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body=%s", rec.Code, rec.Body.String())
	}
}

func TestRenameSource_InvalidJSON(t *testing.T) {
	s, st := newTestServer(t)
	ctx := context.Background()
	src, err := st.UpsertSource(ctx, store.Source{
		Name: "192.168.3.223", Address: "192.168.3.223", Transport: "tcp/601", Parser: "rfc5424", HeartbeatSec: 900,
	})
	if err != nil {
		t.Fatalf("UpsertSource() error = %v", err)
	}

	token := authToken(t, st, "admin", 10)
	req := httptest.NewRequest(http.MethodPut, "/sources/"+itoa(src.ID)+"/rename", strings.NewReader("not json"))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestSetHeartbeat_RequiresAdmin(t *testing.T) {
	s, st := newTestServer(t)
	ctx := context.Background()
	src, err := st.UpsertSource(ctx, store.Source{
		Name: "192.168.3.223", Address: "192.168.3.223", Transport: "tcp/601", Parser: "rfc5424", HeartbeatSec: 900,
	})
	if err != nil {
		t.Fatalf("UpsertSource() error = %v", err)
	}

	token := authToken(t, st, "analyst", 50)
	req := httptest.NewRequest(http.MethodPut, "/sources/"+itoa(src.ID)+"/heartbeat",
		strings.NewReader(`{"heartbeat_sec":3600}`))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestSetHeartbeat_AdminSucceeds_AndShowsInListSources(t *testing.T) {
	s, st := newTestServer(t)
	ctx := context.Background()
	src, err := st.UpsertSource(ctx, store.Source{
		Name: "192.168.3.223", Address: "192.168.3.223", Transport: "tcp/601", Parser: "rfc5424", HeartbeatSec: 900,
	})
	if err != nil {
		t.Fatalf("UpsertSource() error = %v", err)
	}

	adminToken := authToken(t, st, "admin", 10)
	req := httptest.NewRequest(http.MethodPut, "/sources/"+itoa(src.ID)+"/heartbeat",
		strings.NewReader(`{"heartbeat_sec":3600}`))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204, body=%s", rec.Code, rec.Body.String())
	}

	viewerToken := authToken(t, st, "viewer", 100)
	listReq := httptest.NewRequest(http.MethodGet, "/sources", nil)
	listReq.Header.Set("Authorization", "Bearer "+viewerToken)
	listRec := httptest.NewRecorder()
	s.Handler().ServeHTTP(listRec, listReq)

	var got []sourceResponse
	if err := json.Unmarshal(listRec.Body.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if len(got) != 1 || got[0].HeartbeatSec != 3600 {
		t.Fatalf("got = %+v, want HeartbeatSec=3600", got)
	}
}

func TestSetHeartbeat_UnknownID_Returns404(t *testing.T) {
	s, st := newTestServer(t)
	token := authToken(t, st, "admin", 10)
	req := httptest.NewRequest(http.MethodPut, "/sources/999/heartbeat", strings.NewReader(`{"heartbeat_sec":3600}`))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body=%s", rec.Code, rec.Body.String())
	}
}

func TestSetHeartbeat_InvalidJSON(t *testing.T) {
	s, st := newTestServer(t)
	ctx := context.Background()
	src, err := st.UpsertSource(ctx, store.Source{
		Name: "192.168.3.223", Address: "192.168.3.223", Transport: "tcp/601", Parser: "rfc5424", HeartbeatSec: 900,
	})
	if err != nil {
		t.Fatalf("UpsertSource() error = %v", err)
	}

	token := authToken(t, st, "admin", 10)
	req := httptest.NewRequest(http.MethodPut, "/sources/"+itoa(src.ID)+"/heartbeat", strings.NewReader("not json"))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestSetHeartbeat_TooLow_Returns400(t *testing.T) {
	s, st := newTestServer(t)
	ctx := context.Background()
	src, err := st.UpsertSource(ctx, store.Source{
		Name: "192.168.3.223", Address: "192.168.3.223", Transport: "tcp/601", Parser: "rfc5424", HeartbeatSec: 900,
	})
	if err != nil {
		t.Fatalf("UpsertSource() error = %v", err)
	}

	token := authToken(t, st, "admin", 10)
	req := httptest.NewRequest(http.MethodPut, "/sources/"+itoa(src.ID)+"/heartbeat",
		strings.NewReader(`{"heartbeat_sec":5}`))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body=%s", rec.Code, rec.Body.String())
	}
}

func TestDeleteSource_RequiresAdmin(t *testing.T) {
	s, st := newTestServer(t)
	ctx := context.Background()
	src, err := st.UpsertSource(ctx, store.Source{
		Name: "192.168.3.223", Address: "192.168.3.223", Transport: "tcp/601", Parser: "rfc5424", HeartbeatSec: 900,
	})
	if err != nil {
		t.Fatalf("UpsertSource() error = %v", err)
	}

	token := authToken(t, st, "analyst", 50)
	req := httptest.NewRequest(http.MethodDelete, "/sources/"+itoa(src.ID), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestDeleteSource_AdminSucceeds_AndRemovesFromListSources(t *testing.T) {
	s, st := newTestServer(t)
	ctx := context.Background()
	src, err := st.UpsertSource(ctx, store.Source{
		Name: "192.168.3.223", Address: "192.168.3.223", Transport: "tcp/601", Parser: "rfc5424", HeartbeatSec: 900,
	})
	if err != nil {
		t.Fatalf("UpsertSource() error = %v", err)
	}

	adminToken := authToken(t, st, "admin", 10)
	req := httptest.NewRequest(http.MethodDelete, "/sources/"+itoa(src.ID), nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204, body=%s", rec.Code, rec.Body.String())
	}

	sources, err := st.ListSources(ctx)
	if err != nil {
		t.Fatalf("ListSources() error = %v", err)
	}
	if len(sources) != 0 {
		t.Fatalf("ListSources() after delete = %+v, want empty", sources)
	}
}

func TestDeleteSource_UnknownID_Returns404(t *testing.T) {
	s, st := newTestServer(t)
	token := authToken(t, st, "admin", 10)
	req := httptest.NewRequest(http.MethodDelete, "/sources/999", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body=%s", rec.Code, rec.Body.String())
	}
}

func TestDeleteSource_InvalidID(t *testing.T) {
	s, st := newTestServer(t)
	token := authToken(t, st, "admin", 10)
	req := httptest.NewRequest(http.MethodDelete, "/sources/not-a-number", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
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

func TestListSources_HealthyWhenWithinHeartbeat(t *testing.T) {
	s, st := newTestServer(t)
	ctx := context.Background()
	src, err := st.UpsertSource(ctx, store.Source{
		Name: "udm-ultra", Address: "10.0.0.1", Transport: "udp/514", Parser: "unifi-os", HeartbeatSec: 900,
	})
	if err != nil {
		t.Fatalf("UpsertSource() error = %v", err)
	}
	if err := st.TouchSourceLastSeen(ctx, src.Name, time.Now().UTC()); err != nil {
		t.Fatalf("TouchSourceLastSeen() error = %v", err)
	}

	token := authToken(t, st, "viewer", 100)
	req := httptest.NewRequest(http.MethodGet, "/sources", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	var got []sourceResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if len(got) != 1 || got[0].Status != "healthy" {
		t.Fatalf("got = %+v, want status=healthy", got)
	}
}

func TestListSources_SilentWhenNeverSeen(t *testing.T) {
	s, st := newTestServer(t)
	ctx := context.Background()
	if _, err := st.UpsertSource(ctx, store.Source{
		Name: "unclaimed-host", Address: "10.0.0.2", Transport: "tcp/601", Parser: "rfc5424", HeartbeatSec: 900,
	}); err != nil {
		t.Fatalf("UpsertSource() error = %v", err)
	}

	token := authToken(t, st, "viewer", 100)
	req := httptest.NewRequest(http.MethodGet, "/sources", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	var got []sourceResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if len(got) != 1 || got[0].Status != "silent" {
		t.Fatalf("got = %+v, want status=silent (never seen)", got)
	}
}

func TestListSources_SilentWhenPastHeartbeat(t *testing.T) {
	s, st := newTestServer(t)
	ctx := context.Background()
	src, err := st.UpsertSource(ctx, store.Source{
		Name: "udm-ultra", Address: "10.0.0.1", Transport: "udp/514", Parser: "unifi-os", HeartbeatSec: 60,
	})
	if err != nil {
		t.Fatalf("UpsertSource() error = %v", err)
	}
	if err := st.TouchSourceLastSeen(ctx, src.Name, time.Now().UTC().Add(-10*time.Minute)); err != nil {
		t.Fatalf("TouchSourceLastSeen() error = %v", err)
	}

	token := authToken(t, st, "viewer", 100)
	req := httptest.NewRequest(http.MethodGet, "/sources", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	var got []sourceResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if len(got) != 1 || got[0].Status != "silent" {
		t.Fatalf("got = %+v, want status=silent (10m since last seen, 60s heartbeat)", got)
	}
}

func TestListSources_IncludesEventsPerMin(t *testing.T) {
	fakeLoki := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"success","data":{"resultType":"matrix","result":[
			{"metric":{"source":"udm-ultra"},"values":[[1700000000,"25"],[1700000300,"50"]]}
		]}}`))
	}))
	defer fakeLoki.Close()

	s, st := newTestServer(t)
	s.deps.Loki = loki.New(fakeLoki.URL, fakeLoki.Client())
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

	var got []sourceResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if len(got) != 1 || got[0].EventsPerMin != 10 {
		t.Fatalf("got = %+v, want events_per_min=10 (50/5)", got)
	}
}

func TestListSources_EventsPerMinZeroWhenLokiUnset(t *testing.T) {
	// Regression guard: handleListSources must not panic when s.deps.Loki is
	// nil, which is the case for every other existing sources_test.go case
	// (newTestServer doesn't set Loki unless a test opts in).
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
	if len(got) != 1 || got[0].EventsPerMin != 0 {
		t.Fatalf("got = %+v, want events_per_min=0 when Loki is unset", got)
	}
}
