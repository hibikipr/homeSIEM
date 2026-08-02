package rules

import (
	"context"
	"fmt"
	"time"

	"github.com/hibikipr/homeSIEM/siem-api/internal/alerts"
	"github.com/hibikipr/homeSIEM/siem-api/internal/loki"
	"github.com/hibikipr/homeSIEM/siem-api/internal/store"
)

type SeenStore interface {
	HasSeenValue(ctx context.Context, ruleID int64, value string) (bool, error)
	MarkSeenValue(ctx context.Context, ruleID int64, value string, at time.Time) error
}

type FirstSeenEvaluator struct {
	Querier LokiQuerier
	Seen    SeenStore
}

func (e *FirstSeenEvaluator) Evaluate(ctx context.Context, rule store.Rule) ([]alerts.Candidate, error) {
	end := time.Now().UTC()
	start := end.Add(-time.Duration(rule.WindowSec) * time.Second)

	result, err := e.Querier.QueryRange(ctx, rule.LogQL, start, end, 5000)
	if err != nil {
		return nil, err
	}

	byValue := map[string][]loki.LogEntry{}
	for _, entry := range result.Entries {
		v := groupKeyFor(entry, rule.GroupBy)
		if v == "" {
			continue
		}
		byValue[v] = append(byValue[v], entry)
	}

	var candidates []alerts.Candidate
	for value, entries := range byValue {
		seen, err := e.Seen.HasSeenValue(ctx, rule.ID, value)
		if err != nil {
			return nil, err
		}
		if seen {
			continue
		}
		if err := e.Seen.MarkSeenValue(ctx, rule.ID, value, time.Now().UTC()); err != nil {
			return nil, err
		}

		candidates = append(candidates, alerts.Candidate{
			RuleID:   rule.ID,
			GroupKey: value,
			Severity: rule.Severity,
			Title:    fmt.Sprintf("%s: new value %q", rule.Name, value),
			Body:     fmt.Sprintf("%q was not observed before this window", value),
			Samples:  samplesFrom(entries),
		})
	}
	return candidates, nil
}
