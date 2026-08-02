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
