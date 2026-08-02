package store

import (
	"context"
	"testing"
	"time"
)

func TestHasSeenValue_UnseenThenSeen(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	rule := createTestRule(t, s)

	seen, err := s.HasSeenValue(ctx, rule.ID, "new-domain.example")
	if err != nil {
		t.Fatalf("HasSeenValue() error = %v", err)
	}
	if seen {
		t.Error("HasSeenValue() = true, want false before MarkSeenValue")
	}

	if err := s.MarkSeenValue(ctx, rule.ID, "new-domain.example", time.Now().UTC()); err != nil {
		t.Fatalf("MarkSeenValue() error = %v", err)
	}

	seen, err = s.HasSeenValue(ctx, rule.ID, "new-domain.example")
	if err != nil {
		t.Fatalf("second HasSeenValue() error = %v", err)
	}
	if !seen {
		t.Error("HasSeenValue() = false, want true after MarkSeenValue")
	}
}

func TestMarkSeenValue_Idempotent(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	rule := createTestRule(t, s)

	if err := s.MarkSeenValue(ctx, rule.ID, "v", time.Now().UTC()); err != nil {
		t.Fatalf("MarkSeenValue() error = %v", err)
	}
	if err := s.MarkSeenValue(ctx, rule.ID, "v", time.Now().UTC()); err != nil {
		t.Fatalf("second MarkSeenValue() error = %v, want nil (must be idempotent)", err)
	}
}

func TestHasSeenValue_ScopedPerRule(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	rule1 := createTestRule(t, s)
	rule2, err := s.CreateRule(ctx, Rule{Name: "other-rule", Shape: "first_seen", Severity: "low",
		Destinations: []string{"inapp"}, CooldownSec: 60, IntervalSec: 60, Enabled: true}, nil)
	if err != nil {
		t.Fatalf("CreateRule() error = %v", err)
	}

	if err := s.MarkSeenValue(ctx, rule1.ID, "shared-value", time.Now().UTC()); err != nil {
		t.Fatalf("MarkSeenValue() error = %v", err)
	}

	seen, err := s.HasSeenValue(ctx, rule2.ID, "shared-value")
	if err != nil {
		t.Fatalf("HasSeenValue() error = %v", err)
	}
	if seen {
		t.Error("HasSeenValue() = true for rule2, want false — seen_values must be scoped per rule")
	}
}
