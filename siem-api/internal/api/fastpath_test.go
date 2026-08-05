package api

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hibikipr/homeSIEM/siem-api/internal/store"
)

func TestFastpath_MissingToken(t *testing.T) {
	s, _ := newTestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/ingest/fastpath", bytes.NewReader([]byte(`{}`)))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestFastpath_WanDrop_RaisesAlertForExistingRule(t *testing.T) {
	s, st := newTestServer(t)
	ctx := context.Background()

	if _, err := st.CreateRule(ctx, store.Rule{
		Name: "wan-drop", Shape: "threshold", Severity: "warning",
		Destinations: []string{"inapp"}, CooldownSec: 3600, IntervalSec: 60, Enabled: true,
	}, nil); err != nil {
		t.Fatalf("CreateRule() error = %v", err)
	}

	body := `{"src_ip":"10.0.0.5","dst_ip":"1.2.3.4","dst_port":22,"action":"drop","message":"drop line"}`
	req := httptest.NewRequest(http.MethodPost, "/ingest/fastpath", bytes.NewReader([]byte(body)))
	req.Header.Set("X-Fastpath-Token", "test-fastpath-token")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202, body=%s", rec.Code, rec.Body.String())
	}

	alertsList, err := st.ListAlerts(ctx, "open")
	if err != nil {
		t.Fatalf("ListAlerts() error = %v", err)
	}
	if len(alertsList) != 1 || alertsList[0].GroupKey != "10.0.0.5" {
		t.Fatalf("alerts = %+v, want one open alert for 10.0.0.5", alertsList)
	}
}

func TestFastpath_UnconfiguredRuleSkippedSilently(t *testing.T) {
	s, st := newTestServer(t)
	ctx := context.Background()

	// No "wan-drop" rule created.
	body := `{"src_ip":"10.0.0.5","dst_ip":"1.2.3.4","dst_port":22,"action":"drop","message":"drop line"}`
	req := httptest.NewRequest(http.MethodPost, "/ingest/fastpath", bytes.NewReader([]byte(body)))
	req.Header.Set("X-Fastpath-Token", "test-fastpath-token")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202 even when no matching rule exists", rec.Code)
	}
	alertsList, err := st.ListAlerts(ctx, "")
	if err != nil {
		t.Fatalf("ListAlerts() error = %v", err)
	}
	if len(alertsList) != 0 {
		t.Fatalf("alerts = %+v, want none", alertsList)
	}
}

func TestFastpath_ThreatIntelHit_RaisesAlert(t *testing.T) {
	s, st := newTestServer(t)
	ctx := context.Background()

	if _, err := st.CreateRule(ctx, store.Rule{
		Name: "threat-intel-hit", Shape: "threshold", Severity: "critical",
		Destinations: []string{"inapp"}, CooldownSec: 3600, IntervalSec: 60, Enabled: true,
	}, nil); err != nil {
		t.Fatalf("CreateRule() error = %v", err)
	}

	body := `{"src_ip":"203.0.113.9","threat_intel":"known-scanner","message":"threat line"}`
	req := httptest.NewRequest(http.MethodPost, "/ingest/fastpath", bytes.NewReader([]byte(body)))
	req.Header.Set("X-Fastpath-Token", "test-fastpath-token")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", rec.Code)
	}
	alertsList, err := st.ListAlerts(ctx, "open")
	if err != nil {
		t.Fatalf("ListAlerts() error = %v", err)
	}
	if len(alertsList) != 1 || alertsList[0].GroupKey != "203.0.113.9" {
		t.Fatalf("alerts = %+v, want one open alert for 203.0.113.9", alertsList)
	}
}
