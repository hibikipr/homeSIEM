package rules

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/hibikipr/homeSIEM/siem-api/internal/alerts"
	"github.com/hibikipr/homeSIEM/siem-api/internal/loki"
	"github.com/hibikipr/homeSIEM/siem-api/internal/sse"
	"github.com/hibikipr/homeSIEM/siem-api/internal/store"
)

func TestSchedulerEndToEnd_ThresholdRuleRaisesRealAlert(t *testing.T) {
	fakeLoki := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"success","data":{"result":[
			{"stream":{"job":"siem"},"values":[
				["1700000000000000000", "{\"src_ip\":\"10.0.0.5\"}"],
				["1700000001000000000", "{\"src_ip\":\"10.0.0.5\"}"],
				["1700000002000000000", "{\"src_ip\":\"10.0.0.5\"}"]
			]}
		]}}`))
	}))
	defer fakeLoki.Close()

	dbPath := filepath.Join(t.TempDir(), "siem.db")
	db, err := store.Open("sqlite://" + dbPath)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	if err := store.Migrate(db); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	st := store.New(db)

	threshold := 3
	rule, err := st.CreateRule(context.Background(), store.Rule{
		Name: "wan-portscan", Shape: "threshold", LogQL: `{job="siem"}`,
		WindowSec: 60, Threshold: &threshold, GroupBy: []string{"src_ip"},
		Severity: "critical", Destinations: []string{"inapp"},
		CooldownSec: 3600, IntervalSec: 1, Enabled: true,
	}, nil)
	if err != nil {
		t.Fatalf("CreateRule() error = %v", err)
	}

	hub := sse.NewHub()
	alertsSvc := alerts.NewService(st, hub, nil, "", schedulerTestLogger())
	lokiClient := loki.New(fakeLoki.URL, fakeLoki.Client())
	evaluators := map[string]Evaluator{"threshold": &ThresholdEvaluator{Querier: lokiClient}}
	scheduler := NewScheduler(st, evaluators, alertsSvc, schedulerTestLogger())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := scheduler.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer scheduler.Stop()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		openAlerts, err := st.ListAlerts(context.Background(), "open")
		if err != nil {
			t.Fatalf("ListAlerts() error = %v", err)
		}
		if len(openAlerts) > 0 {
			if openAlerts[0].RuleID != rule.ID {
				t.Fatalf("RuleID = %d, want %d", openAlerts[0].RuleID, rule.ID)
			}
			if openAlerts[0].GroupKey != "10.0.0.5" {
				t.Fatalf("GroupKey = %q, want 10.0.0.5", openAlerts[0].GroupKey)
			}
			if openAlerts[0].Severity != "critical" {
				t.Fatalf("Severity = %q, want critical", openAlerts[0].Severity)
			}
			return // success
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("timed out waiting for the scheduler to raise a real alert end-to-end")
}
