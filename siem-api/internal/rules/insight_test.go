package rules

import (
	"context"
	"testing"
	"time"

	"github.com/hibikipr/homeSIEM/siem-api/internal/store"
)

type fakeInsightStore struct {
	insights []store.Insight
	err      error

	// Captured for assertions on what the evaluator actually asked for.
	gotSince       time.Time
	gotMinSeverity string
}

func (f *fakeInsightStore) ListInsightsCreatedSince(_ context.Context, since time.Time, minSeverity string) ([]store.Insight, error) {
	f.gotSince = since
	f.gotMinSeverity = minSeverity
	return f.insights, f.err
}

func TestInsightEvaluator_OneCandidatePerInsight(t *testing.T) {
	insights := &fakeInsightStore{insights: []store.Insight{
		{ID: 1, Title: "UI-poller errors", Detail: "repeated failures", Severity: "critical", Category: "operational", Fingerprint: "fp-1"},
		{ID: 2, Title: "Unauthorized API access", Detail: "33 attempts", Severity: "warning", Category: "security", Fingerprint: "fp-2"},
	}}
	e := &InsightEvaluator{Store: insights}

	rule := store.Rule{ID: 7, Severity: "warning", CreatedAt: time.Now().UTC()}
	candidates, err := e.Evaluate(context.Background(), rule)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if len(candidates) != 2 {
		t.Fatalf("len(candidates) = %d, want 2", len(candidates))
	}

	for i, in := range insights.insights {
		c := candidates[i]
		if c.RuleID != 7 {
			t.Errorf("candidates[%d].RuleID = %d, want 7", i, c.RuleID)
		}
		if c.GroupKey != in.Fingerprint {
			t.Errorf("candidates[%d].GroupKey = %q, want the insight's fingerprint %q", i, c.GroupKey, in.Fingerprint)
		}
		// The insight's own severity, not the rule's configured floor -
		// see InsightEvaluator's doc for why.
		if c.Severity != in.Severity {
			t.Errorf("candidates[%d].Severity = %q, want the insight's own severity %q", i, c.Severity, in.Severity)
		}
		if c.Context["insight_id"] != in.ID {
			t.Errorf("candidates[%d].Context[insight_id] = %v, want %d", i, c.Context["insight_id"], in.ID)
		}
	}
}

func TestInsightEvaluator_NoInsightsNoCandidates(t *testing.T) {
	e := &InsightEvaluator{Store: &fakeInsightStore{insights: nil}}

	candidates, err := e.Evaluate(context.Background(), store.Rule{ID: 1, CreatedAt: time.Now().UTC()})
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if len(candidates) != 0 {
		t.Fatalf("len(candidates) = %d, want 0", len(candidates))
	}
}

func TestInsightEvaluator_UsesLastRunAtWhenSet(t *testing.T) {
	lastRun := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	insights := &fakeInsightStore{}
	e := &InsightEvaluator{Store: insights}

	rule := store.Rule{ID: 1, Severity: "info", CreatedAt: time.Now().UTC().Add(-24 * time.Hour), LastRunAt: &lastRun}
	if _, err := e.Evaluate(context.Background(), rule); err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if !insights.gotSince.Equal(lastRun) {
		t.Errorf("gotSince = %v, want LastRunAt %v", insights.gotSince, lastRun)
	}
	if insights.gotMinSeverity != "info" {
		t.Errorf("gotMinSeverity = %q, want %q", insights.gotMinSeverity, "info")
	}
}

func TestInsightEvaluator_FallsBackToCreatedAtWhenNeverRun(t *testing.T) {
	createdAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	insights := &fakeInsightStore{}
	e := &InsightEvaluator{Store: insights}

	// LastRunAt nil - a rule that has never fired yet, so there's no
	// watermark to resume from. Falling back to CreatedAt (rather than,
	// say, the zero time) avoids surfacing every insight ever generated
	// the moment a user enables this rule for the first time.
	rule := store.Rule{ID: 1, Severity: "warning", CreatedAt: createdAt, LastRunAt: nil}
	if _, err := e.Evaluate(context.Background(), rule); err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if !insights.gotSince.Equal(createdAt) {
		t.Errorf("gotSince = %v, want CreatedAt %v", insights.gotSince, createdAt)
	}
}
