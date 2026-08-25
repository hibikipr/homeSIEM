package rules

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/hibikipr/homeSIEM/siem-api/internal/alerts"
	"github.com/hibikipr/homeSIEM/siem-api/internal/loki"
	"github.com/hibikipr/homeSIEM/siem-api/internal/store"
)

type SourcesStore interface {
	StaleSources(ctx context.Context, now time.Time) ([]store.Source, error)
	// TouchSourceLastSeen lets reconcileWithLoki correct a source row's
	// last_seen_at from a real Loki event it found - so Sources/AlertDetail
	// stop showing a stale timestamp too, not just this one evaluation
	// pass. Same method the /sources/heartbeat endpoint itself uses.
	TouchSourceLastSeen(ctx context.Context, name string, at time.Time) error
}

type AbsenceEvaluator struct {
	Sources SourcesStore
	// Querier and JobLabel cross-check a source against Loki directly
	// before trusting sources.last_seen_at - see reconcileWithLoki's own
	// doc comment for why. Left nil in any test that doesn't care about
	// this (every existing one before this field existed), reconciliation
	// is skipped and StaleSources' verdict is trusted as-is, matching
	// this evaluator's original behavior.
	Querier  LokiQuerier
	JobLabel string
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

// sourceContext is the JSON shape AlertDetail reads (via alert.context) to
// render absence-shaped alerts - last-seen/heartbeat/overdue-by, rather than
// the network-alert stat tiles (matched events, ports, source IP) that a
// threshold rule's Context doesn't apply here. Always "sources": [...],
// even for a single-source alert, so the frontend has one shape to handle
// instead of branching on whether this was a correlated alert.
type sourceContext struct {
	Name         string  `json:"name"`
	DisplayName  string  `json:"display_name"`
	LastSeenAt   *string `json:"last_seen_at"`
	HeartbeatSec int     `json:"heartbeat_sec"`
}

func toSourceContext(src store.Source) sourceContext {
	sc := sourceContext{Name: src.Name, DisplayName: displayName(src), HeartbeatSec: src.HeartbeatSec}
	if src.LastSeenAt != nil {
		ts := src.LastSeenAt.Format(time.RFC3339)
		sc.LastSeenAt = &ts
	}
	return sc
}

func (e *AbsenceEvaluator) Evaluate(ctx context.Context, rule store.Rule) ([]alerts.Candidate, error) {
	stale, err := e.Sources.StaleSources(ctx, time.Now().UTC())
	if err != nil {
		return nil, err
	}

	stale, err = e.reconcileWithLoki(ctx, stale)
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
			Context:  map[string]any{"sources": []sourceContext{toSourceContext(src)}},
		})
	}
	return candidates, nil
}

// reconcileWithLoki cross-checks each candidate-stale source against Loki
// directly before trusting StaleSources' verdict. sources.last_seen_at only
// ever updates via siem-ingest's heartbeat_throttle, which allows at most
// one /sources/heartbeat ping per 900s per source - a rate limiter, not a
// guarantee of exactly-once-per-window delivery. A single missed or delayed
// ping (a siem-api restart landing mid-window, a brief network blip between
// siem-ingest and siem-api) pushes the recorded gap past heartbeat_sec even
// for a source that never actually stopped sending real events, producing a
// false "gone silent" alert while the source's own event stream in Loki
// never had a gap at all - confirmed directly against a real production
// case this way (Loki showed continuous per-minute activity for a source
// with three separate false alarms the same day).
//
// Ground truth is Loki's own event stream, not the throttled heartbeat row,
// so ask it directly: if a real event exists for this source within its own
// heartbeat_sec window, it is not silent, full stop - regardless of what
// the DB row says. Self-heals the DB row from that real timestamp too (via
// TouchSourceLastSeen), so Sources/AlertDetail stop showing a stale
// last_seen_at as a side effect of this check, not just this evaluation
// pass, and don't need a genuinely fresh heartbeat ping to catch back up.
func (e *AbsenceEvaluator) reconcileWithLoki(ctx context.Context, stale []store.Source) ([]store.Source, error) {
	if e.Querier == nil || len(stale) == 0 {
		return stale, nil
	}

	now := time.Now().UTC()
	out := make([]store.Source, 0, len(stale))
	for _, src := range stale {
		window := time.Duration(src.HeartbeatSec) * time.Second
		logql := loki.BuildQuery(e.JobLabel, loki.Filters{Source: src.Name})

		result, err := e.Querier.QueryRange(ctx, logql, now.Add(-window), now, 1)
		if err != nil {
			// Loki being unreachable must not block absence detection
			// entirely - that would let a real Loki outage silently mask
			// every other source's outage too. Fall back to trusting
			// StaleSources' verdict for this source, same as before this
			// check existed.
			out = append(out, src)
			continue
		}
		if len(result.Entries) == 0 {
			out = append(out, src) // genuinely stale - no recent real events either
			continue
		}

		// A real event exists inside the heartbeat window - not silent,
		// regardless of what last_seen_at says. Correct it and drop this
		// source from the stale set instead of appending.
		latest := result.Entries[0].Timestamp
		if err := e.Sources.TouchSourceLastSeen(ctx, src.Name, latest); err != nil {
			return nil, err
		}
	}
	return out, nil
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
	sourceContexts := make([]sourceContext, 0, len(stale))
	for _, src := range stale {
		groupNames = append(groupNames, src.Name)
		lastSeen := "never"
		if src.LastSeenAt != nil {
			lastSeen = src.LastSeenAt.Format(time.RFC3339)
		}
		lines = append(lines, fmt.Sprintf("- %q: last seen %s (heartbeat %ds)", displayName(src), lastSeen, src.HeartbeatSec))
		sourceContexts = append(sourceContexts, toSourceContext(src))
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
		Context: map[string]any{"sources": sourceContexts},
	}, true
}
