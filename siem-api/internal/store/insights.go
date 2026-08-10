package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type Insight struct {
	ID           int64
	CreatedAt    time.Time
	Title        string
	Detail       string
	Severity     string // info/warning/critical
	Category     string
	EvidenceJSON string // JSON array, opaque to this package
	Dismissed    bool
}

func scanInsight(row rowScanner) (Insight, error) {
	var in Insight
	var dismissed int
	if err := row.Scan(&in.ID, scanTime(&in.CreatedAt), &in.Title, &in.Detail,
		&in.Severity, &in.Category, &in.EvidenceJSON, &dismissed); err != nil {
		return Insight{}, err
	}
	in.Dismissed = dismissed != 0
	return in, nil
}

func (s *Store) InsertInsight(ctx context.Context, in Insight) (Insight, error) {
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO insights (created_at, title, detail, severity, category, evidence_json, dismissed)
		VALUES (?, ?, ?, ?, ?, ?, 0)
	`, formatTime(time.Now()), in.Title, in.Detail, in.Severity, in.Category, in.EvidenceJSON)
	if err != nil {
		return Insight{}, fmt.Errorf("store: insert insight: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Insight{}, fmt.Errorf("store: insert insight: %w", err)
	}
	return s.GetInsight(ctx, id)
}

func (s *Store) GetInsight(ctx context.Context, id int64) (Insight, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, created_at, title, detail, severity, category, evidence_json, dismissed
		FROM insights WHERE id = ?
	`, id)
	in, err := scanInsight(row)
	if err != nil {
		return Insight{}, fmt.Errorf("store: get insight: %w", err)
	}
	return in, nil
}

func (s *Store) ListInsights(ctx context.Context, includeDismissed bool, limit int) ([]Insight, error) {
	query := `
		SELECT id, created_at, title, detail, severity, category, evidence_json, dismissed
		FROM insights
	`
	if !includeDismissed {
		query += ` WHERE dismissed = 0`
	}
	query += ` ORDER BY created_at DESC, id DESC LIMIT ?`

	rows, err := s.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("store: list insights: %w", err)
	}
	defer rows.Close()

	var out []Insight
	for rows.Next() {
		in, err := scanInsight(rows)
		if err != nil {
			return nil, fmt.Errorf("store: list insights: %w", err)
		}
		out = append(out, in)
	}
	return out, rows.Err()
}

func (s *Store) DismissInsight(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `UPDATE insights SET dismissed = 1 WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: dismiss insight: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: dismiss insight: %w", err)
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
