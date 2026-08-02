package rules

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/hibikipr/homeSIEM/siem-api/internal/alerts"
	"github.com/hibikipr/homeSIEM/siem-api/internal/loki"
	"github.com/hibikipr/homeSIEM/siem-api/internal/store"
)

type LokiQuerier interface {
	QueryRange(ctx context.Context, logql string, start, end time.Time, limit int) (loki.QueryResult, error)
}

type Evaluator interface {
	Evaluate(ctx context.Context, rule store.Rule) ([]alerts.Candidate, error)
}

type ThresholdEvaluator struct {
	Querier LokiQuerier
}

func (e *ThresholdEvaluator) Evaluate(ctx context.Context, rule store.Rule) ([]alerts.Candidate, error) {
	end := time.Now().UTC()
	start := end.Add(-time.Duration(rule.WindowSec) * time.Second)

	result, err := e.Querier.QueryRange(ctx, rule.LogQL, start, end, 5000)
	if err != nil {
		return nil, err
	}

	threshold := 1
	if rule.Threshold != nil {
		threshold = *rule.Threshold
	}

	groups := map[string][]loki.LogEntry{}
	for _, entry := range result.Entries {
		k := groupKeyFor(entry, rule.GroupBy)
		groups[k] = append(groups[k], entry)
	}

	var candidates []alerts.Candidate
	for k, entries := range groups {
		if len(entries) < threshold {
			continue
		}
		candidates = append(candidates, alerts.Candidate{
			RuleID:   rule.ID,
			GroupKey: k,
			Severity: rule.Severity,
			Title:    fmt.Sprintf("%s: %d events for %s", rule.Name, len(entries), k),
			Body:     fmt.Sprintf("%d matching events in the last %ds", len(entries), rule.WindowSec),
			Samples:  samplesFrom(entries),
		})
	}
	return candidates, nil
}

func groupKeyFor(entry loki.LogEntry, groupBy []string) string {
	if len(groupBy) == 0 {
		return "_all"
	}
	var fields map[string]any
	_ = json.Unmarshal([]byte(entry.Line), &fields) // best-effort; ungrouped fields become ""

	parts := make([]string, len(groupBy))
	for i, field := range groupBy {
		if v, ok := fields[field]; ok {
			parts[i] = fmt.Sprintf("%v", v)
		}
	}
	return strings.Join(parts, "|")
}

func samplesFrom(entries []loki.LogEntry) []alerts.Sample {
	n := len(entries)
	if n > 5 {
		n = 5
	}
	out := make([]alerts.Sample, n)
	for i := 0; i < n; i++ {
		out[i] = alerts.Sample{TS: entries[i].Timestamp, Line: entries[i].Line}
	}
	return out
}
