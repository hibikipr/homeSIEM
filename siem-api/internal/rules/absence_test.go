package rules

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/hibikipr/homeSIEM/siem-api/internal/store"
)

type fakeSourcesStore struct {
	stale []store.Source
	err   error
}

func (f *fakeSourcesStore) StaleSources(ctx context.Context, now time.Time) ([]store.Source, error) {
	return f.stale, f.err
}

func TestAbsenceEvaluator_OneCandidateForASingleStaleSource(t *testing.T) {
	lastSeen := time.Now().UTC().Add(-2 * time.Hour)
	sources := &fakeSourcesStore{stale: []store.Source{
		{Name: "silent-host", HeartbeatSec: 900, LastSeenAt: &lastSeen},
	}}
	e := &AbsenceEvaluator{Sources: sources}

	rule := store.Rule{ID: 1, Name: "source-heartbeat", Severity: "warning"}
	candidates, err := e.Evaluate(context.Background(), rule)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("len(candidates) = %d, want 1", len(candidates))
	}
	c := candidates[0]
	if c.RuleID != 1 || c.Severity != "warning" || c.GroupKey != "silent-host" {
		t.Errorf("candidate = %+v, want RuleID=1 Severity=warning GroupKey=silent-host", c)
	}

	gotSources, ok := c.Context["sources"].([]sourceContext)
	if !ok || len(gotSources) != 1 {
		t.Fatalf("Context[sources] = %#v, want a single sourceContext", c.Context["sources"])
	}
	if gotSources[0].Name != "silent-host" || gotSources[0].HeartbeatSec != 900 {
		t.Errorf("gotSources[0] = %+v, want Name=silent-host HeartbeatSec=900", gotSources[0])
	}
	if gotSources[0].LastSeenAt == nil || *gotSources[0].LastSeenAt != lastSeen.Format(time.RFC3339) {
		t.Errorf("gotSources[0].LastSeenAt = %v, want %s", gotSources[0].LastSeenAt, lastSeen.Format(time.RFC3339))
	}
}

func TestAbsenceEvaluator_UsesDisplayNameInTitleAndBodyNotGroupKey(t *testing.T) {
	lastSeen := time.Now().UTC().Add(-2 * time.Hour)
	sources := &fakeSourcesStore{stale: []store.Source{
		{Name: "192.168.3.223", DisplayName: "Home Assistant", HeartbeatSec: 900, LastSeenAt: &lastSeen},
	}}
	e := &AbsenceEvaluator{Sources: sources}

	candidates, err := e.Evaluate(context.Background(), store.Rule{ID: 1, Name: "source-heartbeat", Severity: "warning"})
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	c := candidates[0]
	if c.GroupKey != "192.168.3.223" {
		t.Errorf("GroupKey = %q, want the raw name (192.168.3.223), never the display name - it's the dedup identity", c.GroupKey)
	}
	if !strings.Contains(c.Title, "Home Assistant") || !strings.Contains(c.Body, "Home Assistant") {
		t.Errorf("Title/Body = %q / %q, want the display name Home Assistant, not the raw IP", c.Title, c.Body)
	}
}

func TestAbsenceEvaluator_NoneStaleNoCandidates(t *testing.T) {
	sources := &fakeSourcesStore{stale: nil}
	e := &AbsenceEvaluator{Sources: sources}

	candidates, err := e.Evaluate(context.Background(), store.Rule{ID: 1})
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if len(candidates) != 0 {
		t.Fatalf("len(candidates) = %d, want 0", len(candidates))
	}
}

// TestAbsenceEvaluator_CorrelatesSourcesGoneQuietTogether is the regression
// test for a real production case: three unrelated devices (a source's own
// IP, a UDM, a Homebridge Pi) all fired independent "gone silent" alerts
// within a few hours of each other overnight - read as three coincidental
// device failures instead of what it almost certainly was, one shared
// cause (the collector, a network segment, a router) hiccupping.
func TestAbsenceEvaluator_CorrelatesSourcesGoneQuietTogether(t *testing.T) {
	base := time.Now().UTC()
	t1 := base.Add(-5 * time.Hour)
	t2 := base.Add(-3 * time.Hour)
	t3 := base.Add(-1 * time.Hour)
	sources := &fakeSourcesStore{stale: []store.Source{
		{Name: "udm-ultra", HeartbeatSec: 900, LastSeenAt: &t2},
		{Name: "192.168.3.223", DisplayName: "Home Assistant", HeartbeatSec: 900, LastSeenAt: &t1},
		{Name: "homebridge", HeartbeatSec: 900, LastSeenAt: &t3},
	}}
	e := &AbsenceEvaluator{Sources: sources}

	// WindowSec generous enough to cover the ~4h spread between the
	// earliest and latest LastSeenAt above.
	rule := store.Rule{ID: 1, Name: "source-heartbeat", Severity: "warning", WindowSec: 6 * 3600}
	candidates, err := e.Evaluate(context.Background(), rule)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("len(candidates) = %d, want 1 (one correlated alert, not three individual ones)", len(candidates))
	}

	c := candidates[0]
	if !strings.HasPrefix(c.GroupKey, "multi:") {
		t.Errorf("GroupKey = %q, want a multi: prefix", c.GroupKey)
	}
	if !strings.Contains(c.Title, "3 sources") {
		t.Errorf("Title = %q, want it to mention 3 sources", c.Title)
	}
	for _, want := range []string{"udm-ultra", "Home Assistant", "homebridge"} {
		if !strings.Contains(c.Body, want) {
			t.Errorf("Body = %q, want it to mention %q", c.Body, want)
		}
	}
	// The raw name, not the display name, must still appear in GroupKey -
	// it's the identity for dedup, not display text.
	if !strings.Contains(c.GroupKey, "192.168.3.223") {
		t.Errorf("GroupKey = %q, want the raw source name 192.168.3.223, not the display name", c.GroupKey)
	}

	gotSources, ok := c.Context["sources"].([]sourceContext)
	if !ok || len(gotSources) != 3 {
		t.Fatalf("Context[sources] = %#v, want 3 sourceContext entries", c.Context["sources"])
	}
}

func TestAbsenceEvaluator_DoesNotCorrelateWhenSpreadExceedsWindow(t *testing.T) {
	base := time.Now().UTC()
	t1 := base.Add(-5 * time.Hour)
	t2 := base.Add(-1 * time.Hour)
	sources := &fakeSourcesStore{stale: []store.Source{
		{Name: "host-a", HeartbeatSec: 900, LastSeenAt: &t1},
		{Name: "host-b", HeartbeatSec: 900, LastSeenAt: &t2},
	}}
	e := &AbsenceEvaluator{Sources: sources}

	// 4h apart, 30-minute window - not close enough to plausibly share a
	// cause, so this must fall back to one individual alert per source.
	rule := store.Rule{ID: 1, Name: "source-heartbeat", Severity: "warning", WindowSec: 1800}
	candidates, err := e.Evaluate(context.Background(), rule)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if len(candidates) != 2 {
		t.Fatalf("len(candidates) = %d, want 2 (spread exceeds the window, no correlation)", len(candidates))
	}
	for _, c := range candidates {
		if strings.HasPrefix(c.GroupKey, "multi:") {
			t.Errorf("candidate = %+v, want individual GroupKeys, not a multi: group", c)
		}
	}
}

func TestAbsenceEvaluator_NeverSeenSourceDoesNotBlockCorrelation(t *testing.T) {
	base := time.Now().UTC()
	t1 := base.Add(-1 * time.Hour)
	sources := &fakeSourcesStore{stale: []store.Source{
		{Name: "host-a", HeartbeatSec: 900, LastSeenAt: &t1},
		{Name: "host-never-seen", HeartbeatSec: 900, LastSeenAt: nil},
	}}
	e := &AbsenceEvaluator{Sources: sources}

	rule := store.Rule{ID: 1, Name: "source-heartbeat", Severity: "warning", WindowSec: 3600}
	candidates, err := e.Evaluate(context.Background(), rule)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("len(candidates) = %d, want 1 - a never-seen source (no timestamp to compare) must not block correlation of the ones that do have one", len(candidates))
	}
	if !strings.Contains(candidates[0].Body, "never") {
		t.Errorf("Body = %q, want it to say the never-seen source was never seen", candidates[0].Body)
	}
}

func TestAbsenceEvaluator_CorrelatedGroupKeyIsOrderIndependent(t *testing.T) {
	base := time.Now().UTC()
	t1 := base.Add(-1 * time.Hour)
	t2 := base.Add(-2 * time.Hour)
	rule := store.Rule{ID: 1, Name: "source-heartbeat", Severity: "warning", WindowSec: 3600 * 6}

	forward := &AbsenceEvaluator{Sources: &fakeSourcesStore{stale: []store.Source{
		{Name: "aaa", HeartbeatSec: 900, LastSeenAt: &t1},
		{Name: "zzz", HeartbeatSec: 900, LastSeenAt: &t2},
	}}}
	reversed := &AbsenceEvaluator{Sources: &fakeSourcesStore{stale: []store.Source{
		{Name: "zzz", HeartbeatSec: 900, LastSeenAt: &t2},
		{Name: "aaa", HeartbeatSec: 900, LastSeenAt: &t1},
	}}}

	got1, err := forward.Evaluate(context.Background(), rule)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	got2, err := reversed.Evaluate(context.Background(), rule)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if got1[0].GroupKey != got2[0].GroupKey {
		t.Errorf("GroupKey = %q vs %q, want the same regardless of StaleSources' return order", got1[0].GroupKey, got2[0].GroupKey)
	}
}
