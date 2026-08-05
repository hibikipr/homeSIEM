package rules

import (
	"context"
	"testing"
	"time"

	"github.com/hibikipr/homeSIEM/siem-api/internal/loki"
	"github.com/hibikipr/homeSIEM/siem-api/internal/store"
)

type fakeLokiQuerier struct {
	result   loki.QueryResult
	err      error
	gotLogQL string
}

func (f *fakeLokiQuerier) QueryRange(ctx context.Context, logql string, start, end time.Time, limit int) (loki.QueryResult, error) {
	f.gotLogQL = logql
	return f.result, f.err
}

func TestThresholdEvaluator_FiresWhenThresholdReached(t *testing.T) {
	now := time.Now().UTC()
	querier := &fakeLokiQuerier{result: loki.QueryResult{Entries: []loki.LogEntry{
		{Timestamp: now, Line: `{"src_ip":"10.0.0.5","dst_port":22}`},
		{Timestamp: now, Line: `{"src_ip":"10.0.0.5","dst_port":23}`},
		{Timestamp: now, Line: `{"src_ip":"10.0.0.5","dst_port":25}`},
		{Timestamp: now, Line: `{"src_ip":"10.0.0.9","dst_port":80}`},
	}}}
	e := &ThresholdEvaluator{Querier: querier}

	threshold := 3
	rule := store.Rule{ID: 1, Name: "wan-portscan", LogQL: `{job="siem"}`, WindowSec: 60,
		Threshold: &threshold, GroupBy: []string{"src_ip"}, Severity: "critical"}

	candidates, err := e.Evaluate(context.Background(), rule)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("len(candidates) = %d, want 1", len(candidates))
	}
	if candidates[0].GroupKey != "10.0.0.5" {
		t.Errorf("GroupKey = %q, want 10.0.0.5", candidates[0].GroupKey)
	}
	if candidates[0].RuleID != 1 {
		t.Errorf("RuleID = %d, want 1", candidates[0].RuleID)
	}
	if len(candidates[0].Samples) != 3 {
		t.Errorf("len(Samples) = %d, want 3", len(candidates[0].Samples))
	}

	if querier.gotLogQL != `{job="siem"}` {
		t.Errorf("QueryRange logql = %q, want rule.LogQL", querier.gotLogQL)
	}
}

func TestThresholdEvaluator_BelowThresholdNoCandidate(t *testing.T) {
	now := time.Now().UTC()
	querier := &fakeLokiQuerier{result: loki.QueryResult{Entries: []loki.LogEntry{
		{Timestamp: now, Line: `{"src_ip":"10.0.0.5"}`},
	}}}
	e := &ThresholdEvaluator{Querier: querier}

	threshold := 3
	rule := store.Rule{ID: 1, LogQL: `{job="siem"}`, WindowSec: 60, Threshold: &threshold, GroupBy: []string{"src_ip"}}

	candidates, err := e.Evaluate(context.Background(), rule)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if len(candidates) != 0 {
		t.Fatalf("len(candidates) = %d, want 0", len(candidates))
	}
}

func TestThresholdEvaluator_NoGroupByGroupsAll(t *testing.T) {
	now := time.Now().UTC()
	querier := &fakeLokiQuerier{result: loki.QueryResult{Entries: []loki.LogEntry{
		{Timestamp: now, Line: `{"anything":"a"}`},
		{Timestamp: now, Line: `{"anything":"b"}`},
	}}}
	e := &ThresholdEvaluator{Querier: querier}

	threshold := 2
	rule := store.Rule{ID: 1, LogQL: `{job="siem"}`, WindowSec: 60, Threshold: &threshold}

	candidates, err := e.Evaluate(context.Background(), rule)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if len(candidates) != 1 || candidates[0].GroupKey != "_all" {
		t.Fatalf("candidates = %+v, want one candidate with GroupKey _all", candidates)
	}
}
