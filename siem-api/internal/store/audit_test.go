package store

import (
	"context"
	"path/filepath"
	"testing"
)

func newTestStore(t *testing.T) *Store {
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
	// Create a test user for FK references
	if _, err := db.Exec(`INSERT INTO users (id, email, role) VALUES (1, 'test@test.com', 'admin')`); err != nil {
		t.Fatalf("create test user: %v", err)
	}
	return New(db)
}

func TestWriteAuditAndList(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx() error = %v", err)
	}
	target := "rule:1"
	if err := writeAudit(tx, AuditEntry{Action: "rule.create", Target: &target, Detail: `{"name":"wan-portscan"}`}); err != nil {
		t.Fatalf("writeAudit() error = %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit() error = %v", err)
	}

	entries, err := s.ListAudit(ctx, 10)
	if err != nil {
		t.Fatalf("ListAudit() error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("len(entries) = %d, want 1", len(entries))
	}
	if entries[0].Action != "rule.create" {
		t.Errorf("Action = %q, want rule.create", entries[0].Action)
	}
	if entries[0].Target == nil || *entries[0].Target != "rule:1" {
		t.Errorf("Target = %v, want rule:1", entries[0].Target)
	}
}

func TestWriteAudit_RolledBackNotListed(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx() error = %v", err)
	}
	if err := writeAudit(tx, AuditEntry{Action: "rule.create", Detail: "{}"}); err != nil {
		t.Fatalf("writeAudit() error = %v", err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("Rollback() error = %v", err)
	}

	entries, err := s.ListAudit(ctx, 10)
	if err != nil {
		t.Fatalf("ListAudit() error = %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("len(entries) = %d, want 0 after rollback", len(entries))
	}
}
