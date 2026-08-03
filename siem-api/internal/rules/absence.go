package rules

import (
	"context"
	"fmt"
	"time"

	"github.com/hibikipr/homeSIEM/siem-api/internal/alerts"
	"github.com/hibikipr/homeSIEM/siem-api/internal/store"
)

type SourcesStore interface {
	StaleSources(ctx context.Context, now time.Time) ([]store.Source, error)
}

type AbsenceEvaluator struct {
	Sources SourcesStore
}

func (e *AbsenceEvaluator) Evaluate(ctx context.Context, rule store.Rule) ([]alerts.Candidate, error) {
	stale, err := e.Sources.StaleSources(ctx, time.Now().UTC())
	if err != nil {
		return nil, err
	}

	var candidates []alerts.Candidate
	for _, src := range stale {
		lastSeen := "never"
		if src.LastSeenAt != nil {
			lastSeen = src.LastSeenAt.Format(time.RFC3339)
		}
		candidates = append(candidates, alerts.Candidate{
			RuleID:   rule.ID,
			GroupKey: src.Name,
			Severity: rule.Severity,
			Title:    fmt.Sprintf("%s: source %q has gone silent", rule.Name, src.Name),
			Body:     fmt.Sprintf("no events from %q since %s (heartbeat %ds)", src.Name, lastSeen, src.HeartbeatSec),
		})
	}
	return candidates, nil
}
