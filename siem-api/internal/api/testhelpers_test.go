package api

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/hibikipr/homeSIEM/siem-api/internal/alerts"
	"github.com/hibikipr/homeSIEM/siem-api/internal/auth"
	"github.com/hibikipr/homeSIEM/siem-api/internal/sse"
	"github.com/hibikipr/homeSIEM/siem-api/internal/store"
)

const testSessionSecret = "0123456789abcdef0123456789abcdef"

// newTestServer builds a Server backed by a real temp-dir SQLite store (fast
// enough that faking it isn't worth it — see the design spec's testing
// section) and a real sse.Hub, with a nil ntfy notifier and nil Loki client;
// tasks that need Loki (24) or the scheduler (26) set those Deps fields
// themselves after calling this.
func newTestServer(t *testing.T) (*Server, *store.Store) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "siem.db")
	db, err := store.Open("sqlite://" + dbPath)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := store.Migrate(db); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	// Create a test user for FK references (e.g., audit entries)
	if _, err := db.Exec(`INSERT INTO users (id, email, role) VALUES (1, 'test@test.com', 'admin')`); err != nil {
		t.Fatalf("create test user: %v", err)
	}
	st := store.New(db)
	hub := sse.NewHub()
	alertsSvc := alerts.NewService(st, hub, nil, "", apiTestLogger())

	deps := Deps{
		Store:         st,
		Hub:           hub,
		Alerts:        alertsSvc,
		Verifier:      auth.NewTokenVerifier([]byte(testSessionSecret)),
		FastpathToken: "test-fastpath-token",
		JobLabel:      "siem",
		Logger:        apiTestLogger(),
	}
	return NewServer(deps), st
}

// authToken mints a token newTestServer's Verifier will accept, for a caller
// whose groups map to role via a role_mappings row this helper creates.
func authToken(t *testing.T, st *store.Store, role string, priority int) string {
	t.Helper()
	group := "test-group-" + role
	if _, err := st.UpsertRoleMapping(context.Background(), store.RoleMapping{
		GroupClaim: group, Role: role, Priority: priority,
	}); err != nil {
		t.Fatalf("UpsertRoleMapping() error = %v", err)
	}

	claims := struct {
		jwt.RegisteredClaims
		UserID int64    `json:"user_id"`
		Groups []string `json:"groups"`
	}{
		RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))},
		UserID:           1,
		Groups:           []string{group},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(testSessionSecret))
	if err != nil {
		t.Fatalf("SignedString() error = %v", err)
	}
	return token
}
