package store

import (
	"context"
	"testing"
	"time"
)

func createTestRule(t *testing.T, s *Store) Rule {
	t.Helper()
	ctx := context.Background()
	r, err := s.CreateRule(ctx, Rule{Name: "wan-portscan", Shape: "threshold", Severity: "critical",
		Destinations: []string{"inapp"}, CooldownSec: 3600, IntervalSec: 60, Enabled: true}, nil)
	if err != nil {
		t.Fatalf("CreateRule() error = %v", err)
	}
	return r
}

func TestFindOpenAlert_NotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	rule := createTestRule(t, s)

	got, err := s.FindOpenAlert(ctx, rule.ID, "10.0.0.5")
	if err != nil {
		t.Fatalf("FindOpenAlert() error = %v", err)
	}
	if got != nil {
		t.Fatalf("FindOpenAlert() = %+v, want nil", got)
	}
}

func TestInsertAndFindOpenAlert(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	rule := createTestRule(t, s)
	now := time.Now().UTC()

	inserted, err := s.InsertAlert(ctx, Alert{
		RuleID: rule.ID, GroupKey: "10.0.0.5", Severity: "critical",
		Title: "Port scan from 10.0.0.5", Body: "40 dropped connections in 60s",
		EventCount: 1, Context: "{}", State: "open", FirstSeenAt: now, LastSeenAt: now,
	})
	if err != nil {
		t.Fatalf("InsertAlert() error = %v", err)
	}
	if inserted.ID == 0 {
		t.Error("InsertAlert() ID = 0, want nonzero")
	}

	found, err := s.FindOpenAlert(ctx, rule.ID, "10.0.0.5")
	if err != nil {
		t.Fatalf("FindOpenAlert() error = %v", err)
	}
	if found == nil || found.ID != inserted.ID {
		t.Fatalf("FindOpenAlert() = %+v, want id %d", found, inserted.ID)
	}
}

func TestTouchAlert_IncrementsEventCount(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	rule := createTestRule(t, s)
	now := time.Now().UTC()

	inserted, err := s.InsertAlert(ctx, Alert{
		RuleID: rule.ID, GroupKey: "10.0.0.5", Severity: "critical", Title: "t", Body: "b",
		EventCount: 1, Context: "{}", State: "open", FirstSeenAt: now, LastSeenAt: now,
	})
	if err != nil {
		t.Fatalf("InsertAlert() error = %v", err)
	}

	later := now.Add(5 * time.Minute)
	if err := s.TouchAlert(ctx, inserted.ID, later); err != nil {
		t.Fatalf("TouchAlert() error = %v", err)
	}

	got, err := s.GetAlert(ctx, inserted.ID)
	if err != nil {
		t.Fatalf("GetAlert() error = %v", err)
	}
	if got.EventCount != 2 {
		t.Errorf("EventCount = %d, want 2", got.EventCount)
	}
}

func TestAddAlertSample_CapsAtTen(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	rule := createTestRule(t, s)
	now := time.Now().UTC()

	inserted, err := s.InsertAlert(ctx, Alert{
		RuleID: rule.ID, GroupKey: "10.0.0.5", Severity: "critical", Title: "t", Body: "b",
		EventCount: 1, Context: "{}", State: "open", FirstSeenAt: now, LastSeenAt: now,
	})
	if err != nil {
		t.Fatalf("InsertAlert() error = %v", err)
	}

	for i := 0; i < 15; i++ {
		ts := now.Add(time.Duration(i) * time.Second)
		if err := s.AddAlertSample(ctx, inserted.ID, ts, "line"); err != nil {
			t.Fatalf("AddAlertSample() error = %v", err)
		}
	}

	samples, err := s.ListAlertSamples(ctx, inserted.ID)
	if err != nil {
		t.Fatalf("ListAlertSamples() error = %v", err)
	}
	if len(samples) != 10 {
		t.Fatalf("len(samples) = %d, want 10", len(samples))
	}
}

func TestAckAlert(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	rule := createTestRule(t, s)
	now := time.Now().UTC()

	inserted, err := s.InsertAlert(ctx, Alert{
		RuleID: rule.ID, GroupKey: "10.0.0.5", Severity: "critical", Title: "t", Body: "b",
		EventCount: 1, Context: "{}", State: "open", FirstSeenAt: now, LastSeenAt: now,
	})
	if err != nil {
		t.Fatalf("InsertAlert() error = %v", err)
	}

	if err := s.AckAlert(ctx, inserted.ID, 1, now); err != nil {
		t.Fatalf("AckAlert() error = %v", err)
	}

	got, err := s.GetAlert(ctx, inserted.ID)
	if err != nil {
		t.Fatalf("GetAlert() error = %v", err)
	}
	if got.State != "acked" {
		t.Errorf("State = %q, want acked", got.State)
	}
	if got.AckedBy == nil || *got.AckedBy != 1 {
		t.Errorf("AckedBy = %v, want 1", got.AckedBy)
	}

	entries, err := s.ListAudit(ctx, 10)
	if err != nil {
		t.Fatalf("ListAudit() error = %v", err)
	}
	found := false
	for _, e := range entries {
		if e.Action == "alert.ack" {
			found = true
		}
	}
	if !found {
		t.Error("no alert.ack audit entry found")
	}
}

func TestReopenAlert(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	rule := createTestRule(t, s)
	now := time.Now().UTC()

	inserted, err := s.InsertAlert(ctx, Alert{
		RuleID: rule.ID, GroupKey: "10.0.0.5", Severity: "critical", Title: "t", Body: "b",
		EventCount: 1, Context: "{}", State: "closed", FirstSeenAt: now, LastSeenAt: now,
	})
	if err != nil {
		t.Fatalf("InsertAlert() error = %v", err)
	}

	later := now.Add(time.Hour)
	if err := s.ReopenAlert(ctx, inserted.ID, later); err != nil {
		t.Fatalf("ReopenAlert() error = %v", err)
	}

	got, err := s.GetAlert(ctx, inserted.ID)
	if err != nil {
		t.Fatalf("GetAlert() error = %v", err)
	}
	if got.State != "open" {
		t.Errorf("State = %q, want open", got.State)
	}
}

func TestListAlerts_FilterByState(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	rule := createTestRule(t, s)
	now := time.Now().UTC()

	if _, err := s.InsertAlert(ctx, Alert{RuleID: rule.ID, GroupKey: "a", Severity: "low", Title: "t", Body: "b",
		EventCount: 1, Context: "{}", State: "open", FirstSeenAt: now, LastSeenAt: now}); err != nil {
		t.Fatalf("InsertAlert() error = %v", err)
	}
	if _, err := s.InsertAlert(ctx, Alert{RuleID: rule.ID, GroupKey: "b", Severity: "low", Title: "t", Body: "b",
		EventCount: 1, Context: "{}", State: "acked", FirstSeenAt: now, LastSeenAt: now}); err != nil {
		t.Fatalf("InsertAlert() error = %v", err)
	}

	open, err := s.ListAlerts(ctx, "open")
	if err != nil {
		t.Fatalf("ListAlerts() error = %v", err)
	}
	if len(open) != 1 || open[0].GroupKey != "a" {
		t.Fatalf("ListAlerts(open) = %+v, want [a]", open)
	}

	all, err := s.ListAlerts(ctx, "")
	if err != nil {
		t.Fatalf("ListAlerts() error = %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("ListAlerts(\"\") len = %d, want 2", len(all))
	}
}

func TestMuteAlert(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	rule := createTestRule(t, s)
	now := time.Now().UTC()

	inserted, err := s.InsertAlert(ctx, Alert{
		RuleID: rule.ID, GroupKey: "10.0.0.5", Severity: "critical", Title: "t", Body: "b",
		EventCount: 1, Context: "{}", State: "open", FirstSeenAt: now, LastSeenAt: now,
	})
	if err != nil {
		t.Fatalf("InsertAlert() error = %v", err)
	}

	until := now.Add(time.Hour)
	if err := s.MuteAlert(ctx, inserted.ID, 1, until); err != nil {
		t.Fatalf("MuteAlert() error = %v", err)
	}

	got, err := s.GetAlert(ctx, inserted.ID)
	if err != nil {
		t.Fatalf("GetAlert() error = %v", err)
	}
	if got.State != "muted" {
		t.Errorf("State = %q, want muted", got.State)
	}
	// Compare without nanoseconds since database stores only seconds
	expectedUntil := until.Truncate(time.Second)
	if got.MutedUntil == nil || !got.MutedUntil.Equal(expectedUntil) {
		t.Errorf("MutedUntil = %v, want %v", got.MutedUntil, expectedUntil)
	}

	entries, err := s.ListAudit(ctx, 10)
	if err != nil {
		t.Fatalf("ListAudit() error = %v", err)
	}
	found := false
	for _, e := range entries {
		if e.Action == "alert.mute" {
			found = true
		}
	}
	if !found {
		t.Error("no alert.mute audit entry found")
	}
}
