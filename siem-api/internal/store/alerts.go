package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type Alert struct {
	ID          int64
	RuleID      int64
	GroupKey    string
	Severity    string
	Title       string
	Body        string
	EventCount  int
	Context     string
	State       string
	FirstSeenAt time.Time
	LastSeenAt  time.Time
	AckedBy     *int64
	AckedAt     *time.Time
	MutedUntil  *time.Time
}

type AlertSample struct {
	ID      int64
	AlertID int64
	TS      time.Time
	Line    string
}

const alertSelect = `SELECT id, rule_id, group_key, severity, title, body, event_count,
	context, state, first_seen_at, last_seen_at, acked_by, acked_at, muted_until FROM alerts`

func scanAlert(row rowScanner) (Alert, error) {
	var a Alert
	if err := row.Scan(&a.ID, &a.RuleID, &a.GroupKey, &a.Severity, &a.Title, &a.Body,
		&a.EventCount, &a.Context, &a.State, scanTime(&a.FirstSeenAt), scanTime(&a.LastSeenAt),
		&a.AckedBy, scanNullTime(&a.AckedAt), scanNullTime(&a.MutedUntil)); err != nil {
		return Alert{}, err
	}
	return a, nil
}

func (s *Store) FindOpenAlert(ctx context.Context, ruleID int64, groupKey string) (*Alert, error) {
	row := s.db.QueryRowContext(ctx,
		alertSelect+` WHERE rule_id = ? AND group_key = ? AND state = 'open'`, ruleID, groupKey)
	a, err := scanAlert(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (s *Store) FindLatestAlert(ctx context.Context, ruleID int64, groupKey string) (*Alert, error) {
	row := s.db.QueryRowContext(ctx,
		alertSelect+` WHERE rule_id = ? AND group_key = ? ORDER BY last_seen_at DESC LIMIT 1`, ruleID, groupKey)
	a, err := scanAlert(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (s *Store) InsertAlert(ctx context.Context, a Alert) (Alert, error) {
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO alerts (rule_id, group_key, severity, title, body, event_count, context,
			state, first_seen_at, last_seen_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, a.RuleID, a.GroupKey, a.Severity, a.Title, a.Body, a.EventCount, a.Context, a.State,
		formatTime(a.FirstSeenAt), formatTime(a.LastSeenAt))
	if err != nil {
		return Alert{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Alert{}, err
	}
	return s.GetAlert(ctx, id)
}

func (s *Store) TouchAlert(ctx context.Context, id int64, at time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE alerts SET event_count = event_count + 1, last_seen_at = ? WHERE id = ?`,
		formatTime(at), id)
	return err
}

func (s *Store) ReopenAlert(ctx context.Context, id int64, at time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE alerts SET state = 'open', last_seen_at = ?, event_count = 1, acked_by = NULL, acked_at = NULL, muted_until = NULL WHERE id = ?`,
		formatTime(at), id)
	return err
}

func (s *Store) AddAlertSample(ctx context.Context, alertID int64, ts time.Time, line string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO alert_samples (alert_id, ts, line) VALUES (?, ?, ?)`,
		alertID, formatTime(ts), line); err != nil {
		return err
	}

	// Keep only the 10 most recent samples for this alert.
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM alert_samples WHERE alert_id = ? AND id NOT IN (
			SELECT id FROM alert_samples WHERE alert_id = ? ORDER BY ts DESC LIMIT 10
		)
	`, alertID, alertID); err != nil {
		return err
	}

	return tx.Commit()
}

func (s *Store) ListAlertSamples(ctx context.Context, alertID int64) ([]AlertSample, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, alert_id, ts, line FROM alert_samples WHERE alert_id = ? ORDER BY ts DESC`, alertID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []AlertSample
	for rows.Next() {
		var sample AlertSample
		if err := rows.Scan(&sample.ID, &sample.AlertID, scanTime(&sample.TS), &sample.Line); err != nil {
			return nil, err
		}
		out = append(out, sample)
	}
	return out, rows.Err()
}

func (s *Store) AckAlert(ctx context.Context, id int64, userID int64, at time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx,
		`UPDATE alerts SET state = 'acked', acked_by = ?, acked_at = ? WHERE id = ?`,
		userID, formatTime(at), id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}

	target := "alert:" + strconvItoa(id)
	uid := userID
	if err := writeAudit(tx, AuditEntry{UserID: &uid, Action: "alert.ack", Target: &target, Detail: "{}"}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) MuteAlert(ctx context.Context, id int64, userID int64, until time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx,
		`UPDATE alerts SET state = 'muted', muted_until = ? WHERE id = ?`,
		formatTime(until), id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}

	target := "alert:" + strconvItoa(id)
	uid := userID
	if err := writeAudit(tx, AuditEntry{UserID: &uid, Action: "alert.mute", Target: &target, Detail: "{}"}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) GetAlert(ctx context.Context, id int64) (Alert, error) {
	row := s.db.QueryRowContext(ctx, alertSelect+` WHERE id = ?`, id)
	return scanAlert(row)
}

func (s *Store) ListAlerts(ctx context.Context, state string) ([]Alert, error) {
	query := alertSelect
	var args []any
	if state != "" {
		query += ` WHERE state = ?`
		args = append(args, state)
	}
	query += ` ORDER BY last_seen_at DESC`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Alert
	for rows.Next() {
		a, err := scanAlert(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
