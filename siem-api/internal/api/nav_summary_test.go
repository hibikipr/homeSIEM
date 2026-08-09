package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hibikipr/homeSIEM/siem-api/internal/loki"
	"github.com/hibikipr/homeSIEM/siem-api/internal/store"
)

func TestNavSummary_ReturnsRateAndOpenAlertCount(t *testing.T) {
	var gotPath string
	fakeLoki := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[
			{"metric":{},"value":[1700000000,"42"]}
		]}}`))
	}))
	defer fakeLoki.Close()

	s, st := newTestServer(t)
	s.deps.Loki = loki.New(fakeLoki.URL, fakeLoki.Client())
	ctx := context.Background()

	rule, err := st.CreateRule(ctx, store.Rule{Name: "r", Shape: "absence", Severity: "warning",
		Destinations: []string{"inapp"}, CooldownSec: 60, IntervalSec: 60, Enabled: true}, nil)
	if err != nil {
		t.Fatalf("CreateRule() error = %v", err)
	}
	now := time.Now().UTC()
	if _, err := st.InsertAlert(ctx, store.Alert{RuleID: rule.ID, GroupKey: "a", Severity: "warning",
		Title: "t", Body: "b", EventCount: 1, Context: "{}", State: "open", FirstSeenAt: now, LastSeenAt: now}); err != nil {
		t.Fatalf("InsertAlert() error = %v", err)
	}
	if _, err := st.InsertAlert(ctx, store.Alert{RuleID: rule.ID, GroupKey: "b", Severity: "warning",
		Title: "t", Body: "b", EventCount: 1, Context: "{}", State: "acked", FirstSeenAt: now, LastSeenAt: now}); err != nil {
		t.Fatalf("InsertAlert() error = %v", err)
	}

	token := authToken(t, st, "viewer", 100)
	req := httptest.NewRequest(http.MethodGet, "/nav/summary", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}

	// Uses the instant /query endpoint, never /query_range - same reason
	// as QueryInstant's other callers (see its doc comment in matrix.go).
	if gotPath != "/loki/api/v1/query" {
		t.Errorf("loki request path = %q, want /loki/api/v1/query (instant)", gotPath)
	}

	var got navSummaryResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if got.EventsPerMin != 42 {
		t.Errorf("EventsPerMin = %d, want 42", got.EventsPerMin)
	}
	// Only the "open" alert counts - the "acked" one must not be included.
	if got.OpenAlertCount != 1 {
		t.Errorf("OpenAlertCount = %d, want 1", got.OpenAlertCount)
	}
}

func TestNavSummary_ZeroEventsWhenLokiHasNoData(t *testing.T) {
	fakeLoki := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[]}}`))
	}))
	defer fakeLoki.Close()

	s, st := newTestServer(t)
	s.deps.Loki = loki.New(fakeLoki.URL, fakeLoki.Client())

	token := authToken(t, st, "viewer", 100)
	req := httptest.NewRequest(http.MethodGet, "/nav/summary", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	var got navSummaryResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if got.EventsPerMin != 0 {
		t.Errorf("EventsPerMin = %d, want 0 (genuinely quiet minute, not an error)", got.EventsPerMin)
	}
	if got.OpenAlertCount != 0 {
		t.Errorf("OpenAlertCount = %d, want 0 (no alerts inserted)", got.OpenAlertCount)
	}
}

func TestNavSummary_RequiresViewerRole(t *testing.T) {
	s, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/nav/summary", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 with no Authorization header, body=%s", rec.Code, rec.Body.String())
	}
}

func TestNavSummary_LokiFailureReturns502(t *testing.T) {
	s, st := newTestServer(t)
	s.deps.Loki = loki.New("http://127.0.0.1:1", nil) // nothing listens here

	token := authToken(t, st, "viewer", 100)
	req := httptest.NewRequest(http.MethodGet, "/nav/summary", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502 when Loki is unreachable, body=%s", rec.Code, rec.Body.String())
	}
}
