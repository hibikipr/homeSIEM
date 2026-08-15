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

	for _, col := range insightColumns {
		if err := addColumnIfMissing(db, "insights", col.name, col.ddl); err != nil {
			return fmt.Errorf("store: add insights column: %w", err)
		}
	}
	if err := backfillInsightFingerprints(db); err != nil {
		return fmt.Errorf("store: backfill insight fingerprints: %w", err)
	}

	for _, col := range mutedFingerprintColumns {
		if err := addColumnIfMissing(db, "muted_insight_fingerprints", col.name, col.ddl); err != nil {
			return fmt.Errorf("store: add muted_insight_fingerprints column: %w", err)
		}
	}

	for _, col := range sourceColumns {
		if err := addColumnIfMissing(db, "sources", col.name, col.ddl); err != nil {
			return fmt.Errorf("store: add sources column: %w", err)
		}
	}

	return nil
}

// insightColumns are added to the insights table after its initial
// migrations.sql CREATE TABLE - unlike that file's statements, they can't
// be expressed as an idempotent CREATE, since SQLite has no
// "ALTER TABLE ... ADD COLUMN IF NOT EXISTS" and a bare ADD COLUMN fails
// with "duplicate column name" on every startup after the first. Applied
// via addColumnIfMissing instead, which checks PRAGMA table_info first.
var insightColumns = []struct{ name, ddl string }{
	{"fingerprint", "ALTER TABLE insights ADD COLUMN fingerprint TEXT NOT NULL DEFAULT ''"},
	{"occurrence_count", "ALTER TABLE insights ADD COLUMN occurrence_count INTEGER NOT NULL DEFAULT 1"},
	{"last_seen_at", "ALTER TABLE insights ADD COLUMN last_seen_at TEXT NOT NULL DEFAULT ''"},
	{"recommended_fix", "ALTER TABLE insights ADD COLUMN recommended_fix TEXT NOT NULL DEFAULT ''"},
}

// mutedFingerprintColumns: same addColumnIfMissing treatment as
// insightColumns, for muted_insight_fingerprints - category+programs alone
// ("operational" / "UI-poller") turned out too coarse to remind anyone what
// they actually muted, since the mute scope is deliberately broader than
// any one insight's title (see ComputeFingerprint). example_title captures
// the specific insight's title at the moment it was muted, purely for
// display in the "Muted patterns" list - it plays no part in matching.
var mutedFingerprintColumns = []struct{ name, ddl string }{
	{"example_title", "ALTER TABLE muted_insight_fingerprints ADD COLUMN example_title TEXT NOT NULL DEFAULT ''"},
}

// sourceColumns: same addColumnIfMissing treatment as insightColumns.
// display_name is deliberately a separate column from `name`, not a
// rename of it - `name` is the natural key UpsertSource/TouchSourceLastSeen
// match incoming heartbeats against (see sources.go), always whatever the
// raw syslog HOSTNAME says (e.g. a bare IP, for senders that never set a
// real hostname). Overwriting `name` itself to a friendly label would
// desync it from every future heartbeat for that address, which would
// either stop updating last_seen_at on the renamed row or spawn a second,
// unclaimed row for the same address. display_name is purely a display
// override, never matched against.
var sourceColumns = []struct{ name, ddl string }{
	{"display_name", "ALTER TABLE sources ADD COLUMN display_name TEXT NOT NULL DEFAULT ''"},
}

// addColumnIfMissing runs ddl only if table doesn't already have column -
// see insightColumns for why this can't just be an idempotent CREATE.
func addColumnIfMissing(db *sql.DB, table, column, ddl string) error {
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return fmt.Errorf("table_info(%s): %w", table, err)
	}
	defer rows.Close()

	for rows.Next() {
		var cid, notnull, pk int
		var name, ctype string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return fmt.Errorf("table_info(%s) scan: %w", table, err)
		}
		if name == column {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	if _, err := db.Exec(ddl); err != nil {
		return fmt.Errorf("add column %s.%s: %w", table, column, err)
	}
	return nil
}
