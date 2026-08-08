package alerts

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/hibikipr/homeSIEM/siem-api/internal/sse"
	"github.com/hibikipr/homeSIEM/siem-api/internal/store"
)

type fakeAlertStore struct {
	rules           map[int64]store.Rule
	openAlerts      map[string]*store.Alert // key: ruleID:groupKey
	nextID          int64
	inserted        []store.Alert
	touched         []int64
	reopened        []int64
	samplesAdded    int
	minSeverity     string
	minSeverityErr  error
}

func newFakeAlertStore() *fakeAlertStore {
	return &fakeAlertStore{rules: map[int64]store.Rule{}, openAlerts: map[string]*store.Alert{}}
}

func key(ruleID int64, groupKey string) string {
	return fmt.Sprintf("%d:%s", ruleID, groupKey)
}

func (f *fakeAlertStore) GetRule(ctx context.Context, id int64) (store.Rule, error) {
	return f.rules[id], nil
}

func (f *fakeAlertStore) FindLatestAlert(ctx context.Context, ruleID int64, groupKey string) (*store.Alert, error) {
	return f.openAlerts[key(ruleID, groupKey)], nil
}

func (f *fakeAlertStore) InsertAlert(ctx context.Context, a store.Alert) (store.Alert, error) {
	f.nextID++
	a.ID = f.nextID
	f.inserted = append(f.inserted, a)
	f.openAlerts[key(a.RuleID, a.GroupKey)] = &a
	return a, nil
}

func (f *fakeAlertStore) TouchAlert(ctx context.Context, id int64, at time.Time) error {
	f.touched = append(f.touched, id)
	for _, a := range f.openAlerts {
		if a.ID == id {
			a.EventCount++
			a.LastSeenAt = at
		}
	}
	return nil
}

func (f *fakeAlertStore) ReopenAlert(ctx context.Context, id int64, at time.Time) error {
	f.reopened = append(f.reopened, id)
	for _, a := range f.openAlerts {
		if a.ID == id {
			a.LastSeenAt = at
		}
	}
	return nil
}

func (f *fakeAlertStore) AddAlertSample(ctx context.Context, alertID int64, ts time.Time, line string) error {
	f.samplesAdded++
	return nil
}

func (f *fakeAlertStore) GetMinNotifySeverity(ctx context.Context) (string, error) {
	if f.minSeverityErr != nil {
		return "", f.minSeverityErr
	}
	return f.minSeverity, nil
}

type fakeNotifier struct {
	calls int
}

func (f *fakeNotifier) Publish(ctx context.Context, title, message, priority string) error {
	f.calls++
	return nil
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestRaise_NewAlert_InsertsAndNotifies(t *testing.T) {
	fs := newFakeAlertStore()
	fs.rules[1] = store.Rule{ID: 1, CooldownSec: 3600}
	hub := sse.NewHub()
	ch, cancel := hub.Subscribe("alerts")
	defer cancel()
	notifier := &fakeNotifier{}

	svc := NewService(fs, hub, notifier, testLogger())
	err := svc.Raise(context.Background(), Candidate{
		RuleID: 1, GroupKey: "10.0.0.5", Severity: "critical",
		Title: "Port scan", Body: "40 drops", Samples: []Sample{{TS: time.Now(), Line: "line1"}},
	})
	if err != nil {
		t.Fatalf("Raise() error = %v", err)
	}

	if len(fs.inserted) != 1 {
		t.Fatalf("inserted = %d, want 1", len(fs.inserted))
	}
	if fs.samplesAdded != 1 {
		t.Errorf("samplesAdded = %d, want 1", fs.samplesAdded)
	}
	if notifier.calls != 1 {
		t.Errorf("notifier.calls = %d, want 1", notifier.calls)
	}

	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatal("expected an SSE publish for a new alert")
	}
}

func TestRaise_BelowMinSeverity_NoNotify(t *testing.T) {
	fs := newFakeAlertStore()
	fs.rules[1] = store.Rule{ID: 1, CooldownSec: 3600}
	fs.minSeverity = "critical"
	hub := sse.NewHub()
	ch, cancel := hub.Subscribe("alerts")
	defer cancel()
	notifier := &fakeNotifier{}

	svc := NewService(fs, hub, notifier, testLogger())
	err := svc.Raise(context.Background(), Candidate{
		RuleID: 1, GroupKey: "10.0.0.5", Severity: "warning", Title: "t", Body: "b",
	})
	if err != nil {
		t.Fatalf("Raise() error = %v", err)
	}

	if notifier.calls != 0 {
		t.Errorf("notifier.calls = %d, want 0 for warning below a critical threshold", notifier.calls)
	}
	if len(fs.inserted) != 1 {
		t.Errorf("inserted = %d, want 1 - the alert itself must still be recorded regardless of the notify filter", len(fs.inserted))
	}
	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatal("expected an SSE publish even though the severity is below the notify threshold - in-app visibility is unaffected by this filter")
	}
}

func TestRaise_MinSeverityReadFails_NotifiesAnyway(t *testing.T) {
	fs := newFakeAlertStore()
	fs.rules[1] = store.Rule{ID: 1, CooldownSec: 3600}
	fs.minSeverityErr = errors.New("db unavailable")
	hub := sse.NewHub()
	notifier := &fakeNotifier{}

	svc := NewService(fs, hub, notifier, testLogger())
	err := svc.Raise(context.Background(), Candidate{
		RuleID: 1, GroupKey: "10.0.0.5", Severity: "info", Title: "t", Body: "b",
	})
	if err != nil {
		t.Fatalf("Raise() error = %v", err)
	}
	if notifier.calls != 1 {
		t.Errorf("notifier.calls = %d, want 1 - a failed threshold read must fail open, not silently drop a real notification", notifier.calls)
	}
}

func TestRaise_WithinCooldown_TouchesOnlyNoNotify(t *testing.T) {
	fs := newFakeAlertStore()
	fs.rules[1] = store.Rule{ID: 1, CooldownSec: 3600}
	now := time.Now().UTC()
	fs.openAlerts[key(1, "10.0.0.5")] = &store.Alert{ID: 99, RuleID: 1, GroupKey: "10.0.0.5", State: "open", LastSeenAt: now}
	hub := sse.NewHub()
	ch, cancel := hub.Subscribe("alerts")
	defer cancel()
	notifier := &fakeNotifier{}

	svc := NewService(fs, hub, notifier, testLogger())
	err := svc.Raise(context.Background(), Candidate{RuleID: 1, GroupKey: "10.0.0.5", Severity: "critical", Title: "t", Body: "b"})
	if err != nil {
		t.Fatalf("Raise() error = %v", err)
	}

	if len(fs.touched) != 1 || fs.touched[0] != 99 {
		t.Errorf("touched = %v, want [99]", fs.touched)
	}
	if len(fs.inserted) != 0 || len(fs.reopened) != 0 {
		t.Error("expected no insert/reopen within cooldown")
	}
	if notifier.calls != 0 {
		t.Errorf("notifier.calls = %d, want 0 within cooldown", notifier.calls)
	}

	select {
	case msg := <-ch:
		t.Fatalf("expected no SSE publish within cooldown, got %q", msg)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestSeverityToPriority(t *testing.T) {
	cases := []struct {
		severity string
		want     string
	}{
		{"critical", "urgent"},
		{"warning", "default"},
		{"info", "low"},
		{"unrecognized", "low"},
	}
	for _, c := range cases {
		if got := severityToPriority(c.severity); got != c.want {
			t.Errorf("severityToPriority(%q) = %q, want %q", c.severity, got, c.want)
		}
	}
}

func TestSeverityRank(t *testing.T) {
	cases := []struct {
		severity string
		want     int
	}{
		{"critical", 2},
		{"warning", 1},
		{"info", 0},
		{"unrecognized", 0},
	}
	for _, c := range cases {
		if got := severityRank(c.severity); got != c.want {
			t.Errorf("severityRank(%q) = %d, want %d", c.severity, got, c.want)
		}
	}
}

func TestRaise_PastCooldown_ReopensAndNotifies(t *testing.T) {
	fs := newFakeAlertStore()
	fs.rules[1] = store.Rule{ID: 1, CooldownSec: 60}
	old := time.Now().UTC().Add(-time.Hour)
	fs.openAlerts[key(1, "10.0.0.5")] = &store.Alert{ID: 99, RuleID: 1, GroupKey: "10.0.0.5", LastSeenAt: old}
	hub := sse.NewHub()
	notifier := &fakeNotifier{}

	svc := NewService(fs, hub, notifier, testLogger())
	err := svc.Raise(context.Background(), Candidate{RuleID: 1, GroupKey: "10.0.0.5", Severity: "critical", Title: "t", Body: "b"})
	if err != nil {
		t.Fatalf("Raise() error = %v", err)
	}

	if len(fs.reopened) != 1 || fs.reopened[0] != 99 {
		t.Errorf("reopened = %v, want [99]", fs.reopened)
	}
	if notifier.calls != 1 {
		t.Errorf("notifier.calls = %d, want 1 past cooldown", notifier.calls)
	}
}

func TestRaise_MutedAndUnexpired_TouchesOnlyNoNotify(t *testing.T) {
	fs := newFakeAlertStore()
	fs.rules[1] = store.Rule{ID: 1, CooldownSec: 60}
	now := time.Now().UTC()
	until := now.Add(30 * time.Minute)
	fs.openAlerts[key(1, "10.0.0.5")] = &store.Alert{
		ID: 99, RuleID: 1, GroupKey: "10.0.0.5", State: "muted",
		LastSeenAt: now.Add(-time.Hour), MutedUntil: &until,
	}
	hub := sse.NewHub()
	ch, cancel := hub.Subscribe("alerts")
	defer cancel()
	notifier := &fakeNotifier{}

	svc := NewService(fs, hub, notifier, testLogger())
	err := svc.Raise(context.Background(), Candidate{RuleID: 1, GroupKey: "10.0.0.5", Severity: "critical", Title: "t", Body: "b"})
	if err != nil {
		t.Fatalf("Raise() error = %v", err)
	}

	if len(fs.touched) != 1 || fs.touched[0] != 99 {
		t.Errorf("touched = %v, want [99]", fs.touched)
	}
	if len(fs.reopened) != 0 {
		t.Error("expected no reopen while muted and unexpired")
	}
	if notifier.calls != 0 {
		t.Errorf("notifier.calls = %d, want 0 while muted", notifier.calls)
	}

	select {
	case msg := <-ch:
		t.Fatalf("expected no SSE publish while muted, got %q", msg)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestRaise_MutedAndExpired_ReopensAndNotifies(t *testing.T) {
	fs := newFakeAlertStore()
	fs.rules[1] = store.Rule{ID: 1, CooldownSec: 60}
	now := time.Now().UTC()
	expired := now.Add(-time.Minute)
	fs.openAlerts[key(1, "10.0.0.5")] = &store.Alert{
		ID: 99, RuleID: 1, GroupKey: "10.0.0.5", State: "muted",
		LastSeenAt: now.Add(-2 * time.Hour), MutedUntil: &expired,
	}
	hub := sse.NewHub()
	notifier := &fakeNotifier{}

	svc := NewService(fs, hub, notifier, testLogger())
	err := svc.Raise(context.Background(), Candidate{RuleID: 1, GroupKey: "10.0.0.5", Severity: "critical", Title: "t", Body: "b"})
	if err != nil {
		t.Fatalf("Raise() error = %v", err)
	}

	if len(fs.reopened) != 1 || fs.reopened[0] != 99 {
		t.Errorf("reopened = %v, want [99]", fs.reopened)
	}
	if notifier.calls != 1 {
		t.Errorf("notifier.calls = %d, want 1 after mute expiry", notifier.calls)
	}
}
