package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hibikipr/homeSIEM/siem-api/internal/store"
)

func TestListAlerts_FiltersByState(t *testing.T) {
	s, st := newTestServer(t)
	ctx := context.Background()

	rule, err := st.CreateRule(ctx, store.Rule{Name: "r", Shape: "absence", Severity: "low",
		Destinations: []string{"inapp"}, CooldownSec: 60, IntervalSec: 60, Enabled: true}, nil)
	if err != nil {
		t.Fatalf("CreateRule() error = %v", err)
	}
	now := time.Now().UTC()
	if _, err := st.InsertAlert(ctx, store.Alert{RuleID: rule.ID, GroupKey: "a", Severity: "low",
		Title: "t", Body: "b", EventCount: 1, Context: "{}", State: "open", FirstSeenAt: now, LastSeenAt: now}); err != nil {
		t.Fatalf("InsertAlert() error = %v", err)
	}
	if _, err := st.InsertAlert(ctx, store.Alert{RuleID: rule.ID, GroupKey: "b", Severity: "low",
		Title: "t", Body: "b", EventCount: 1, Context: "{}", State: "acked", FirstSeenAt: now, LastSeenAt: now}); err != nil {
		t.Fatalf("InsertAlert() error = %v", err)
	}

	token := authToken(t, st, "viewer", 100)
	req := httptest.NewRequest(http.MethodGet, "/alerts?state=open", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	var got []alertResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if len(got) != 1 || got[0].GroupKey != "a" {
		t.Fatalf("got = %+v, want one alert with GroupKey a", got)
	}
}

func TestAckAlert_ViewerForbidden(t *testing.T) {
	s, st := newTestServer(t)
	ctx := context.Background()
	rule, err := st.CreateRule(ctx, store.Rule{Name: "r", Shape: "absence", Severity: "low",
		Destinations: []string{"inapp"}, CooldownSec: 60, IntervalSec: 60, Enabled: true}, nil)
	if err != nil {
		t.Fatalf("CreateRule() error = %v", err)
	}
	now := time.Now().UTC()
	alert, err := st.InsertAlert(ctx, store.Alert{RuleID: rule.ID, GroupKey: "a", Severity: "low",
		Title: "t", Body: "b", EventCount: 1, Context: "{}", State: "open", FirstSeenAt: now, LastSeenAt: now})
	if err != nil {
		t.Fatalf("InsertAlert() error = %v", err)
	}

	token := authToken(t, st, "viewer", 100)
	req := httptest.NewRequest(http.MethodPost, "/alerts/"+itoa(alert.ID)+"/ack", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestAckAlert_AnalystSucceedsAndAudits(t *testing.T) {
	s, st := newTestServer(t)
	ctx := context.Background()
	rule, err := st.CreateRule(ctx, store.Rule{Name: "r", Shape: "absence", Severity: "low",
		Destinations: []string{"inapp"}, CooldownSec: 60, IntervalSec: 60, Enabled: true}, nil)
	if err != nil {
		t.Fatalf("CreateRule() error = %v", err)
	}
	now := time.Now().UTC()
	alert, err := st.InsertAlert(ctx, store.Alert{RuleID: rule.ID, GroupKey: "a", Severity: "low",
		Title: "t", Body: "b", EventCount: 1, Context: "{}", State: "open", FirstSeenAt: now, LastSeenAt: now})
	if err != nil {
		t.Fatalf("InsertAlert() error = %v", err)
	}

	token := authToken(t, st, "analyst", 50)
	req := httptest.NewRequest(http.MethodPost, "/alerts/"+itoa(alert.ID)+"/ack", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204, body=%s", rec.Code, rec.Body.String())
	}

	got, err := st.GetAlert(ctx, alert.ID)
	if err != nil {
		t.Fatalf("GetAlert() error = %v", err)
	}
	if got.State != "acked" {
		t.Errorf("State = %q, want acked", got.State)
	}

	entries, err := st.ListAudit(ctx, 10)
	if err != nil {
		t.Fatalf("ListAudit() error = %v", err)
	}
	found := false
	for _, e := range entries {
		if e.Action == "alert.ack" {
			found = true
		}
	}
	if !found {
		t.Error("no alert.ack audit entry found")
	}
}

func TestAckAlert_NotFound(t *testing.T) {
	s, st := newTestServer(t)
	token := authToken(t, st, "analyst", 50)

	req := httptest.NewRequest(http.MethodPost, "/alerts/999999/ack", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body=%s", rec.Code, rec.Body.String())
	}
}

func TestAlertsStream_PublishesFromHub(t *testing.T) {
	s, st := newTestServer(t)
	token := authToken(t, st, "viewer", 100)

	req := httptest.NewRequest(http.MethodGet, "/alerts/stream", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	ctx, cancel := context.WithCancel(context.Background())
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		s.Handler().ServeHTTP(rec, req)
		close(done)
	}()

	for s.deps.Hub.SubscriberCount("alerts") == 0 {
		time.Sleep(time.Millisecond)
	}
	s.deps.Hub.Publish("alerts", []byte(`{"id":1}`))
	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("handler did not return after context cancellation")
	}
	if !strings.Contains(rec.Body.String(), `"id":1`) {
		t.Errorf("body = %q, want it to contain the published alert", rec.Body.String())
	}
}

func TestMuteAlert_ViewerForbidden(t *testing.T) {
	s, st := newTestServer(t)
	ctx := context.Background()
	rule, err := st.CreateRule(ctx, store.Rule{Name: "r", Shape: "absence", Severity: "low",
		Destinations: []string{"inapp"}, CooldownSec: 60, IntervalSec: 60, Enabled: true}, nil)
	if err != nil {
		t.Fatalf("CreateRule() error = %v", err)
	}
	now := time.Now().UTC()
	alert, err := st.InsertAlert(ctx, store.Alert{RuleID: rule.ID, GroupKey: "a", Severity: "low",
		Title: "t", Body: "b", EventCount: 1, Context: "{}", State: "open", FirstSeenAt: now, LastSeenAt: now})
	if err != nil {
		t.Fatalf("InsertAlert() error = %v", err)
	}

	token := authToken(t, st, "viewer", 100)
	req := httptest.NewRequest(http.MethodPost, "/alerts/"+itoa(alert.ID)+"/mute", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestMuteAlert_AnalystSucceedsAndAudits(t *testing.T) {
	s, st := newTestServer(t)
	ctx := context.Background()
	rule, err := st.CreateRule(ctx, store.Rule{Name: "r", Shape: "absence", Severity: "low",
		Destinations: []string{"inapp"}, CooldownSec: 60, IntervalSec: 60, Enabled: true}, nil)
	if err != nil {
		t.Fatalf("CreateRule() error = %v", err)
	}
	now := time.Now().UTC()
	alert, err := st.InsertAlert(ctx, store.Alert{RuleID: rule.ID, GroupKey: "a", Severity: "low",
		Title: "t", Body: "b", EventCount: 1, Context: "{}", State: "open", FirstSeenAt: now, LastSeenAt: now})
	if err != nil {
		t.Fatalf("InsertAlert() error = %v", err)
	}

	token := authToken(t, st, "analyst", 50)
	req := httptest.NewRequest(http.MethodPost, "/alerts/"+itoa(alert.ID)+"/mute", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204, body=%s", rec.Code, rec.Body.String())
	}

	got, err := st.GetAlert(ctx, alert.ID)
	if err != nil {
		t.Fatalf("GetAlert() error = %v", err)
	}
	if got.State != "muted" {
		t.Errorf("State = %q, want muted", got.State)
	}
	if got.MutedUntil == nil || !got.MutedUntil.After(now) {
		t.Errorf("MutedUntil = %v, want a time after %v", got.MutedUntil, now)
	}
}

func TestMuteAlert_NotFound(t *testing.T) {
	s, st := newTestServer(t)
	token := authToken(t, st, "analyst", 50)
	req := httptest.NewRequest(http.MethodPost, "/alerts/9999/mute", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body=%s", rec.Code, rec.Body.String())
	}
}

func TestListAlertSamples_ReturnsStoredSamples(t *testing.T) {
	s, st := newTestServer(t)
	ctx := context.Background()
	rule, err := st.CreateRule(ctx, store.Rule{Name: "r", Shape: "absence", Severity: "low",
		Destinations: []string{"inapp"}, CooldownSec: 60, IntervalSec: 60, Enabled: true}, nil)
	if err != nil {
		t.Fatalf("CreateRule() error = %v", err)
	}
	now := time.Now().UTC()
	alert, err := st.InsertAlert(ctx, store.Alert{RuleID: rule.ID, GroupKey: "a", Severity: "low",
		Title: "t", Body: "b", EventCount: 1, Context: "{}", State: "open", FirstSeenAt: now, LastSeenAt: now})
	if err != nil {
		t.Fatalf("InsertAlert() error = %v", err)
	}
	if err := st.AddAlertSample(ctx, alert.ID, now, `{"src_ip":"10.0.0.5","dst_port":443}`); err != nil {
		t.Fatalf("AddAlertSample() error = %v", err)
	}

	token := authToken(t, st, "viewer", 100)
	req := httptest.NewRequest(http.MethodGet, "/alerts/"+itoa(alert.ID)+"/samples", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}

	var resp []alertSampleResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if len(resp) != 1 {
		t.Fatalf("len(resp) = %d, want 1", len(resp))
	}
	if resp[0].Line != `{"src_ip":"10.0.0.5","dst_port":443}` {
		t.Errorf("Line = %q, want the inserted sample", resp[0].Line)
	}
}

func TestListAlertSamples_NotFound(t *testing.T) {
	s, st := newTestServer(t)
	token := authToken(t, st, "viewer", 100)
	req := httptest.NewRequest(http.MethodGet, "/alerts/9999/samples", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body=%s", rec.Code, rec.Body.String())
	}
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
