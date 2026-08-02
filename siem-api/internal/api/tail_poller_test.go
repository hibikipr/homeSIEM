package api

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/hibikipr/homeSIEM/siem-api/internal/loki"
	"github.com/hibikipr/homeSIEM/siem-api/internal/sse"
)

type fakeTailQuerier struct {
	entries []loki.LogEntry
}

func (f *fakeTailQuerier) QueryRange(ctx context.Context, logql string, start, end time.Time, limit int) (loki.QueryResult, error) {
	var out []loki.LogEntry
	for _, e := range f.entries {
		if e.Timestamp.After(start) && !e.Timestamp.After(end) {
			out = append(out, e)
		}
	}
	return loki.QueryResult{Entries: out}, nil
}

func TestRunTailPoller_PublishesNewEntriesOnce(t *testing.T) {
	now := time.Now().UTC()
	querier := &fakeTailQuerier{entries: []loki.LogEntry{
		{Timestamp: now.Add(10 * time.Millisecond), Line: "first"},
	}}
	hub := sse.NewHub()
	ch, cancel := hub.Subscribe("tail")
	defer cancel()

	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	go RunTailPoller(ctx, querier, "siem", hub, 20*time.Millisecond, apiTestLogger())

	var got struct{ Line string }
	select {
	case msg := <-ch:
		if err := json.Unmarshal(msg, &got); err != nil {
			t.Fatalf("json.Unmarshal() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the tail poller to publish")
	}
	if got.Line != "first" {
		t.Errorf("Line = %q, want first", got.Line)
	}

	// No further entries added — should not receive a duplicate publish.
	select {
	case msg := <-ch:
		t.Fatalf("received an unexpected second publish: %s", msg)
	case <-time.After(100 * time.Millisecond):
	}
}
