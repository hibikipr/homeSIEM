// Package insights automates the manual severity-audit workflow used
// throughout this project's history (query err/warning grouped by program,
// dedupe by message shape, reason about what's genuine vs misclassified)
// into a scheduled + on-demand pass against a locally-hosted LLM. See
// docs/superpowers/specs/2026-08-10-siem-insights-llm-design.md.
package insights

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/hibikipr/homeSIEM/siem-api/internal/loki"
	"github.com/hibikipr/homeSIEM/siem-api/internal/store"
)

const sampleCap = 50
const messagePrefixLen = 60

type LokiQuerier interface {
	QueryRange(ctx context.Context, logql string, start, end time.Time, limit int) (loki.QueryResult, error)
	QueryInstant(ctx context.Context, logql string, at time.Time) (loki.MatrixResult, error)
}

type AlertLister interface {
	ListAlerts(ctx context.Context, state string) ([]store.Alert, error)
}

type PromptBuilder struct {
	Loki     LokiQuerier
	Alerts   AlertLister
	JobLabel string
}

// extractMessage pulls the human-readable .message field out of the raw
// JSON siem-ingest's enrich_geo transform stores as the Loki line (the
// entire enriched event, not just the text a user actually wrote) -
// same shape as siem-web's extractMessage (logline.ts), same best-effort
// fallback pattern already used elsewhere in this package
// (rules/threshold.go's groupKeyFor).
func extractMessage(line string) string {
	var fields map[string]any
	if err := json.Unmarshal([]byte(line), &fields); err != nil {
		return line
	}
	if msg, ok := fields["message"].(string); ok && msg != "" {
		return msg
	}
	return line
}

type sampleGroup struct {
	Program       string
	SampleMessage string
	Count         int
}

// dedupeSamples groups entries by (program, first messagePrefixLen chars of
// message) and returns the capCount most frequent groups, most frequent
// first. This is the piece that matters most in practice - see the design
// doc's Data flow section: raw volume is almost always repeats of a small
// number of distinct patterns, and what's actually informative to a
// reviewer (human or model) is the distinct shapes, not the raw count.
func dedupeSamples(entries []loki.LogEntry, capCount int) []sampleGroup {
	type key struct{ program, prefix string }
	counts := map[key]int{}
	samples := map[key]string{}
	var order []key

	for _, e := range entries {
		program := e.Labels["program"]
		msg := extractMessage(e.Line)
		prefix := msg
		if len(prefix) > messagePrefixLen {
			prefix = prefix[:messagePrefixLen]
		}
		k := key{program, prefix}
		if _, seen := counts[k]; !seen {
			order = append(order, k)
			samples[k] = msg
		}
		counts[k]++
	}

	groups := make([]sampleGroup, 0, len(order))
	for _, k := range order {
		groups = append(groups, sampleGroup{Program: k.program, SampleMessage: samples[k], Count: counts[k]})
	}
	sort.SliceStable(groups, func(i, j int) bool { return groups[i].Count > groups[j].Count })
	if len(groups) > capCount {
		groups = groups[:capCount]
	}
	return groups
}

type rollupRow struct {
	Program   string
	ErrCount  float64
	WarnCount float64
}

func (b *PromptBuilder) buildRollup(ctx context.Context, lookback time.Duration, at time.Time) ([]rollupRow, error) {
	byProgram := map[string]*rollupRow{}
	get := func(program string) *rollupRow {
		row, ok := byProgram[program]
		if !ok {
			row = &rollupRow{Program: program}
			byProgram[program] = row
		}
		return row
	}

	window := fmt.Sprintf("%dm", int(lookback.Minutes()))
	for _, sev := range []string{"err", "warning"} {
		logql := fmt.Sprintf(`sum by (program) (count_over_time({job=%q, severity=%q}[%s]))`, b.JobLabel, sev, window)
		result, err := b.Loki.QueryInstant(ctx, logql, at)
		if err != nil {
			return nil, fmt.Errorf("insights: severity rollup query (%s): %w", sev, err)
		}
		for _, series := range result.Series {
			var count float64
			if len(series.Samples) > 0 {
				count = series.Samples[len(series.Samples)-1].Value
			}
			row := get(series.Labels["program"])
			if sev == "err" {
				row.ErrCount = count
			} else {
				row.WarnCount = count
			}
		}
	}

	rows := make([]rollupRow, 0, len(byProgram))
	for _, row := range byProgram {
		rows = append(rows, *row)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].ErrCount+rows[i].WarnCount > rows[j].ErrCount+rows[j].WarnCount })
	return rows, nil
}

func formatAlertsSummary(alerts []store.Alert) string {
	var b strings.Builder
	b.WriteString("## Open alerts\n")
	if len(alerts) == 0 {
		b.WriteString("None.\n")
		return b.String()
	}
	for _, a := range alerts {
		fmt.Fprintf(&b, "- [%s] %s (first seen %s, last seen %s)\n",
			a.Severity, a.Title, a.FirstSeenAt.Format(time.RFC3339), a.LastSeenAt.Format(time.RFC3339))
	}
	return b.String()
}

func formatRollup(rows []rollupRow) string {
	var b strings.Builder
	b.WriteString("## Severity by program\n")
	if len(rows) == 0 {
		b.WriteString("No err/warning activity in the lookback window.\n")
		return b.String()
	}
	for _, r := range rows {
		fmt.Fprintf(&b, "- %s: %.0f err, %.0f warning\n", r.Program, r.ErrCount, r.WarnCount)
	}
	return b.String()
}

func formatSamples(groups []sampleGroup) string {
	var b strings.Builder
	b.WriteString("## Recent error/warning samples (deduplicated, most frequent first)\n")
	if len(groups) == 0 {
		b.WriteString("None.\n")
		return b.String()
	}
	for _, g := range groups {
		fmt.Fprintf(&b, "- [%s] (%d occurrences) %s\n", g.Program, g.Count, g.SampleMessage)
	}
	return b.String()
}

const systemPromptText = `You are reviewing recent activity in a homelab SIEM (security/log
monitoring) system. You will be given: a list of open alerts, a rollup of
error/warning counts by program, and a deduplicated sample of recent
error/warning log lines.

Your job is to find genuinely actionable patterns - things like: a
program's errors that look like they might be mistagged (routine activity
landing as an error), a repeated failure that suggests something needs
attention, or a security-relevant pattern worth a human's review.

Rules:
- Only reference programs, counts, and messages that actually appear in
  the data you were given. Never invent or assume anything beyond it.
- If nothing in the data looks actionable, return an empty JSON array - do
  not manufacture a suggestion just to have something to say.
- Respond with ONLY a JSON array (no prose, no markdown code fence)
  matching exactly this shape:
[
  {
    "title": "one line",
    "detail": "a few sentences",
    "severity": "info|warning|critical",
    "category": "severity-misclassification|operational|security|other",
    "evidence": [{"program": "string", "sample_message": "string", "count": 0}]
  }
]`

// Build gathers the three data sources (open alerts, severity×program
// rollup, deduplicated err/warning samples) and assembles the fixed system
// prompt plus a user prompt from them. See the design doc's Data flow
// section for why this is aggregated/deduplicated data, never raw log
// lines.
func (b *PromptBuilder) Build(ctx context.Context, lookback time.Duration) (systemPrompt, userPrompt string, err error) {
	end := time.Now().UTC()
	start := end.Add(-lookback)

	openAlerts, err := b.Alerts.ListAlerts(ctx, "open")
	if err != nil {
		return "", "", fmt.Errorf("insights: list open alerts: %w", err)
	}

	rollup, err := b.buildRollup(ctx, lookback, end)
	if err != nil {
		return "", "", err
	}

	logql := fmt.Sprintf(`{job=%q, severity=~"err|warning"}`, b.JobLabel)
	result, err := b.Loki.QueryRange(ctx, logql, start, end, 5000)
	if err != nil {
		return "", "", fmt.Errorf("insights: query err/warning samples: %w", err)
	}
	samples := dedupeSamples(result.Entries, sampleCap)

	var user strings.Builder
	user.WriteString(formatAlertsSummary(openAlerts))
	user.WriteString("\n")
	user.WriteString(formatRollup(rollup))
	user.WriteString("\n")
	user.WriteString(formatSamples(samples))

	return systemPromptText, user.String(), nil
}
