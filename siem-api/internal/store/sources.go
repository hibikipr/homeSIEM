package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type Source struct {
	ID           int64
	Name         string
	DisplayName  string // operator-set label, shown in place of Name when non-empty; never matched against
	Address      string
	Transport    string
	Parser       string
	Claimed      bool
	HeartbeatSec int
	LastSeenAt   *time.Time
	CreatedAt    time.Time
}

const sourceColumnList = `id, name, display_name, address, transport, parser, claimed, heartbeat_sec, last_seen_at, created_at`

func scanSource(row rowScanner) (Source, error) {
	var src Source
	if err := row.Scan(&src.ID, &src.Name, &src.DisplayName, &src.Address, &src.Transport, &src.Parser,
		&src.Claimed, &src.HeartbeatSec, scanNullTime(&src.LastSeenAt), scanTime(&src.CreatedAt)); err != nil {
		return Source{}, err
	}
	return src, nil
}

// UpsertSource deliberately never touches display_name - it's an operator
// override (see RenameSource), and a heartbeat re-upsert (every claim's
// worth of incoming events) must not clobber it back to blank, the same
// reason `claimed` is left out of the ON CONFLICT SET below.
func (s *Store) UpsertSource(ctx context.Context, src Source) (Source, error) {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO sources (name, address, transport, parser, heartbeat_sec)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(name) DO UPDATE SET
			address = excluded.address,
			transport = excluded.transport,
			parser = excluded.parser,
			heartbeat_sec = excluded.heartbeat_sec
	`, src.Name, src.Address, src.Transport, src.Parser, src.HeartbeatSec)
	if err != nil {
		return Source{}, err
	}

	row := s.db.QueryRowContext(ctx, `SELECT `+sourceColumnList+` FROM sources WHERE name = ?`, src.Name)
	return scanSource(row)
}

func (s *Store) ListSources(ctx context.Context) ([]Source, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+sourceColumnList+` FROM sources ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Source
	for rows.Next() {
		src, err := scanSource(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, src)
	}
	return out, rows.Err()
}

func (s *Store) TouchSourceLastSeen(ctx context.Context, name string, at time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE sources SET last_seen_at = ? WHERE name = ?`, formatTime(at), name)
	return err
}

func (s *Store) ClaimSource(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE sources SET claimed = 1 WHERE id = ?`, id)
	return err
}

// RenameSource sets display_name - purely cosmetic, never matched against
// (see sourceColumns in store.go for why `name` itself can't just be
// edited directly). An empty displayName clears the override, falling
// back to showing `name` again.
func (s *Store) RenameSource(ctx context.Context, id int64, displayName string) error {
	res, err := s.db.ExecContext(ctx, `UPDATE sources SET display_name = ? WHERE id = ?`, displayName, id)
	if err != nil {
		return fmt.Errorf("store: rename source: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: rename source: %w", err)
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// DeleteSource removes a source row entirely. Note it isn't a permanent
// ban: the next heartbeat from a still-actively-sending sender re-creates
// it via UpsertSource's ON CONFLICT(name) upsert, as a fresh unclaimed row
// (new ID, no display_name) - the same behavior a device sending its very
// first heartbeat ever gets. That's expected for the real use case, which
// is clearing out silent/decommissioned sources, not preventing a live one
// from ever appearing again.
func (s *Store) DeleteSource(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM sources WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete source: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: delete source: %w", err)
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) StaleSources(ctx context.Context, now time.Time) ([]Source, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, address, transport, parser, claimed, heartbeat_sec, last_seen_at, created_at
		FROM sources
		WHERE last_seen_at IS NULL
		   OR (julianday(?) - julianday(last_seen_at)) * 86400 > heartbeat_sec
	`, formatTime(now))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Source
	for rows.Next() {
		var src Source
		if err := rows.Scan(&src.ID, &src.Name, &src.Address, &src.Transport, &src.Parser,
			&src.Claimed, &src.HeartbeatSec, scanNullTime(&src.LastSeenAt), scanTime(&src.CreatedAt)); err != nil {
			return nil, err
		}
		out = append(out, src)
	}
	return out, rows.Err()
}

// -- time helpers shared across store files --

func formatTime(t time.Time) string {
	return t.UTC().Format("2006-01-02 15:04:05")
}

func scanTime(dst *time.Time) *timeScanner {
	return &timeScanner{dst: dst}
}

func scanNullTime(dst **time.Time) *nullTimeScanner {
	return &nullTimeScanner{dst: dst}
}

type timeScanner struct{ dst *time.Time }

func (ts *timeScanner) Scan(src any) error {
	s, ok := src.(string)
	if !ok {
		return sql.ErrNoRows
	}
	t, err := time.Parse("2006-01-02 15:04:05", s)
	if err != nil {
		return err
	}
	*ts.dst = t
	return nil
}

type nullTimeScanner struct{ dst **time.Time }

func (ns *nullTimeScanner) Scan(src any) error {
	if src == nil {
		*ns.dst = nil
		return nil
	}
	s, ok := src.(string)
	if !ok {
		return sql.ErrNoRows
	}
	t, err := time.Parse("2006-01-02 15:04:05", s)
	if err != nil {
		return err
	}
	*ns.dst = &t
	return nil
}
