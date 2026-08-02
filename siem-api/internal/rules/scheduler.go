package rules

import (
	"context"
	"log/slog"
	"math/rand"
	"sync"
	"time"

	"github.com/hibikipr/homeSIEM/siem-api/internal/alerts"
	"github.com/hibikipr/homeSIEM/siem-api/internal/store"
)

type RulesStore interface {
	ListEnabledRules(ctx context.Context) ([]store.Rule, error)
	TouchRuleLastRun(ctx context.Context, id int64, at time.Time) error
}

type Raiser interface {
	Raise(ctx context.Context, c alerts.Candidate) error
}

type Scheduler struct {
	rulesStore RulesStore
	evaluators map[string]Evaluator
	raiser     Raiser
	logger     *slog.Logger

	mu      sync.Mutex
	cancels map[int64]context.CancelFunc
	wg      sync.WaitGroup
}

func NewScheduler(rulesStore RulesStore, evaluators map[string]Evaluator, raiser Raiser, logger *slog.Logger) *Scheduler {
	return &Scheduler{
		rulesStore: rulesStore,
		evaluators: evaluators,
		raiser:     raiser,
		logger:     logger,
		cancels:    make(map[int64]context.CancelFunc),
	}
}

func (s *Scheduler) Start(ctx context.Context) error {
	rules, err := s.rulesStore.ListEnabledRules(ctx)
	if err != nil {
		return err
	}
	for _, rule := range rules {
		s.StartRule(ctx, rule)
	}
	return nil
}

func (s *Scheduler) StartRule(ctx context.Context, rule store.Rule) {
	s.StopRule(rule.ID) // replace any existing goroutine for this rule

	ruleCtx, cancel := context.WithCancel(ctx)

	s.mu.Lock()
	s.cancels[rule.ID] = cancel
	s.mu.Unlock()

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.runRuleLoop(ruleCtx, rule)
	}()
}

func (s *Scheduler) StopRule(ruleID int64) {
	s.mu.Lock()
	cancel, ok := s.cancels[ruleID]
	delete(s.cancels, ruleID)
	s.mu.Unlock()

	if ok {
		cancel()
	}
}

func (s *Scheduler) Stop() {
	s.mu.Lock()
	for id, cancel := range s.cancels {
		cancel()
		delete(s.cancels, id)
	}
	s.mu.Unlock()

	s.wg.Wait()
}

func (s *Scheduler) runRuleLoop(ctx context.Context, rule store.Rule) {
	intervalSec := rule.IntervalSec
	if intervalSec <= 0 {
		intervalSec = 60
	}
	interval := time.Duration(intervalSec) * time.Second

	jitter := time.Duration(rand.Int63n(int64(interval) + 1))
	select {
	case <-ctx.Done():
		return
	case <-time.After(jitter):
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	s.evaluateOnce(ctx, rule)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.evaluateOnce(ctx, rule)
		}
	}
}

func (s *Scheduler) evaluateOnce(ctx context.Context, rule store.Rule) {
	evaluator, ok := s.evaluators[rule.Shape]
	if !ok {
		s.logger.Error("no evaluator registered for rule shape", "rule", rule.Name, "shape", rule.Shape)
		return
	}

	candidates, err := evaluator.Evaluate(ctx, rule)
	if err != nil {
		s.logger.Error("rule evaluation failed", "rule", rule.Name, "error", err)
		return
	}

	for _, c := range candidates {
		if err := s.raiser.Raise(ctx, c); err != nil {
			s.logger.Error("raise failed", "rule", rule.Name, "error", err)
		}
	}

	if err := s.rulesStore.TouchRuleLastRun(ctx, rule.ID, time.Now().UTC()); err != nil {
		s.logger.Error("touch rule last run failed", "rule", rule.Name, "error", err)
	}
}
