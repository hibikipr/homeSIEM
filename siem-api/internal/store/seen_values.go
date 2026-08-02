package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

func (s *Store) HasSeenValue(ctx context.Context, ruleID int64, value string) (bool, error) {
	var id int64
	err := s.db.QueryRowContext(ctx,
		`SELECT id FROM seen_values WHERE rule_id = ? AND value = ?`, ruleID, value).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) MarkSeenValue(ctx context.Context, ruleID int64, value string, at time.Time) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO seen_values (rule_id, value, first_seen_at) VALUES (?, ?, ?)
		ON CONFLICT(rule_id, value) DO NOTHING
	`, ruleID, value, formatTime(at))
	return err
}
