package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

type Insight struct {
	ID              int64
	CreatedAt       time.Time
	Title           string
	Detail          string
	Severity        string // info/warning/critical
	Category        string
	EvidenceJSON    string // JSON array, opaque to this package
	Dismissed       bool
	Fingerprint     string
	OccurrenceCount int
	LastSeenAt      time.Time
}

func scanInsight(row rowScanner) (Insight, error) {
	var in Insight
	var dismissed int
	if err := row.Scan(&in.ID, scanTime(&in.CreatedAt), &in.Title, &in.Detail,
		&in.Severity, &in.Category, &in.EvidenceJSON, &dismissed,
		&in.Fingerprint, &in.OccurrenceCount, scanTime(&in.LastSeenAt)); err != nil {
		return Insight{}, err
	}
	in.Dismissed = dismissed != 0
	return in, nil
}

const insightColumnList = `id, created_at, title, detail, severity, category, evidence_json, dismissed, fingerprint, occurrence_count, last_seen_at`

// ComputeFingerprint derives a stable identity for "the same underlying
// finding recurring" from category plus the set of programs in its
// evidence - the parts of an insight the model didn't freely author, unlike
// title/detail text, which can vary pass to pass (even at the low
// temperature configured for this task) for what a human would call the
// same finding. Programs are deduped and sorted so evidence ordering
// doesn't change the result.
func ComputeFingerprint(category string, programs []string) string {
	uniq := make(map[string]struct{}, len(programs))
	for _, p := range programs {
		if p != "" {
			uniq[p] = struct{}{}
		}
	}
	sorted := make([]string, 0, len(uniq))
	for p := range uniq {
		sorted = append(sorted, p)
	}
	sort.Strings(sorted)
	sum := sha256.Sum256([]byte(category + "|" + strings.Join(sorted, ",")))
	return hex.EncodeToString(sum[:])[:16]
}

// InsertInsight creates a new insight with occurrence_count=1. Callers that
// already hold a fingerprint (Service.GenerateNow, after checking
// FindMostRecentInsightByFingerprint found nothing) should set in.Fingerprint;
// an empty fingerprint is valid too (e.g. tests not exercising dedup).
func (s *Store) InsertInsight(ctx context.Context, in Insight) (Insight, error) {
	now := formatTime(time.Now())
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO insights (created_at, title, detail, severity, category, evidence_json, dismissed, fingerprint, occurrence_count, last_seen_at)
		VALUES (?, ?, ?, ?, ?, ?, 0, ?, 1, ?)
	`, now, in.Title, in.Detail, in.Severity, in.Category, in.EvidenceJSON, in.Fingerprint, now)
	if err != nil {
		return Insight{}, fmt.Errorf("store: insert insight: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Insight{}, fmt.Errorf("store: insert insight: %w", err)
	}
	return s.GetInsight(ctx, id)
}

// BumpInsight records a repeat occurrence of an existing insight:
// increments occurrence_count, refreshes last_seen_at/detail/severity/
// evidence_json to the latest pass's version (so the row doesn't show stale
// text from whenever it first appeared), and un-dismisses it. A recurrence
// after being dismissed is new information worth re-surfacing, same as a
// first-time hit - only an explicit mute (see MuteFingerprint) suppresses a
// fingerprint regardless of recurrence.
func (s *Store) BumpInsight(ctx context.Context, id int64, detail, severity, evidenceJSON string) (Insight, error) {
	_, err := s.db.ExecContext(ctx, `
		UPDATE insights
		SET occurrence_count = occurrence_count + 1,
		    last_seen_at = ?,
		    detail = ?,
		    severity = ?,
		    evidence_json = ?,
		    dismissed = 0
		WHERE id = ?
	`, formatTime(time.Now()), detail, severity, evidenceJSON, id)
	if err != nil {
		return Insight{}, fmt.Errorf("store: bump insight: %w", err)
	}
	return s.GetInsight(ctx, id)
}

// FindMostRecentInsightByFingerprint returns the newest insight (dismissed
// or not) matching fingerprint, so GenerateNow can bump it instead of
// inserting a duplicate row. The bool return is false, not an error, when
// no such row exists - "no prior occurrence" is an expected outcome, not a
// failure.
func (s *Store) FindMostRecentInsightByFingerprint(ctx context.Context, fingerprint string) (Insight, bool, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT `+insightColumnList+`
		FROM insights WHERE fingerprint = ? ORDER BY created_at DESC, id DESC LIMIT 1
	`, fingerprint)
	in, err := scanInsight(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return Insight{}, false, nil
		}
		return Insight{}, false, fmt.Errorf("store: find insight by fingerprint: %w", err)
	}
	return in, true, nil
}

func (s *Store) GetInsight(ctx context.Context, id int64) (Insight, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT `+insightColumnList+`
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
		SELECT ` + insightColumnList + `
		FROM insights
	`
	if !includeDismissed {
		query += ` WHERE dismissed = 0`
	}
	query += ` ORDER BY last_seen_at DESC, id DESC LIMIT ?`

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

// --------------------------------------------------------- muted fingerprints

type MutedFingerprint struct {
	Fingerprint string
	Category    string
	Programs    string // comma-joined, for display in a "manage mutes" UI
	MutedAt     time.Time
}

func (s *Store) IsFingerprintMuted(ctx context.Context, fingerprint string) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM muted_insight_fingerprints WHERE fingerprint = ?`, fingerprint,
	).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("store: check muted fingerprint: %w", err)
	}
	return n > 0, nil
}

// MuteFingerprint is idempotent (INSERT OR REPLACE) so muting an
// already-muted fingerprint just refreshes muted_at rather than erroring.
func (s *Store) MuteFingerprint(ctx context.Context, fingerprint, category, programs string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT OR REPLACE INTO muted_insight_fingerprints (fingerprint, category, programs, muted_at)
		VALUES (?, ?, ?, ?)
	`, fingerprint, category, programs, formatTime(time.Now()))
	if err != nil {
		return fmt.Errorf("store: mute fingerprint: %w", err)
	}
	return nil
}

func (s *Store) UnmuteFingerprint(ctx context.Context, fingerprint string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM muted_insight_fingerprints WHERE fingerprint = ?`, fingerprint)
	if err != nil {
		return fmt.Errorf("store: unmute fingerprint: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: unmute fingerprint: %w", err)
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) ListMutedFingerprints(ctx context.Context) ([]MutedFingerprint, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT fingerprint, category, programs, muted_at
		FROM muted_insight_fingerprints
		ORDER BY muted_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("store: list muted fingerprints: %w", err)
	}
	defer rows.Close()

	var out []MutedFingerprint
	for rows.Next() {
		var m MutedFingerprint
		if err := rows.Scan(&m.Fingerprint, &m.Category, &m.Programs, scanTime(&m.MutedAt)); err != nil {
			return nil, fmt.Errorf("store: list muted fingerprints: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// --------------------------------------------------------- migration backfill

// evidenceProgram is just enough of the evidence JSON shape (see
// api.evidenceItem / insights.modelEvidence) to read .program back out for
// backfillInsightFingerprints - this package doesn't otherwise care about
// evidence structure, which is why InsightStore treats evidence_json as
// opaque everywhere else.
type evidenceProgram struct {
	Program string `json:"program"`
}

// backfillInsightFingerprints fills in fingerprint/last_seen_at for rows
// created before this feature existed, so a fresh occurrence of a
// pre-existing finding finds and bumps the newest matching row instead of
// starting a new fingerprint lineage. Cheap no-op once every row has both
// fields set (the WHERE clause only ever matches rows migrated once).
// Deliberately does NOT merge pre-existing duplicate rows sharing a
// backfilled fingerprint into one - that's a one-time data cleanup, not a
// migration concern, and out of scope here.
func backfillInsightFingerprints(db *sql.DB) error {
	rows, err := db.Query(`SELECT id, category, evidence_json, created_at FROM insights WHERE fingerprint = '' OR last_seen_at = ''`)
	if err != nil {
		return fmt.Errorf("select unbackfilled insights: %w", err)
	}
	type pending struct {
		id                     int64
		fingerprint, createdAt string
	}
	var toUpdate []pending
	for rows.Next() {
		var id int64
		var category, evidenceJSON, createdAt string
		if err := rows.Scan(&id, &category, &evidenceJSON, &createdAt); err != nil {
			rows.Close()
			return fmt.Errorf("scan unbackfilled insight: %w", err)
		}
		var evidence []evidenceProgram
		_ = json.Unmarshal([]byte(evidenceJSON), &evidence) // best-effort, same as toInsightResponse
		programs := make([]string, 0, len(evidence))
		for _, e := range evidence {
			programs = append(programs, e.Program)
		}
		toUpdate = append(toUpdate, pending{id, ComputeFingerprint(category, programs), createdAt})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterate unbackfilled insights: %w", err)
	}
	rows.Close()

	for _, p := range toUpdate {
		if _, err := db.Exec(
			`UPDATE insights SET fingerprint = ?, last_seen_at = ? WHERE id = ? AND (fingerprint = '' OR last_seen_at = '')`,
			p.fingerprint, p.createdAt, p.id,
		); err != nil {
			return fmt.Errorf("backfill insight %d: %w", p.id, err)
		}
	}
	return nil
}
