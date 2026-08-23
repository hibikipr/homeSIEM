package rules

import (
	"context"
	"fmt"
	"sort"
	"strings"
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

// displayName prefers an operator-set Sources rename over the raw
// syslog-derived name, same convention as everywhere else a source name
// reaches a human (see AlertDetail's resolvedSourceName, search.ts's
// mergeSourceFacet).
func displayName(src store.Source) string {
	if src.DisplayName != "" {
		return src.DisplayName
	}
	return src.Name
}

func (e *AbsenceEvaluator) Evaluate(ctx context.Context, rule store.Rule) ([]alerts.Candidate, error) {
	stale, err := e.Sources.StaleSources(ctx, time.Now().UTC())
	if err != nil {
		return nil, err
	}

	if candidate, ok := correlatedCandidate(rule, stale); ok {
		return []alerts.Candidate{candidate}, nil
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
			Title:    fmt.Sprintf("%s: source %q has gone silent", rule.Name, displayName(src)),
			Body:     fmt.Sprintf("no events from %q since %s (heartbeat %ds)", displayName(src), lastSeen, src.HeartbeatSec),
		})
	}
	return candidates, nil
}

// correlatedCandidate returns a single alert covering every currently-stale
// source, instead of one nearly-identical card per source, when there are
// at least two AND their last-seen timestamps all fall within rule.WindowSec
// of each other. Multiple unrelated devices going quiet within a short
// window of each other is far more likely to share one cause - the
// collector, a network segment, a router - than to be independent
// coincidences; three source-quiet alerts firing within a few hours of each
// other overnight (a real production case that prompted this) reads very
// differently as three symptoms of one outage than as three unrelated
// device failures.
//
// rule.WindowSec is otherwise unused by this rule shape (this evaluator
// only ever reads a source's own HeartbeatSec to decide staleness, never
// the rule's), so it's repurposed here as the correlation window rather
// than left vestigial - RuleForm/RuleDetail label it accordingly for the
// absence shape specifically.
//
// ok is false (callers fall back to one individual alert per source,
// unchanged from before this existed) when fewer than two sources are
// stale, or when they're stale for reasons spread too far apart in time to
// plausibly share a cause. A source that has *never* been seen at all
// (LastSeenAt nil - no timestamp to compare) doesn't block correlation of
// the ones that do have one; it's folded into whichever group forms.
func correlatedCandidate(rule store.Rule, stale []store.Source) (alerts.Candidate, bool) {
	if len(stale) < 2 {
		return alerts.Candidate{}, false
	}

	window := time.Duration(rule.WindowSec) * time.Second
	var earliest, latest *time.Time
	for _, src := range stale {
		if src.LastSeenAt == nil {
			continue
		}
		if earliest == nil || src.LastSeenAt.Before(*earliest) {
			earliest = src.LastSeenAt
		}
		if latest == nil || src.LastSeenAt.After(*latest) {
			latest = src.LastSeenAt
		}
	}
	if earliest != nil && latest != nil && latest.Sub(*earliest) > window {
		return alerts.Candidate{}, false
	}

	groupNames := make([]string, 0, len(stale))
	lines := make([]string, 0, len(stale))
	for _, src := range stale {
		groupNames = append(groupNames, src.Name)
		lastSeen := "never"
		if src.LastSeenAt != nil {
			lastSeen = src.LastSeenAt.Format(time.RFC3339)
		}
		lines = append(lines, fmt.Sprintf("- %q: last seen %s (heartbeat %ds)", displayName(src), lastSeen, src.HeartbeatSec))
	}
	// Sorted so the same set of affected sources always produces the same
	// GroupKey regardless of StaleSources' return order - this is what
	// lets a recurrence of the exact same outage reuse the same alert row
	// (touched/reopened via the existing cooldown logic) instead of
	// spawning a fresh one every evaluation pass, same principle as every
	// other rule shape's GroupKey.
	sort.Strings(groupNames)

	return alerts.Candidate{
		RuleID:   rule.ID,
		GroupKey: "multi:" + strings.Join(groupNames, ","),
		Severity: rule.Severity,
		Title:    fmt.Sprintf("%s: %d sources went silent together", rule.Name, len(stale)),
		Body: fmt.Sprintf(
			"%d sources stopped sending events within %s of each other - more likely one shared cause (the collector, a network segment, a router) than independent coincidences:\n%s",
			len(stale), window, strings.Join(lines, "\n"),
		),
	}, true
}
