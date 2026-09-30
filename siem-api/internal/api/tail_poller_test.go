package api

import (
	"context"
	"encoding/json"
	"sync"
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

// lagTailQuerier simulates real Loki ingestion lag: an entry's own event
// timestamp is early, but the entry doesn't become visible to queries until
// revealAt (wall-clock), well after that timestamp.
type lagTailQuerier struct {
	entries  []loki.LogEntry
	revealAt time.Time
}

func (f *lagTailQuerier) QueryRange(ctx context.Context, logql string, start, end time.Time, limit int) (loki.QueryResult, error) {
	if time.Now().Before(f.revealAt) {
		return loki.QueryResult{}, nil
	}
	var out []loki.LogEntry
	for _, e := range f.entries {
		if e.Timestamp.After(start) && !e.Timestamp.After(end) {
			out = append(out, e)
		}
	}
	return loki.QueryResult{Entries: out}, nil
}

func TestRunTailPoller_DoesNotDropEntriesDelayedByIngestionLag(t *testing.T) {
	now := time.Now().UTC()
	querier := &lagTailQuerier{
		entries:  []loki.LogEntry{{Timestamp: now.Add(10 * time.Millisecond), Line: "delayed"}},
		revealAt: time.Now().Add(150 * time.Millisecond),
	}
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
		t.Fatal("timed out waiting for the tail poller to publish the delayed entry")
	}
	if got.Line != "delayed" {
		t.Errorf("Line = %q, want delayed", got.Line)
	}
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

// countingTailQuerier counts calls and serves whatever entries are
// currently set, filtered to the queried window.
type countingTailQuerier struct {
	mu      sync.Mutex
	calls   int
	entries []loki.LogEntry
}

func (f *countingTailQuerier) QueryRange(ctx context.Context, logql string, start, end time.Time, limit int) (loki.QueryResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	var out []loki.LogEntry
	for _, e := range f.entries {
		if e.Timestamp.After(start) && !e.Timestamp.After(end) {
			out = append(out, e)
		}
	}
	return loki.QueryResult{Entries: out}, nil
}

func (f *countingTailQuerier) set(entries ...loki.LogEntry) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.entries = entries
}

func (f *countingTailQuerier) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func TestRunTailPoller_SkipsLokiWithNoSubscribers(t *testing.T) {
	querier := &countingTailQuerier{}
	hub := sse.NewHub()

	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	go RunTailPoller(ctx, querier, "siem", hub, 10*time.Millisecond, apiTestLogger())

	time.Sleep(80 * time.Millisecond)
	if n := querier.callCount(); n != 0 {
		t.Fatalf("QueryRange calls with no subscribers = %d, want 0", n)
	}

	_, cancel := hub.Subscribe("tail")
	defer cancel()
	deadline := time.Now().Add(2 * time.Second)
	for querier.callCount() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("poller never queried Loki after a subscriber joined")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// A slower sender's entry can become visible in Loki only after a newer
// entry from a faster sender was already published. It must still be
// published (the old high-water-mark design skipped it forever).
func TestRunTailPoller_PublishesLateEntryOlderThanOnePublishedAlready(t *testing.T) {
	now := time.Now().UTC()
	older := loki.LogEntry{Timestamp: now.Add(-2 * time.Second), Labels: map[string]string{"source": "slow"}, Line: "late"}
	newer := loki.LogEntry{Timestamp: now.Add(-1 * time.Second), Labels: map[string]string{"source": "fast"}, Line: "prompt"}
	querier := &countingTailQuerier{}
	querier.set(newer)

	hub := sse.NewHub()
	ch, cancel := hub.Subscribe("tail")
	defer cancel()

	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	go RunTailPoller(ctx, querier, "siem", hub, 10*time.Millisecond, apiTestLogger())

	next := func() string {
		t.Helper()
		select {
		case msg := <-ch:
			var got struct{ Line string }
			if err := json.Unmarshal(msg, &got); err != nil {
				t.Fatalf("json.Unmarshal() error = %v", err)
			}
			return got.Line
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for a publish")
			return ""
		}
	}

	if got := next(); got != "prompt" {
		t.Fatalf("first publish = %q, want prompt", got)
	}
	querier.set(older, newer) // the slow sender's entry finally lands
	if got := next(); got != "late" {
		t.Fatalf("second publish = %q, want late", got)
	}
	select {
	case msg := <-ch:
		t.Fatalf("unexpected extra publish: %s", msg)
	case <-time.After(60 * time.Millisecond):
	}
}
