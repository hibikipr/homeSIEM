package alerts

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/hibikipr/homeSIEM/siem-api/internal/sse"
	"github.com/hibikipr/homeSIEM/siem-api/internal/store"
)

func TestRaise_AckThenReoccur_NoUniqueConstraintViolation(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "siem.db")
	db, err := store.Open("sqlite://" + dbPath)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()
	if err := store.Migrate(db); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	// AckAlert's acked_by column FK-references users(id); create the user it acks as.
	if _, err := db.Exec(`INSERT INTO users (id, email, role) VALUES (1, 'test@test.com', 'admin')`); err != nil {
		t.Fatalf("create test user: %v", err)
	}
	st := store.New(db)
	ctx := context.Background()

	rule, err := st.CreateRule(ctx, store.Rule{
		Name: "r", Shape: "threshold", Severity: "critical",
		Destinations: []string{"inapp"}, CooldownSec: 0, IntervalSec: 60, Enabled: true,
	}, nil)
	if err != nil {
		t.Fatalf("CreateRule() error = %v", err)
	}

	hub := sse.NewHub()
	svc := NewService(st, hub, nil, testLogger())

	// First occurrence.
	if err := svc.Raise(ctx, Candidate{RuleID: rule.ID, GroupKey: "10.0.0.5", Severity: "critical", Title: "t", Body: "b"}); err != nil {
		t.Fatalf("first Raise() error = %v", err)
	}

	alertsList, err := st.ListAlerts(ctx, "open")
	if err != nil {
		t.Fatalf("ListAlerts() error = %v", err)
	}
	if len(alertsList) != 1 {
		t.Fatalf("open alerts = %d, want 1", len(alertsList))
	}

	// Ack it.
	if err := st.AckAlert(ctx, alertsList[0].ID, 1, time.Now().UTC()); err != nil {
		t.Fatalf("AckAlert() error = %v", err)
	}

	// Same candidate fires again — must reuse the same row, not insert a second one.
	if err := svc.Raise(ctx, Candidate{RuleID: rule.ID, GroupKey: "10.0.0.5", Severity: "critical", Title: "t", Body: "b"}); err != nil {
		t.Fatalf("second Raise() error = %v", err)
	}

	reopened, err := st.ListAlerts(ctx, "open")
	if err != nil {
		t.Fatalf("ListAlerts() error = %v", err)
	}
	if len(reopened) != 1 || reopened[0].ID != alertsList[0].ID {
		t.Fatalf("reopened = %+v, want exactly the original alert id=%d back in open state", reopened, alertsList[0].ID)
	}

	// Ack it again — this must NOT fail with a UNIQUE constraint error.
	if err := st.AckAlert(ctx, reopened[0].ID, 1, time.Now().UTC()); err != nil {
		t.Fatalf("second AckAlert() error = %v (this is the bug: acking a recurring alert must never fail)", err)
	}
}
