package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hibikipr/homeSIEM/siem-api/internal/ntfy"
)

func TestGetNotificationSettings_RequiresAdmin(t *testing.T) {
	s, st := newTestServer(t)
	token := authToken(t, st, "analyst", 50)

	req := httptest.NewRequest(http.MethodGet, "/settings/notifications", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestGetNotificationSettings_NotConfiguredByDefault(t *testing.T) {
	s, st := newTestServer(t)
	token := authToken(t, st, "admin", 5)

	req := httptest.NewRequest(http.MethodGet, "/settings/notifications", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	var resp notificationSettingsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if resp.NtfyConfigured {
		t.Error("NtfyConfigured = true, want false when NtfyURL/NtfyTopic are unset")
	}
	if resp.MinSeverity != "info" {
		t.Errorf("MinSeverity = %q, want info (the default)", resp.MinSeverity)
	}
}

func TestGetNotificationSettings_ConfiguredWhenUrlAndTopicSet(t *testing.T) {
	s, st := newTestServer(t)
	s.deps.NtfyURL = "https://ntfy.townsville.cc"
	s.deps.NtfyTopic = "homesiem"
	token := authToken(t, st, "admin", 5)

	req := httptest.NewRequest(http.MethodGet, "/settings/notifications", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	var resp notificationSettingsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if !resp.NtfyConfigured {
		t.Error("NtfyConfigured = false, want true when NtfyURL and NtfyTopic are both set")
	}
}

func TestUpdateNotificationSettings_PersistsValidSeverity(t *testing.T) {
	s, st := newTestServer(t)
	token := authToken(t, st, "admin", 5)

	body := `{"min_severity":"critical"}`
	req := httptest.NewRequest(http.MethodPut, "/settings/notifications", bytes.NewReader([]byte(body)))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204, body=%s", rec.Code, rec.Body.String())
	}

	got, err := st.GetMinNotifySeverity(req.Context())
	if err != nil {
		t.Fatalf("GetMinNotifySeverity() error = %v", err)
	}
	if got != "critical" {
		t.Errorf("min_severity = %q, want critical", got)
	}
}

func TestUpdateNotificationSettings_RejectsInvalidSeverity(t *testing.T) {
	s, st := newTestServer(t)
	token := authToken(t, st, "admin", 5)

	body := `{"min_severity":"apocalyptic"}`
	req := httptest.NewRequest(http.MethodPut, "/settings/notifications", bytes.NewReader([]byte(body)))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body=%s", rec.Code, rec.Body.String())
	}
}

func TestTestNotification_NotConfigured(t *testing.T) {
	s, st := newTestServer(t)
	token := authToken(t, st, "admin", 5)

	req := httptest.NewRequest(http.MethodPost, "/settings/notifications/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body=%s", rec.Code, rec.Body.String())
	}
}

func TestTestNotification_Success(t *testing.T) {
	var published bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		published = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	s, st := newTestServer(t)
	s.deps.NtfyURL = srv.URL
	s.deps.NtfyTopic = "homesiem"
	s.deps.Ntfy = ntfy.New(srv.URL, "homesiem", "", srv.Client())
	token := authToken(t, st, "admin", 5)

	req := httptest.NewRequest(http.MethodPost, "/settings/notifications/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	if !published {
		t.Error("expected the fake ntfy server to receive a publish request")
	}
}

func TestTestNotification_PublishFailureReturns502(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	s, st := newTestServer(t)
	s.deps.NtfyURL = srv.URL
	s.deps.NtfyTopic = "homesiem"
	s.deps.Ntfy = ntfy.New(srv.URL, "homesiem", "", srv.Client())
	token := authToken(t, st, "admin", 5)

	req := httptest.NewRequest(http.MethodPost, "/settings/notifications/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502, body=%s", rec.Code, rec.Body.String())
	}
}
