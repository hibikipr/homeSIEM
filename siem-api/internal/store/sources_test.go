package store

import (
	"context"
	"database/sql"
	"errors"
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

func TestRenameSource_SetAndClear(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	created, err := s.UpsertSource(ctx, Source{Name: "192.168.3.223", Address: "192.168.3.223", Transport: "tcp/601", Parser: "rfc5424", HeartbeatSec: 900})
	if err != nil {
		t.Fatalf("UpsertSource() error = %v", err)
	}
	if created.DisplayName != "" {
		t.Errorf("DisplayName on fresh insert = %q, want empty", created.DisplayName)
	}

	if err := s.RenameSource(ctx, created.ID, "Home Assistant"); err != nil {
		t.Fatalf("RenameSource() error = %v", err)
	}
	sources, err := s.ListSources(ctx)
	if err != nil {
		t.Fatalf("ListSources() error = %v", err)
	}
	if sources[0].DisplayName != "Home Assistant" {
		t.Errorf("DisplayName = %q, want %q", sources[0].DisplayName, "Home Assistant")
	}
	// The natural key must be untouched - it's what future heartbeats for
	// this address match against.
	if sources[0].Name != "192.168.3.223" {
		t.Errorf("Name = %q, want unchanged %q", sources[0].Name, "192.168.3.223")
	}

	if err := s.RenameSource(ctx, created.ID, ""); err != nil {
		t.Fatalf("RenameSource(clear) error = %v", err)
	}
	sources, err = s.ListSources(ctx)
	if err != nil {
		t.Fatalf("ListSources() error = %v", err)
	}
	if sources[0].DisplayName != "" {
		t.Errorf("DisplayName after clearing = %q, want empty", sources[0].DisplayName)
	}
}

func TestRenameSource_UnknownID_ReturnsErrNoRows(t *testing.T) {
	s := newTestStore(t)
	err := s.RenameSource(context.Background(), 999, "Anything")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("RenameSource(unknown) error = %v, want sql.ErrNoRows", err)
	}
}

// TestUpsertSource_PreservesDisplayNameAcrossHeartbeats guards the exact
// bug a naive "just UPDATE the name column" rename would hit: every
// incoming heartbeat re-upserts by the natural `name` key, and must not
// silently wipe an operator-set display_name back to blank.
func TestUpsertSource_PreservesDisplayNameAcrossHeartbeats(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	created, err := s.UpsertSource(ctx, Source{Name: "192.168.3.223", Address: "192.168.3.223", Transport: "tcp/601", Parser: "rfc5424", HeartbeatSec: 900})
	if err != nil {
		t.Fatalf("UpsertSource() error = %v", err)
	}
	if err := s.RenameSource(ctx, created.ID, "Home Assistant"); err != nil {
		t.Fatalf("RenameSource() error = %v", err)
	}

	// Simulate the next heartbeat for the same address.
	if _, err := s.UpsertSource(ctx, Source{Name: "192.168.3.223", Address: "192.168.3.223", Transport: "tcp/601", Parser: "rfc5424", HeartbeatSec: 900}); err != nil {
		t.Fatalf("second UpsertSource() error = %v", err)
	}

	sources, err := s.ListSources(ctx)
	if err != nil {
		t.Fatalf("ListSources() error = %v", err)
	}
	if len(sources) != 1 {
		t.Fatalf("len(sources) = %d, want 1 (re-upsert must match the existing row, not create a second one)", len(sources))
	}
	if sources[0].DisplayName != "Home Assistant" {
		t.Errorf("DisplayName after re-upsert = %q, want it preserved as %q", sources[0].DisplayName, "Home Assistant")
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
