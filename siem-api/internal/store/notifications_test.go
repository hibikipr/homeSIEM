package store

import (
	"context"
	"path/filepath"
	"testing"
)

func newTestStoreForNotifications(t *testing.T) *Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "siem.db")
	db, err := Open("sqlite://" + dbPath)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	return New(db)
}

func TestGetMinNotifySeverity_DefaultsToInfo(t *testing.T) {
	s := newTestStoreForNotifications(t)

	got, err := s.GetMinNotifySeverity(context.Background())
	if err != nil {
		t.Fatalf("GetMinNotifySeverity() error = %v", err)
	}
	if got != "info" {
		t.Errorf("GetMinNotifySeverity() = %q, want info", got)
	}
}

func TestSetMinNotifySeverity_RoundTrips(t *testing.T) {
	s := newTestStoreForNotifications(t)
	ctx := context.Background()

	if err := s.SetMinNotifySeverity(ctx, "critical"); err != nil {
		t.Fatalf("SetMinNotifySeverity() error = %v", err)
	}

	got, err := s.GetMinNotifySeverity(ctx)
	if err != nil {
		t.Fatalf("GetMinNotifySeverity() error = %v", err)
	}
	if got != "critical" {
		t.Errorf("GetMinNotifySeverity() = %q, want critical", got)
	}
}
