package insights

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/hibikipr/homeSIEM/siem-api/internal/loki"
	"github.com/hibikipr/homeSIEM/siem-api/internal/store"
)

func TestExtractMessage(t *testing.T) {
	if got := extractMessage(`{"message":"hello world","other":"x"}`); got != "hello world" {
		t.Errorf("extractMessage() = %q, want %q", got, "hello world")
	}
	if got := extractMessage(`not json`); got != "not json" {
		t.Errorf("extractMessage() = %q, want the raw line unchanged", got)
	}
	if got := extractMessage(`{"other":"x"}`); got != `{"other":"x"}` {
		t.Errorf("extractMessage() = %q, want the raw line when .message is absent", got)
	}
}

func entryWith(program, message string) loki.LogEntry {
	return loki.LogEntry{
		Timestamp: time.Now(),
		Labels:    map[string]string{"program": program},
		Line:      `{"message":` + `"` + message + `"}`,
	}
}

func TestDedupeSamples_GroupsByProgramAndMessagePrefix(t *testing.T) {
	entries := []loki.LogEntry{
		entryWith("Bambuddy", "Connection stale, forcing reconnect"),
		entryWith("Bambuddy", "Connection stale, forcing reconnect"),
		entryWith("Bambuddy", "Connection stale, forcing reconnect"),
		entryWith("manyfold", "Redis connection refused"),
	}

	groups := dedupeSamples(entries, 10)

	if len(groups) != 2 {
		t.Fatalf("len(groups) = %d, want 2", len(groups))
	}
	if groups[0].Program != "Bambuddy" || groups[0].Count != 3 {
		t.Errorf("groups[0] = %+v, want Bambuddy count=3 (most frequent first)", groups[0])
	}
	if groups[1].Program != "manyfold" || groups[1].Count != 1 {
		t.Errorf("groups[1] = %+v, want manyfold count=1", groups[1])
	}
}

func TestDedupeSamples_DifferentMessagesSameProgram_AreSeparateGroups(t *testing.T) {
	entries := []loki.LogEntry{
		entryWith("loki", "error processing requests from scheduler"),
		entryWith("loki", "error notifying scheduler about finished query"),
	}

	groups := dedupeSamples(entries, 10)

	if len(groups) != 2 {
		t.Fatalf("len(groups) = %d, want 2 distinct groups for two different message shapes", len(groups))
	}
}

func TestDedupeSamples_MessagesDifferingOnlyAfterPrefixLength_AreSameGroup(t *testing.T) {
	longPrefix := strings.Repeat("x", 60)
	entries := []loki.LogEntry{
		entryWith("app", longPrefix+" tail one"),
		entryWith("app", longPrefix+" tail two"),
	}

	groups := dedupeSamples(entries, 10)

	if len(groups) != 1 {
		t.Fatalf("len(groups) = %d, want 1 (messages identical in their first 60 chars)", len(groups))
	}
	if groups[0].Count != 2 {
		t.Errorf("groups[0].Count = %d, want 2", groups[0].Count)
	}
}

func TestDedupeSamples_CapsAtCapCount_KeepingMostFrequent(t *testing.T) {
	var entries []loki.LogEntry
	// 5 groups; "big" appears 3 times, the rest once each.
	entries = append(entries, entryWith("big", "m"), entryWith("big", "m"), entryWith("big", "m"))
	for i := 0; i < 5; i++ {
		entries = append(entries, entryWith("small", string(rune('a'+i))))
	}

	groups := dedupeSamples(entries, 2)

	if len(groups) != 2 {
		t.Fatalf("len(groups) = %d, want 2 (capped)", len(groups))
	}
	if groups[0].Program != "big" || groups[0].Count != 3 {
		t.Errorf("groups[0] = %+v, want the most frequent group first", groups[0])
	}
}

func TestDedupeSamples_Empty(t *testing.T) {
	if got := dedupeSamples(nil, 10); len(got) != 0 {
		t.Errorf("dedupeSamples(nil) = %+v, want empty", got)
	}
}

type fakeLokiQuerier struct {
	rangeEntries  []loki.LogEntry
	instantSeries map[string][]loki.MatrixSeries // keyed by a substring of the logql, matched via strings.Contains
}

func (f *fakeLokiQuerier) QueryRange(ctx context.Context, logql string, start, end time.Time, limit int) (loki.QueryResult, error) {
	return loki.QueryResult{Entries: f.rangeEntries}, nil
}

func (f *fakeLokiQuerier) QueryInstant(ctx context.Context, logql string, at time.Time) (loki.MatrixResult, error) {
	for substr, series := range f.instantSeries {
		if strings.Contains(logql, substr) {
			return loki.MatrixResult{Series: series}, nil
		}
	}
	return loki.MatrixResult{}, nil
}

type fakeAlertLister struct {
	alerts []store.Alert
}

func (f *fakeAlertLister) ListAlerts(ctx context.Context, state string) ([]store.Alert, error) {
	return f.alerts, nil
}

func TestBuild_IncludesAllThreeSections(t *testing.T) {
	lokiFake := &fakeLokiQuerier{
		rangeEntries: []loki.LogEntry{
			entryWith("Bambuddy", "Connection stale, forcing reconnect"),
		},
		instantSeries: map[string][]loki.MatrixSeries{
			`severity="err"`:     {{Labels: map[string]string{"program": "loki"}, Samples: []loki.MatrixSample{{Value: 5}}}},
			`severity="warning"`: {{Labels: map[string]string{"program": "Bambuddy"}, Samples: []loki.MatrixSample{{Value: 3}}}},
		},
	}
	alertsFake := &fakeAlertLister{alerts: []store.Alert{
		{ID: 1, Title: "Port scan detected", Severity: "critical", State: "open"},
	}}

	b := &PromptBuilder{Loki: lokiFake, Alerts: alertsFake, JobLabel: "siem"}
	system, user, err := b.Build(context.Background(), time.Hour)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	if !strings.Contains(system, "JSON") {
		t.Error("system prompt doesn't mention JSON output format")
	}
	if !strings.Contains(user, "Open alerts") || !strings.Contains(user, "Port scan detected") {
		t.Errorf("user prompt missing the open alerts section: %q", user)
	}
	if !strings.Contains(user, "Severity by program") || !strings.Contains(user, "loki") {
		t.Errorf("user prompt missing the severity rollup section: %q", user)
	}
	if !strings.Contains(user, "Recent error/warning samples") || !strings.Contains(user, "Connection stale") {
		t.Errorf("user prompt missing the deduped samples section: %q", user)
	}
}

func TestBuild_NoOpenAlerts_SaysSoRatherThanOmittingTheSection(t *testing.T) {
	lokiFake := &fakeLokiQuerier{}
	alertsFake := &fakeAlertLister{}

	b := &PromptBuilder{Loki: lokiFake, Alerts: alertsFake, JobLabel: "siem"}
	_, user, err := b.Build(context.Background(), time.Hour)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if !strings.Contains(user, "Open alerts") {
		t.Errorf("user prompt missing the open alerts section header even with no alerts: %q", user)
	}
}
