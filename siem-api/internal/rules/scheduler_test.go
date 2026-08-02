package rules

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/hibikipr/homeSIEM/siem-api/internal/alerts"
	"github.com/hibikipr/homeSIEM/siem-api/internal/store"
)

func schedulerTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

type fakeEvaluator struct {
	mu    sync.Mutex
	calls int
	out   []alerts.Candidate
}

func (f *fakeEvaluator) Evaluate(ctx context.Context, rule store.Rule) ([]alerts.Candidate, error) {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	return f.out, nil
}

func (f *fakeEvaluator) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

type fakeRaiser struct {
	ch chan alerts.Candidate
}

func newFakeRaiser() *fakeRaiser {
	return &fakeRaiser{ch: make(chan alerts.Candidate, 10)}
}

func (f *fakeRaiser) Raise(ctx context.Context, c alerts.Candidate) error {
	f.ch <- c
	return nil
}

type fakeRulesStore struct {
	enabled []store.Rule
	touchCh chan int64
}

func newFakeRulesStore(rules ...store.Rule) *fakeRulesStore {
	return &fakeRulesStore{enabled: rules, touchCh: make(chan int64, 10)}
}

func (f *fakeRulesStore) ListEnabledRules(ctx context.Context) ([]store.Rule, error) {
	return f.enabled, nil
}

func (f *fakeRulesStore) TouchRuleLastRun(ctx context.Context, id int64, at time.Time) error {
	f.touchCh <- id
	return nil
}

func TestScheduler_StartRule_EvaluatesAndRaises(t *testing.T) {
	evaluator := &fakeEvaluator{out: []alerts.Candidate{{RuleID: 1, GroupKey: "g"}}}
	raiser := newFakeRaiser()
	rulesStore := newFakeRulesStore()
	s := NewScheduler(rulesStore, map[string]Evaluator{"threshold": evaluator}, raiser, schedulerTestLogger())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.StartRule(ctx, store.Rule{ID: 1, Shape: "threshold", IntervalSec: 1})
	defer s.Stop()

	select {
	case c := <-raiser.ch:
		if c.RuleID != 1 {
			t.Errorf("Raise() candidate RuleID = %d, want 1", c.RuleID)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for scheduler to evaluate and raise")
	}
}

func TestScheduler_TouchesRuleLastRun(t *testing.T) {
	evaluator := &fakeEvaluator{}
	raiser := newFakeRaiser()
	rulesStore := newFakeRulesStore()
	s := NewScheduler(rulesStore, map[string]Evaluator{"absence": evaluator}, raiser, schedulerTestLogger())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.StartRule(ctx, store.Rule{ID: 5, Shape: "absence", IntervalSec: 1})
	defer s.Stop()

	select {
	case id := <-rulesStore.touchCh:
		if id != 5 {
			t.Errorf("TouchRuleLastRun id = %d, want 5", id)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for TouchRuleLastRun")
	}
}

func TestScheduler_StopRule_StopsFurtherEvaluation(t *testing.T) {
	evaluator := &fakeEvaluator{}
	raiser := newFakeRaiser()
	rulesStore := newFakeRulesStore()
	s := NewScheduler(rulesStore, map[string]Evaluator{"absence": evaluator}, raiser, schedulerTestLogger())

	s.StartRule(context.Background(), store.Rule{ID: 9, Shape: "absence", IntervalSec: 1})

	select {
	case <-rulesStore.touchCh:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for first evaluation")
	}

	s.StopRule(9)
	countAfterStop := evaluator.callCount()

	time.Sleep(1500 * time.Millisecond)
	if evaluator.callCount() > countAfterStop {
		t.Errorf("evaluator called again after StopRule: count went from %d to %d", countAfterStop, evaluator.callCount())
	}
}

func TestScheduler_Start_LoadsEnabledRulesFromStore(t *testing.T) {
	evaluator := &fakeEvaluator{}
	raiser := newFakeRaiser()
	rulesStore := newFakeRulesStore(
		store.Rule{ID: 1, Shape: "absence", IntervalSec: 1},
		store.Rule{ID: 2, Shape: "absence", IntervalSec: 1},
	)
	s := NewScheduler(rulesStore, map[string]Evaluator{"absence": evaluator}, raiser, schedulerTestLogger())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := s.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer s.Stop()

	seen := map[int64]bool{}
	deadline := time.After(3 * time.Second)
	for len(seen) < 2 {
		select {
		case id := <-rulesStore.touchCh:
			seen[id] = true
		case <-deadline:
			t.Fatalf("timed out waiting for both rules to evaluate, saw %v", seen)
		}
	}
}
