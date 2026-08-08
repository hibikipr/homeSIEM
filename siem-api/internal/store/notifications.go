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
	if _, err := s.db.ExecContext(ctx, `UPDATE notification_settings SET min_severity = ? WHERE id = 1`, severity); err != nil {
		return fmt.Errorf("store: set min notify severity: %w", err)
	}
	return nil
}
