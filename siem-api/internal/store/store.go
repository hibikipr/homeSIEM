package store

import (
	"database/sql"
	_ "embed"
	"fmt"
	"strings"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaSQL string

//go:embed migrations.sql
var migrationsSQL string

type Store struct {
	db *sql.DB
}

func New(db *sql.DB) *Store {
	return &Store{db: db}
}

// Open accepts a DATABASE_URL of the form "sqlite:///path/to/file.db"
// (query params, if any, are ignored — pragmas are applied explicitly
// below rather than via DSN, since that's portable across driver versions).
func Open(databaseURL string) (*sql.DB, error) {
	path := strings.TrimPrefix(databaseURL, "sqlite://")
	if idx := strings.IndexByte(path, '?'); idx >= 0 {
		path = path[:idx]
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("store: open %s: %w", path, err)
	}

	for _, pragma := range []string{
		"PRAGMA journal_mode = WAL",
		"PRAGMA busy_timeout = 5000",
		"PRAGMA foreign_keys = ON",
	} {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, fmt.Errorf("store: %s: %w", pragma, err)
		}
	}

	// SQLite locking is unreliable across multiple connections issuing
	// concurrent writes; the schema is designed single-writer, so pin
	// the pool to one connection to avoid SQLITE_BUSY under load.
	db.SetMaxOpenConns(1)

	return db, nil
}

// Migrate applies schema.sql if the schema hasn't been created yet, then
// always applies migrations.sql - a set of individually-idempotent
// statements for anything added after the initial release. schema.sql only
// ever runs once (gated on `sources` not existing); migrations.sql runs on
// every call, including against an already-populated database, since
// schema.sql's one-time gate would otherwise silently skip new tables on
// any existing deployment.
func Migrate(db *sql.DB) error {
	var exists int
	err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='sources'`).Scan(&exists)
	if err != nil {
		return fmt.Errorf("store: check schema: %w", err)
	}
	if exists == 0 {
		if _, err := db.Exec(schemaSQL); err != nil {
			return fmt.Errorf("store: apply schema: %w", err)
		}
	}

	if _, err := db.Exec(migrationsSQL); err != nil {
		return fmt.Errorf("store: apply migrations: %w", err)
	}
	return nil
}
