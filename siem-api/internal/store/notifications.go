package store

import (
	"context"
	"fmt"
)

func (s *Store) GetMinNotifySeverity(ctx context.Context) (string, error) {
	var severity string
	err := s.db.QueryRowContext(ctx, `SELECT min_severity FROM notification_settings WHERE id = 1`).Scan(&severity)
	if err != nil {
		return "", fmt.Errorf("store: get min notify severity: %w", err)
	}
	return severity, nil
}

func (s *Store) SetMinNotifySeverity(ctx context.Context, severity string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO notification_settings (id, min_severity) VALUES (1, ?)
		ON CONFLICT(id) DO UPDATE SET min_severity = excluded.min_severity
	`, severity)
	if err != nil {
		return fmt.Errorf("store: set min notify severity: %w", err)
	}
	return nil
}
