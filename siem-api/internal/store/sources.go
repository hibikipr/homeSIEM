package store

import (
	"context"
	"database/sql"
	"time"
)

type Source struct {
	ID           int64
	Name         string
	Address      string
	Transport    string
	Parser       string
	Claimed      bool
	HeartbeatSec int
	LastSeenAt   *time.Time
	CreatedAt    time.Time
}

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

	var out Source
	err = s.db.QueryRowContext(ctx,
		`SELECT id, name, address, transport, parser, claimed, heartbeat_sec, last_seen_at, created_at
		 FROM sources WHERE name = ?`, src.Name).
		Scan(&out.ID, &out.Name, &out.Address, &out.Transport, &out.Parser,
			&out.Claimed, &out.HeartbeatSec, scanNullTime(&out.LastSeenAt), scanTime(&out.CreatedAt))
	return out, err
}

func (s *Store) ListSources(ctx context.Context) ([]Source, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, address, transport, parser, claimed, heartbeat_sec, last_seen_at, created_at
		 FROM sources ORDER BY name`)
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

func (s *Store) TouchSourceLastSeen(ctx context.Context, name string, at time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE sources SET last_seen_at = ? WHERE name = ?`, formatTime(at), name)
	return err
}

func (s *Store) ClaimSource(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE sources SET claimed = 1 WHERE id = ?`, id)
	return err
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
