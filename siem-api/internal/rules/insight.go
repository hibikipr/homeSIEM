package rules

import (
	"context"
	"fmt"
	"time"

	"github.com/hibikipr/homeSIEM/siem-api/internal/alerts"
	"github.com/hibikipr/homeSIEM/siem-api/internal/store"
)

// InsightStore is the narrow slice of store.Store InsightEvaluator needs -
// named distinctly from insights.InsightStore (that package's own
// dependency for generating insights in the first place; this one only
// ever reads what's already been generated).
type InsightStore interface {
	ListInsightsCreatedSince(ctx context.Context, since time.Time, minSeverity string) ([]store.Insight, error)
}

// InsightEvaluator bridges the LLM-driven Insights pipeline into the same
// rule/alert/ntfy path every other rule shape uses. Insights and Alerts are
// deliberately kept as separate systems with separate data models
// (Insights: fuzzy, LLM-judged, occurrence-counted, its own dismiss/mute -
// Alerts: deterministic, rule-driven, with a real open/acked/resolved
// lifecycle) - this is the one narrow bridge between them, letting a user
// opt in to "notify me when Ollama finds something at or above severity X"
// the same way they already opt in to a source going quiet or a new value
// appearing, rather than either merging the two data models or hardwiring
// every insight straight to ntfy with no way to turn it off or tune it.
//
// Only ever looks at insights *first created* since this rule last ran
// (store.ListInsightsCreatedSince deliberately excludes bumps of an
// already-existing fingerprint) - see that method's own doc for why
// notifying on every recurrence, not just the first sighting, would turn
// this into exactly the kind of notification spam it exists to avoid.
type InsightEvaluator struct {
	Store InsightStore
}

func (e *InsightEvaluator) Evaluate(ctx context.Context, rule store.Rule) ([]alerts.Candidate, error) {
	since := rule.CreatedAt
	if rule.LastRunAt != nil {
		since = *rule.LastRunAt
	}

	found, err := e.Store.ListInsightsCreatedSince(ctx, since, rule.Severity)
	if err != nil {
		return nil, err
	}

	candidates := make([]alerts.Candidate, 0, len(found))
	for _, in := range found {
		candidates = append(candidates, alerts.Candidate{
			RuleID:   rule.ID,
			GroupKey: in.Fingerprint,
			// The insight's own severity, not the rule's configured
			// minimum - rule.Severity is a floor ("notify me at warning or
			// above"), and a critical-severity insight clearing a
			// warning-level floor should still show as critical in Alerts,
			// not be downgraded to whatever the floor happened to be.
			Severity: in.Severity,
			Title:    fmt.Sprintf("Insight: %s", in.Title),
			Body:     in.Detail,
			Context: map[string]any{
				"insight_id": in.ID,
				"category":   in.Category,
			},
		})
	}
	return candidates, nil
}
