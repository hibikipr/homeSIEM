package rules

import (
	"context"
	"testing"
	"time"

	"github.com/hibikipr/homeSIEM/siem-api/internal/store"
)

type fakeSourcesStore struct {
	stale []store.Source
	err   error
}

func (f *fakeSourcesStore) StaleSources(ctx context.Context, now time.Time) ([]store.Source, error) {
	return f.stale, f.err
}

func TestAbsenceEvaluator_OneCandidatePerStaleSource(t *testing.T) {
	lastSeen := time.Now().UTC().Add(-2 * time.Hour)
	sources := &fakeSourcesStore{stale: []store.Source{
		{Name: "silent-host", HeartbeatSec: 900, LastSeenAt: &lastSeen},
		{Name: "another-host", HeartbeatSec: 60, LastSeenAt: nil},
	}}
	e := &AbsenceEvaluator{Sources: sources}

	rule := store.Rule{ID: 1, Name: "source-heartbeat", Severity: "warning"}
	candidates, err := e.Evaluate(context.Background(), rule)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if len(candidates) != 2 {
		t.Fatalf("len(candidates) = %d, want 2", len(candidates))
	}

	var groupKeys []string
	for _, c := range candidates {
		groupKeys = append(groupKeys, c.GroupKey)
		if c.RuleID != 1 || c.Severity != "warning" {
			t.Errorf("candidate = %+v, want RuleID=1 Severity=warning", c)
		}
	}
	if groupKeys[0] != "silent-host" && groupKeys[1] != "silent-host" {
		t.Errorf("groupKeys = %v, want to contain silent-host", groupKeys)
	}
}

func TestAbsenceEvaluator_NoneStaleNoCandidates(t *testing.T) {
	sources := &fakeSourcesStore{stale: nil}
	e := &AbsenceEvaluator{Sources: sources}

	candidates, err := e.Evaluate(context.Background(), store.Rule{ID: 1})
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if len(candidates) != 0 {
		t.Fatalf("len(candidates) = %d, want 0", len(candidates))
	}
}
