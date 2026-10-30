package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hibikipr/homeSIEM/siem-api/internal/store"
)

func getHASummary(t *testing.T, s *Server, authHeader string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/ha/summary", nil)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec
}

func TestHASummary_RejectsMissingWrongAndMalformedToken(t *testing.T) {
	s, _ := newTestServer(t)

	for name, header := range map[string]string{
		"missing":       "",
		"wrong":         "Bearer not-the-token",
		"no scheme":     "test-readonly-token",
		"basic scheme":  "Basic test-readonly-token",
		"fastpath copy": "Bearer test-fastpath-token",
	} {
		if rec := getHASummary(t, s, header); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s: status = %d, want 401", name, rec.Code)
		}
	}
}

func TestHASummary_UnconfiguredTokenRejectsEverything(t *testing.T) {
	s, _ := newTestServer(t)
	s.deps.ReadonlyToken = ""

	if rec := getHASummary(t, s, "Bearer "); rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestHASummary_EmptyWhenNoAlerts(t *testing.T) {
	s, _ := newTestServer(t)

	rec := getHASummary(t, s, "Bearer test-readonly-token")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["alerts_24h"] != float64(0) || got["high_24h"] != float64(0) || got["last"] != nil {
		t.Fatalf("body = %v, want zero counts and last=null", got)
	}
}

func TestHASummary_CountsLast24hAndSkipsMuted(t *testing.T) {
	s, st := newTestServer(t)
	ctx := context.Background()

	rule, err := st.CreateRule(ctx, store.Rule{
		Name: "ssh-brute-force", Shape: "threshold", Severity: "critical",
		Destinations: []string{"inapp"}, CooldownSec: 3600, IntervalSec: 60, Enabled: true,
	}, nil)
	if err != nil {
		t.Fatalf("CreateRule() error = %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	for _, a := range []store.Alert{
		{GroupKey: "192.0.2.1", Severity: "warning", Title: "older warning", State: "open", LastSeenAt: now.Add(-3 * time.Hour)},
		{GroupKey: "192.0.2.2", Severity: "critical", Title: "acked critical", State: "acked", LastSeenAt: now.Add(-2 * time.Hour)},
		{GroupKey: "192.0.2.4", Severity: "critical", Title: "SSH brute force · 192.0.2.4", State: "open", LastSeenAt: now.Add(-1 * time.Hour)},
		{GroupKey: "192.0.2.5", Severity: "critical", Title: "muted critical", State: "muted", LastSeenAt: now.Add(-30 * time.Minute)},
		{GroupKey: "192.0.2.6", Severity: "critical", Title: "two days old", State: "open", LastSeenAt: now.Add(-48 * time.Hour)},
	} {
		a.RuleID, a.Body, a.EventCount, a.FirstSeenAt = rule.ID, "body", 1, a.LastSeenAt
		if _, err := st.InsertAlert(ctx, a); err != nil {
			t.Fatalf("InsertAlert(%s) error = %v", a.Title, err)
		}
	}

	rec := getHASummary(t, s, "Bearer test-readonly-token")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	var got haSummaryResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Alerts24h != 3 || got.High24h != 2 {
		t.Fatalf("counts = %d/%d, want 3 alerts, 2 high", got.Alerts24h, got.High24h)
	}
	want := haSummaryAlert{Title: "SSH brute force · 192.0.2.4", Severity: "critical", At: now.Add(-time.Hour).Format(time.RFC3339)}
	if got.Last == nil || *got.Last != want {
		t.Fatalf("last = %+v, want %+v", got.Last, want)
	}
}

func TestHASummary_TokenRefusedOnOtherRoutes(t *testing.T) {
	s, _ := newTestServer(t)

	for _, route := range []struct{ method, path string }{
		{http.MethodGet, "/alerts"},
		{http.MethodGet, "/nav/summary"},
		{http.MethodGet, "/events/search"},
		{http.MethodGet, "/rules"},
		{http.MethodGet, "/settings/auth"},
		{http.MethodPost, "/alerts/1/ack"},
	} {
		req := httptest.NewRequest(route.method, route.path, nil)
		req.Header.Set("Authorization", "Bearer test-readonly-token")
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s: status = %d, want 401", route.method, route.path, rec.Code)
		}
	}

	req := httptest.NewRequest(http.MethodPost, "/ingest/fastpath", nil)
	req.Header.Set("Authorization", "Bearer test-readonly-token")
	req.Header.Set("X-Fastpath-Token", "test-readonly-token")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("POST /ingest/fastpath: status = %d, want 401", rec.Code)
	}
}

func TestHASummary_SessionTokenDoesNotWork(t *testing.T) {
	s, st := newTestServer(t)

	token := authToken(t, st, "admin", 10)
	if rec := getHASummary(t, s, "Bearer "+token); rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 (route takes only the read-only token)", rec.Code)
	}
}
