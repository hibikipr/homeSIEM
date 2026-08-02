package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"time"
)

type Rule struct {
	ID           int64
	Name         string
	Shape        string
	LogQL        string
	WindowSec    int
	Threshold    *int
	GroupBy      []string
	Severity     string
	Destinations []string
	CooldownSec  int
	IntervalSec  int
	Enabled      bool
	LastRunAt    *time.Time
	CreatedBy    *int64
	CreatedAt    time.Time
}

func (s *Store) CreateRule(ctx context.Context, r Rule, actorUserID *int64) (Rule, error) {
	groupBy, err := json.Marshal(r.GroupBy)
	if err != nil {
		return Rule{}, err
	}
	dests, err := json.Marshal(r.Destinations)
	if err != nil {
		return Rule{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Rule{}, err
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx, `
		INSERT INTO rules (name, shape, logql, window_sec, threshold, group_by, severity,
			destinations, cooldown_sec, interval_sec, enabled, created_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, r.Name, r.Shape, r.LogQL, r.WindowSec, r.Threshold, string(groupBy), r.Severity,
		string(dests), r.CooldownSec, r.IntervalSec, r.Enabled, r.CreatedBy)
	if err != nil {
		return Rule{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Rule{}, err
	}

	target := ruleTarget(id)
	detail, _ := json.Marshal(map[string]any{"name": r.Name, "shape": r.Shape})
	if err := writeAudit(tx, AuditEntry{UserID: actorUserID, Action: "rule.create", Target: &target, Detail: string(detail)}); err != nil {
		return Rule{}, err
	}
	if err := tx.Commit(); err != nil {
		return Rule{}, err
	}

	return s.GetRule(ctx, id)
}

func (s *Store) UpdateRule(ctx context.Context, r Rule, actorUserID *int64) (Rule, error) {
	groupBy, err := json.Marshal(r.GroupBy)
	if err != nil {
		return Rule{}, err
	}
	dests, err := json.Marshal(r.Destinations)
	if err != nil {
		return Rule{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Rule{}, err
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, `
		UPDATE rules SET name=?, shape=?, logql=?, window_sec=?, threshold=?, group_by=?,
			severity=?, destinations=?, cooldown_sec=?, interval_sec=?, enabled=?
		WHERE id=?
	`, r.Name, r.Shape, r.LogQL, r.WindowSec, r.Threshold, string(groupBy), r.Severity,
		string(dests), r.CooldownSec, r.IntervalSec, r.Enabled, r.ID)
	if err != nil {
		return Rule{}, err
	}

	target := ruleTarget(r.ID)
	detail, _ := json.Marshal(map[string]any{"name": r.Name, "enabled": r.Enabled})
	if err := writeAudit(tx, AuditEntry{UserID: actorUserID, Action: "rule.update", Target: &target, Detail: string(detail)}); err != nil {
		return Rule{}, err
	}
	if err := tx.Commit(); err != nil {
		return Rule{}, err
	}

	return s.GetRule(ctx, r.ID)
}

func (s *Store) DeleteRule(ctx context.Context, id int64, actorUserID *int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM rules WHERE id=?`, id); err != nil {
		return err
	}

	target := ruleTarget(id)
	if err := writeAudit(tx, AuditEntry{UserID: actorUserID, Action: "rule.delete", Target: &target, Detail: "{}"}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) GetRule(ctx context.Context, id int64) (Rule, error) {
	row := s.db.QueryRowContext(ctx, ruleSelect+` WHERE id = ?`, id)
	r, err := scanRule(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Rule{}, err
	}
	return r, err
}

func (s *Store) ListRules(ctx context.Context) ([]Rule, error) {
	return s.queryRules(ctx, ruleSelect+` ORDER BY name`)
}

func (s *Store) ListEnabledRules(ctx context.Context) ([]Rule, error) {
	return s.queryRules(ctx, ruleSelect+` WHERE enabled = 1 ORDER BY name`)
}

func (s *Store) TouchRuleLastRun(ctx context.Context, id int64, at time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE rules SET last_run_at = ? WHERE id = ?`, formatTime(at), id)
	return err
}

const ruleSelect = `SELECT id, name, shape, logql, window_sec, threshold, group_by, severity,
	destinations, cooldown_sec, interval_sec, enabled, last_run_at, created_by, created_at
	FROM rules`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanRule(row rowScanner) (Rule, error) {
	var r Rule
	var groupBy, dests string
	if err := row.Scan(&r.ID, &r.Name, &r.Shape, &r.LogQL, &r.WindowSec, &r.Threshold,
		&groupBy, &r.Severity, &dests, &r.CooldownSec, &r.IntervalSec, &r.Enabled,
		scanNullTime(&r.LastRunAt), &r.CreatedBy, scanTime(&r.CreatedAt)); err != nil {
		return Rule{}, err
	}
	if err := json.Unmarshal([]byte(groupBy), &r.GroupBy); err != nil {
		return Rule{}, err
	}
	if err := json.Unmarshal([]byte(dests), &r.Destinations); err != nil {
		return Rule{}, err
	}
	return r, nil
}

func (s *Store) queryRules(ctx context.Context, query string) ([]Rule, error) {
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Rule
	for rows.Next() {
		r, err := scanRule(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func ruleTarget(id int64) string {
	return "rule:" + strconv.FormatInt(id, 10)
}

func strconvItoa(id int64) string {
	return strconv.FormatInt(id, 10)
}
