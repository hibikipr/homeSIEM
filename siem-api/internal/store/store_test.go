package store

import (
	"path/filepath"
	"testing"
)

func TestOpenAndMigrate(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "siem.db")
	db, err := Open("sqlite://" + dbPath)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	tables := []string{"sources", "rules", "alerts", "alert_samples", "users",
		"role_mappings", "saved_searches", "audit", "seen_values"}
	for _, table := range tables {
		var name string
		err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&name)
		if err != nil {
			t.Errorf("table %q not found after Migrate(): %v", table, err)
		}
	}
}

func TestMigrate_Idempotent(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "siem.db")
	db, err := Open("sqlite://" + dbPath)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	if err := Migrate(db); err != nil {
		t.Fatalf("first Migrate() error = %v", err)
	}
	if err := Migrate(db); err != nil {
		t.Fatalf("second Migrate() error = %v, want nil (must be idempotent)", err)
	}
}

func TestMigrate_AddsNotificationSettingsToExistingDatabase(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "siem.db")
	db, err := Open("sqlite://" + dbPath)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	// Simulate a database that predates this change: `sources` already
	// exists (so the one-time schema.sql bootstrap in Migrate no-ops), but
	// notification_settings does not. This is the exact scenario that would
	// have silently broken on an already-deployed database.
	if _, err := db.Exec(`CREATE TABLE sources (id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatalf("create sources: %v", err)
	}

	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	var severity string
	err = db.QueryRow(`SELECT min_severity FROM notification_settings WHERE id = 1`).Scan(&severity)
	if err != nil {
		t.Fatalf("notification_settings row missing after Migrate(): %v", err)
	}
	if severity != "info" {
		t.Errorf("min_severity = %q, want info (the default)", severity)
	}
}

func TestMigrate_NotificationSettingsIdempotent(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "siem.db")
	db, err := Open("sqlite://" + dbPath)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	if err := Migrate(db); err != nil {
		t.Fatalf("first Migrate() error = %v", err)
	}
	if err := Migrate(db); err != nil {
		t.Fatalf("second Migrate() error = %v", err)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM notification_settings`).Scan(&count); err != nil {
		t.Fatalf("count notification_settings: %v", err)
	}
	if count != 1 {
		t.Errorf("notification_settings row count = %d, want 1 (INSERT OR IGNORE must not duplicate)", count)
	}
}
