package store

import (
	"context"
	"testing"
	"time"
)

func TestUpsertAndListSources(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	created, err := s.UpsertSource(ctx, Source{
		Name: "udm-ultra", Address: "10.0.0.1", Transport: "udp/514",
		Parser: "unifi-os", HeartbeatSec: 900,
	})
	if err != nil {
		t.Fatalf("UpsertSource() error = %v", err)
	}
	if created.ID == 0 {
		t.Error("UpsertSource() ID = 0, want nonzero")
	}

	// Upsert again with same name should update, not duplicate.
	_, err = s.UpsertSource(ctx, Source{
		Name: "udm-ultra", Address: "10.0.0.1", Transport: "udp/514",
		Parser: "unifi-os", HeartbeatSec: 600,
	})
	if err != nil {
		t.Fatalf("second UpsertSource() error = %v", err)
	}

	sources, err := s.ListSources(ctx)
	if err != nil {
		t.Fatalf("ListSources() error = %v", err)
	}
	if len(sources) != 1 {
		t.Fatalf("len(sources) = %d, want 1", len(sources))
	}
	if sources[0].HeartbeatSec != 600 {
		t.Errorf("HeartbeatSec = %d, want 600 (upsert should update)", sources[0].HeartbeatSec)
	}
}

func TestTouchSourceLastSeen(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if _, err := s.UpsertSource(ctx, Source{Name: "udm-ultra", Address: "10.0.0.1", Transport: "udp/514", Parser: "unifi-os", HeartbeatSec: 900}); err != nil {
		t.Fatalf("UpsertSource() error = %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	if err := s.TouchSourceLastSeen(ctx, "udm-ultra", now); err != nil {
		t.Fatalf("TouchSourceLastSeen() error = %v", err)
	}

	sources, err := s.ListSources(ctx)
	if err != nil {
		t.Fatalf("ListSources() error = %v", err)
	}
	if sources[0].LastSeenAt == nil {
		t.Fatal("LastSeenAt is nil after TouchSourceLastSeen")
	}
}

func TestClaimSource(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	created, err := s.UpsertSource(ctx, Source{Name: "unclaimed-host", Address: "10.0.0.2", Transport: "tcp/601", Parser: "rfc5424", HeartbeatSec: 900})
	if err != nil {
		t.Fatalf("UpsertSource() error = %v", err)
	}
	if err := s.ClaimSource(ctx, created.ID); err != nil {
		t.Fatalf("ClaimSource() error = %v", err)
	}

	sources, err := s.ListSources(ctx)
	if err != nil {
		t.Fatalf("ListSources() error = %v", err)
	}
	if !sources[0].Claimed {
		t.Error("Claimed = false after ClaimSource()")
	}
}

func TestStaleSources(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if _, err := s.UpsertSource(ctx, Source{Name: "silent-host", Address: "10.0.0.3", Transport: "tcp/601", Parser: "rfc5424", HeartbeatSec: 60}); err != nil {
		t.Fatalf("UpsertSource() error = %v", err)
	}
	old := time.Now().UTC().Add(-2 * time.Hour)
	if err := s.TouchSourceLastSeen(ctx, "silent-host", old); err != nil {
		t.Fatalf("TouchSourceLastSeen() error = %v", err)
	}

	stale, err := s.StaleSources(ctx, time.Now().UTC())
	if err != nil {
		t.Fatalf("StaleSources() error = %v", err)
	}
	if len(stale) != 1 || stale[0].Name != "silent-host" {
		t.Fatalf("StaleSources() = %+v, want [silent-host]", stale)
	}
}
