package rules

import (
	"context"
	"testing"
	"time"

	"github.com/hibikipr/homeSIEM/siem-api/internal/loki"
	"github.com/hibikipr/homeSIEM/siem-api/internal/store"
)

type fakeSeenStore struct {
	seen  map[string]bool
	marks []string
}

func newFakeSeenStore(alreadySeen ...string) *fakeSeenStore {
	s := &fakeSeenStore{seen: map[string]bool{}}
	for _, v := range alreadySeen {
		s.seen[v] = true
	}
	return s
}

func (f *fakeSeenStore) HasSeenValue(ctx context.Context, ruleID int64, value string) (bool, error) {
	return f.seen[value], nil
}

func (f *fakeSeenStore) MarkSeenValue(ctx context.Context, ruleID int64, value string, at time.Time) error {
	f.marks = append(f.marks, value)
	f.seen[value] = true
	return nil
}

func TestFirstSeenEvaluator_NewValueFires(t *testing.T) {
	now := time.Now().UTC()
	querier := &fakeLokiQuerier{result: loki.QueryResult{Entries: []loki.LogEntry{
		{Timestamp: now, Line: `{"domain":"new-domain.example"}`},
	}}}
	seen := newFakeSeenStore() // nothing seen yet
	e := &FirstSeenEvaluator{Querier: querier, Seen: seen}

	rule := store.Rule{ID: 1, Name: "new-domain-burst", LogQL: `{job="siem"}`, WindowSec: 3600,
		GroupBy: []string{"domain"}, Severity: "warning"}

	candidates, err := e.Evaluate(context.Background(), rule)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if len(candidates) != 1 || candidates[0].GroupKey != "new-domain.example" {
		t.Fatalf("candidates = %+v, want one for new-domain.example", candidates)
	}
	if len(seen.marks) != 1 || seen.marks[0] != "new-domain.example" {
		t.Errorf("marks = %v, want [new-domain.example]", seen.marks)
	}
}

func TestFirstSeenEvaluator_AlreadySeenNoFire(t *testing.T) {
	now := time.Now().UTC()
	querier := &fakeLokiQuerier{result: loki.QueryResult{Entries: []loki.LogEntry{
		{Timestamp: now, Line: `{"domain":"known-domain.example"}`},
	}}}
	seen := newFakeSeenStore("known-domain.example")
	e := &FirstSeenEvaluator{Querier: querier, Seen: seen}

	rule := store.Rule{ID: 1, LogQL: `{job="siem"}`, WindowSec: 3600, GroupBy: []string{"domain"}}

	candidates, err := e.Evaluate(context.Background(), rule)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if len(candidates) != 0 {
		t.Fatalf("candidates = %+v, want none for an already-seen value", candidates)
	}
	if len(seen.marks) != 0 {
		t.Errorf("marks = %v, want none — MarkSeenValue must not be called for already-seen values", seen.marks)
	}
}

func TestFirstSeenEvaluator_DedupesWithinBatch(t *testing.T) {
	now := time.Now().UTC()
	querier := &fakeLokiQuerier{result: loki.QueryResult{Entries: []loki.LogEntry{
		{Timestamp: now, Line: `{"domain":"dup.example"}`},
		{Timestamp: now, Line: `{"domain":"dup.example"}`},
	}}}
	seen := newFakeSeenStore()
	e := &FirstSeenEvaluator{Querier: querier, Seen: seen}

	rule := store.Rule{ID: 1, LogQL: `{job="siem"}`, WindowSec: 3600, GroupBy: []string{"domain"}}

	candidates, err := e.Evaluate(context.Background(), rule)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("len(candidates) = %d, want 1 (two entries, same new value)", len(candidates))
	}
}
