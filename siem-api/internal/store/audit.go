package store

import (
	"context"
	"database/sql"
	"time"
)

type AuditEntry struct {
	ID     int64
	TS     time.Time
	UserID *int64
	Action string
	Target *string
	Detail string
}

func writeAudit(tx *sql.Tx, e AuditEntry) error {
	_, err := tx.Exec(
		`INSERT INTO audit (user_id, action, target, detail) VALUES (?, ?, ?, ?)`,
		e.UserID, e.Action, e.Target, e.Detail,
	)
	return err
}

func (s *Store) ListAudit(ctx context.Context, limit int) ([]AuditEntry, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, ts, user_id, action, target, detail FROM audit ORDER BY ts DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []AuditEntry
	for rows.Next() {
		var e AuditEntry
		var ts string
		if err := rows.Scan(&e.ID, &ts, &e.UserID, &e.Action, &e.Target, &e.Detail); err != nil {
			return nil, err
		}
		e.TS, err = time.Parse("2006-01-02 15:04:05", ts)
		if err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}
