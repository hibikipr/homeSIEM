package store

import (
	"context"
	"testing"
	"time"
)

func TestCreateAndGetRule(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	threshold := 5
	created, err := s.CreateRule(ctx, Rule{
		Name: "wan-portscan", Shape: "threshold", LogQL: `{job="siem"} |= "DROP"`,
		WindowSec: 60, Threshold: &threshold, GroupBy: []string{"src_ip"},
		Severity: "critical", Destinations: []string{"inapp", "ntfy"},
		CooldownSec: 3600, IntervalSec: 60, Enabled: true,
	}, nil)
	if err != nil {
		t.Fatalf("CreateRule() error = %v", err)
	}
	if created.ID == 0 {
		t.Error("CreateRule() ID = 0, want nonzero")
	}

	got, err := s.GetRule(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetRule() error = %v", err)
	}
	if got.Name != "wan-portscan" || got.Shape != "threshold" {
		t.Errorf("GetRule() = %+v", got)
	}
	if len(got.GroupBy) != 1 || got.GroupBy[0] != "src_ip" {
		t.Errorf("GroupBy = %v, want [src_ip]", got.GroupBy)
	}
	if got.Threshold == nil || *got.Threshold != 5 {
		t.Errorf("Threshold = %v, want 5", got.Threshold)
	}
}

func TestListEnabledRules(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if _, err := s.CreateRule(ctx, Rule{Name: "on", Shape: "absence", Severity: "low",
		Destinations: []string{"inapp"}, CooldownSec: 60, IntervalSec: 60, Enabled: true}, nil); err != nil {
		t.Fatalf("CreateRule() error = %v", err)
	}
	if _, err := s.CreateRule(ctx, Rule{Name: "off", Shape: "absence", Severity: "low",
		Destinations: []string{"inapp"}, CooldownSec: 60, IntervalSec: 60, Enabled: false}, nil); err != nil {
		t.Fatalf("CreateRule() error = %v", err)
	}

	enabled, err := s.ListEnabledRules(ctx)
	if err != nil {
		t.Fatalf("ListEnabledRules() error = %v", err)
	}
	if len(enabled) != 1 || enabled[0].Name != "on" {
		t.Fatalf("ListEnabledRules() = %+v, want [on]", enabled)
	}
}

func TestUpdateAndDeleteRule(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	created, err := s.CreateRule(ctx, Rule{Name: "r1", Shape: "absence", Severity: "low",
		Destinations: []string{"inapp"}, CooldownSec: 60, IntervalSec: 60, Enabled: true}, nil)
	if err != nil {
		t.Fatalf("CreateRule() error = %v", err)
	}

	created.Enabled = false
	updated, err := s.UpdateRule(ctx, created, nil)
	if err != nil {
		t.Fatalf("UpdateRule() error = %v", err)
	}
	if updated.Enabled {
		t.Error("UpdateRule() Enabled = true, want false")
	}

	if err := s.DeleteRule(ctx, created.ID, nil); err != nil {
		t.Fatalf("DeleteRule() error = %v", err)
	}
	if _, err := s.GetRule(ctx, created.ID); err == nil {
		t.Fatal("GetRule() after delete: error = nil, want not found")
	}

	entries, err := s.ListAudit(ctx, 10)
	if err != nil {
		t.Fatalf("ListAudit() error = %v", err)
	}
	var actions []string
	for _, e := range entries {
		actions = append(actions, e.Action)
	}
	wantContains := []string{"rule.create", "rule.update", "rule.delete"}
	for _, want := range wantContains {
		found := false
		for _, a := range actions {
			if a == want {
				found = true
			}
		}
		if !found {
			t.Errorf("audit actions = %v, want to contain %q", actions, want)
		}
	}
}

func TestTouchRuleLastRun(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	created, err := s.CreateRule(ctx, Rule{Name: "r1", Shape: "absence", Severity: "low",
		Destinations: []string{"inapp"}, CooldownSec: 60, IntervalSec: 60, Enabled: true}, nil)
	if err != nil {
		t.Fatalf("CreateRule() error = %v", err)
	}

	if err := s.TouchRuleLastRun(ctx, created.ID, time.Now()); err != nil {
		t.Fatalf("TouchRuleLastRun() error = %v", err)
	}
	got, err := s.GetRule(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetRule() error = %v", err)
	}
	if got.LastRunAt == nil {
		t.Error("LastRunAt is nil after TouchRuleLastRun")
	}
}
