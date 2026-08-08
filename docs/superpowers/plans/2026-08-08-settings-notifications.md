# Settings → Notifications Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build out the previously-stubbed Notifications section of Settings — real ntfy connection status, a "Send test notification" button, and a persisted minimum-severity filter that controls which alerts actually page (not which appear in-app).

**Architecture:** A new `notification_settings` table (single row) holds the minimum-severity filter, added via a new always-run migration step (the existing one-time `schema.sql` bootstrap is a no-op against any already-deployed database). `alerts.Service.Raise()` checks that threshold immediately before the ntfy push, leaving the DB insert and in-app SSE publish unaffected. `siem-api` exposes `GET`/`PUT /settings/notifications` and `POST /settings/notifications/test` (admin-only, mirroring `/settings/auth`'s existing shape). `siem-web` proxies those through same-origin routes and renders the real Notifications section in place of today's generic placeholder.

**Tech Stack:** Go (siem-api, SQLite via modernc.org/sqlite), SvelteKit (Svelte 5 runes), TypeScript, Vitest.

## Global Constraints

- ntfy's connection settings (URL/topic/token) stay env-var-only — no UI editing, no database storage of deployment secrets. Only the minimum-severity filter is UI-editable and persisted.
- The minimum-severity filter gates the ntfy push only. Alerts always appear in-app (Wall/Alerts SSE stream) regardless of this setting.
- Real alert severity only ever takes the values `info`/`warning`/`critical` (the actual vocabulary — see `siem-web/src/lib/components/RuleFromEventForm.svelte`'s `<select>`). Any unrecognized value is treated as the lowest tier (`info`-equivalent), matching this codebase's existing convention.
- `store.Migrate()` only applies `schema.sql` once (gated on the `sources` table not existing) — it is a no-op against any already-populated database, including production. The new `notification_settings` table MUST be added via a separate, always-run, individually-idempotent step (`CREATE TABLE IF NOT EXISTS`) — never by editing `schema.sql` itself, which would silently never run on an existing deployment.
- A failure reading the minimum-severity threshold at notify time must fail OPEN (notify anyway), never silently drop a real notification — matches this codebase's existing "never let a defensive check drop something real" posture.
- This codebase has no Svelte component test infrastructure and none should be added (see the Nav-avatar-picture plan's precedent, `docs/superpowers/plans/2026-08-07-nav-avatar-picture.md`, for the reasoning). Cover Svelte-adjacent logic with plain Vitest unit tests; verify the new Settings UI manually via Playwright with a minted session cookie (same technique as that plan's Task 2), not a new component-test file.
- No changes to the other stubbed Settings sections (Retention, Parsers, Backups, About).

---

### Task 1: Migration mechanism + `notification_settings` table + store methods

**Files:**
- Modify: `siem-api/internal/store/store.go`
- Create: `siem-api/internal/store/migrations.sql`
- Create: `siem-api/internal/store/notifications.go`
- Create: `siem-api/internal/store/notifications_test.go`
- Modify: `siem-api/internal/store/store_test.go` (or create it if it doesn't exist — check first; if a `Migrate` test already lives in another file, add there instead)

**Interfaces:**
- Produces: `store.Store.GetMinNotifySeverity(ctx context.Context) (string, error)`, `store.Store.SetMinNotifySeverity(ctx context.Context, severity string) error`. Task 2 (alerts package) and Task 3 (HTTP handlers) both call these.

- [ ] **Step 1: Check for an existing `Migrate` test**

Run: `grep -rn "func TestMigrate" siem-api/internal/store/`

If a test file already covers `Migrate`, add the new test from Step 2 there instead of creating `store_test.go`. If none exists, create `siem-api/internal/store/store_test.go`.

- [ ] **Step 2: Write the failing migration test**

```go
package store

import (
	"path/filepath"
	"testing"
)

func TestMigrate_AddsNotificationSettingsToExistingDatabase(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "siem.db")
	db, err := Open("sqlite://" + dbPath)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	// Simulate a database that predates this change: `sources` already
	// exists (so the one-time schema.sql bootstrap in Migrate no-ops), but
	// notification_settings does not. This is the exact scenario that would
	// have silently broken on an already-deployed database.
	if _, err := db.Exec(`CREATE TABLE sources (id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatalf("create sources: %v", err)
	}

	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	var severity string
	err = db.QueryRow(`SELECT min_severity FROM notification_settings WHERE id = 1`).Scan(&severity)
	if err != nil {
		t.Fatalf("notification_settings row missing after Migrate(): %v", err)
	}
	if severity != "info" {
		t.Errorf("min_severity = %q, want info (the default)", severity)
	}
}

func TestMigrate_IsIdempotent(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "siem.db")
	db, err := Open("sqlite://" + dbPath)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	if err := Migrate(db); err != nil {
		t.Fatalf("first Migrate() error = %v", err)
	}
	if err := Migrate(db); err != nil {
		t.Fatalf("second Migrate() error = %v", err)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM notification_settings`).Scan(&count); err != nil {
		t.Fatalf("count notification_settings: %v", err)
	}
	if count != 1 {
		t.Errorf("notification_settings row count = %d, want 1 (INSERT OR IGNORE must not duplicate)", count)
	}
}
```

- [ ] **Step 3: Run the tests to verify they fail**

Run: `cd siem-api && go test ./internal/store/... -run TestMigrate -v`
Expected: FAIL — `notification_settings` table doesn't exist yet (`no such table` error).

- [ ] **Step 4: Add the always-run migration file**

Create `siem-api/internal/store/migrations.sql`:

```sql
-- Tables/columns added after the initial release. Every statement in this
-- file MUST be individually idempotent (IF NOT EXISTS / OR IGNORE) - unlike
-- schema.sql, which only ever runs once against a genuinely fresh database
-- (see store.Migrate), this file runs on EVERY startup, including against
-- an already-populated production database.

CREATE TABLE IF NOT EXISTS notification_settings (
  id           INTEGER PRIMARY KEY CHECK (id = 1),
  min_severity TEXT NOT NULL DEFAULT 'info'
);
INSERT OR IGNORE INTO notification_settings (id, min_severity) VALUES (1, 'info');
```

- [ ] **Step 5: Wire the new file into `Migrate()`**

In `siem-api/internal/store/store.go`, add a second embed directive near the existing one:

```go
//go:embed schema.sql
var schemaSQL string

//go:embed migrations.sql
var migrationsSQL string
```

Update `Migrate`:

```go
// Migrate applies schema.sql if the schema hasn't been created yet, then
// always applies migrations.sql - a set of individually-idempotent
// statements for anything added after the initial release. schema.sql only
// ever runs once (gated on `sources` not existing); migrations.sql runs on
// every call, including against an already-populated database, since
// schema.sql's one-time gate would otherwise silently skip new tables on
// any existing deployment.
func Migrate(db *sql.DB) error {
	var exists int
	err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='sources'`).Scan(&exists)
	if err != nil {
		return fmt.Errorf("store: check schema: %w", err)
	}
	if exists == 0 {
		if _, err := db.Exec(schemaSQL); err != nil {
			return fmt.Errorf("store: apply schema: %w", err)
		}
	}

	if _, err := db.Exec(migrationsSQL); err != nil {
		return fmt.Errorf("store: apply migrations: %w", err)
	}
	return nil
}
```

- [ ] **Step 6: Run the tests to verify they pass**

Run: `cd siem-api && go test ./internal/store/... -run TestMigrate -v`
Expected: PASS

- [ ] **Step 7: Write the failing test for the store methods**

Create `siem-api/internal/store/notifications_test.go`:

```go
package store

import (
	"context"
	"path/filepath"
	"testing"
)

func newTestStoreForNotifications(t *testing.T) *Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "siem.db")
	db, err := Open("sqlite://" + dbPath)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	return New(db)
}

func TestGetMinNotifySeverity_DefaultsToInfo(t *testing.T) {
	s := newTestStoreForNotifications(t)

	got, err := s.GetMinNotifySeverity(context.Background())
	if err != nil {
		t.Fatalf("GetMinNotifySeverity() error = %v", err)
	}
	if got != "info" {
		t.Errorf("GetMinNotifySeverity() = %q, want info", got)
	}
}

func TestSetMinNotifySeverity_RoundTrips(t *testing.T) {
	s := newTestStoreForNotifications(t)
	ctx := context.Background()

	if err := s.SetMinNotifySeverity(ctx, "critical"); err != nil {
		t.Fatalf("SetMinNotifySeverity() error = %v", err)
	}

	got, err := s.GetMinNotifySeverity(ctx)
	if err != nil {
		t.Fatalf("GetMinNotifySeverity() error = %v", err)
	}
	if got != "critical" {
		t.Errorf("GetMinNotifySeverity() = %q, want critical", got)
	}
}
```

- [ ] **Step 8: Run the test to verify it fails**

Run: `cd siem-api && go test ./internal/store/... -run "TestGetMinNotifySeverity|TestSetMinNotifySeverity" -v`
Expected: FAIL to compile — the methods don't exist yet.

- [ ] **Step 9: Add the store methods**

Create `siem-api/internal/store/notifications.go`:

```go
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
```

- [ ] **Step 10: Run the tests to verify they pass**

Run: `cd siem-api && go test ./internal/store/... -v`
Expected: All PASS, including the pre-existing store tests (confirm nothing regressed).

- [ ] **Step 11: Commit**

```bash
git add siem-api/internal/store/store.go siem-api/internal/store/migrations.sql \
  siem-api/internal/store/notifications.go siem-api/internal/store/notifications_test.go
git add siem-api/internal/store/store_test.go 2>/dev/null || true
git commit -m "Add always-run migration step and notification_settings table"
```

---

### Task 2: `alerts` package — severity threshold + fix dead priority-mapping cases

**Files:**
- Modify: `siem-api/internal/alerts/service.go`
- Modify: `siem-api/internal/alerts/service_test.go`

**Interfaces:**
- Consumes: `store.Store.GetMinNotifySeverity` (Task 1) — added to the `AlertStore` interface.
- Produces: `severityRank(severity string) int` (unexported, same file as `severityToPriority`).

- [ ] **Step 1: Write the failing tests**

In `siem-api/internal/alerts/service_test.go`:

Add `"errors"` to the import block.

Add a `minSeverity string` and `minSeverityErr error` field to `fakeAlertStore`'s struct definition, and this method:

```go
func (f *fakeAlertStore) GetMinNotifySeverity(ctx context.Context) (string, error) {
	if f.minSeverityErr != nil {
		return "", f.minSeverityErr
	}
	return f.minSeverity, nil
}
```

Replace the existing `TestSeverityToPriority` test's `cases` table (the real alert-severity vocabulary is `critical`/`warning`/`info` — `high`/`medium`/`low` never occur in practice, since `RuleFromEventForm.svelte`'s severity `<select>` only offers `critical`/`warning`/`info`):

```go
func TestSeverityToPriority(t *testing.T) {
	cases := []struct {
		severity string
		want     string
	}{
		{"critical", "urgent"},
		{"warning", "default"},
		{"info", "low"},
		{"unrecognized", "low"},
	}
	for _, c := range cases {
		if got := severityToPriority(c.severity); got != c.want {
			t.Errorf("severityToPriority(%q) = %q, want %q", c.severity, got, c.want)
		}
	}
}

func TestSeverityRank(t *testing.T) {
	cases := []struct {
		severity string
		want     int
	}{
		{"critical", 2},
		{"warning", 1},
		{"info", 0},
		{"unrecognized", 0},
	}
	for _, c := range cases {
		if got := severityRank(c.severity); got != c.want {
			t.Errorf("severityRank(%q) = %d, want %d", c.severity, got, c.want)
		}
	}
}
```

Add two new tests after `TestRaise_NewAlert_InsertsAndNotifies`:

```go
func TestRaise_BelowMinSeverity_NoNotify(t *testing.T) {
	fs := newFakeAlertStore()
	fs.rules[1] = store.Rule{ID: 1, CooldownSec: 3600}
	fs.minSeverity = "critical"
	hub := sse.NewHub()
	ch, cancel := hub.Subscribe("alerts")
	defer cancel()
	notifier := &fakeNotifier{}

	svc := NewService(fs, hub, notifier, testLogger())
	err := svc.Raise(context.Background(), Candidate{
		RuleID: 1, GroupKey: "10.0.0.5", Severity: "warning", Title: "t", Body: "b",
	})
	if err != nil {
		t.Fatalf("Raise() error = %v", err)
	}

	if notifier.calls != 0 {
		t.Errorf("notifier.calls = %d, want 0 for warning below a critical threshold", notifier.calls)
	}
	if len(fs.inserted) != 1 {
		t.Errorf("inserted = %d, want 1 - the alert itself must still be recorded regardless of the notify filter", len(fs.inserted))
	}
	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatal("expected an SSE publish even though the severity is below the notify threshold - in-app visibility is unaffected by this filter")
	}
}

func TestRaise_MinSeverityReadFails_NotifiesAnyway(t *testing.T) {
	fs := newFakeAlertStore()
	fs.rules[1] = store.Rule{ID: 1, CooldownSec: 3600}
	fs.minSeverityErr = errors.New("db unavailable")
	hub := sse.NewHub()
	notifier := &fakeNotifier{}

	svc := NewService(fs, hub, notifier, testLogger())
	err := svc.Raise(context.Background(), Candidate{
		RuleID: 1, GroupKey: "10.0.0.5", Severity: "info", Title: "t", Body: "b",
	})
	if err != nil {
		t.Fatalf("Raise() error = %v", err)
	}
	if notifier.calls != 1 {
		t.Errorf("notifier.calls = %d, want 1 - a failed threshold read must fail open, not silently drop a real notification", notifier.calls)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd siem-api && go test ./internal/alerts/... -v`
Expected: FAIL to compile (`fakeAlertStore` doesn't satisfy `AlertStore` yet if the interface already requires the new method — but since you haven't changed the interface yet, this will instead fail at the new tests' assertions, e.g. `TestRaise_BelowMinSeverity_NoNotify` sees `notifier.calls == 1` instead of the expected 0). Confirm it fails before proceeding either way.

- [ ] **Step 3: Add `severityRank`, fix `severityToPriority`, extend `AlertStore`, apply the threshold in `Raise`**

In `siem-api/internal/alerts/service.go`, add `GetMinNotifySeverity` to the `AlertStore` interface:

```go
type AlertStore interface {
	GetRule(ctx context.Context, id int64) (store.Rule, error)
	FindLatestAlert(ctx context.Context, ruleID int64, groupKey string) (*store.Alert, error)
	InsertAlert(ctx context.Context, a store.Alert) (store.Alert, error)
	TouchAlert(ctx context.Context, id int64, at time.Time) error
	ReopenAlert(ctx context.Context, id int64, at time.Time) error
	AddAlertSample(ctx context.Context, alertID int64, ts time.Time, line string) error
	GetMinNotifySeverity(ctx context.Context) (string, error)
}
```

Replace the notify block at the end of `Raise`:

```go
	if s.notifier != nil {
		minSeverity, err := s.store.GetMinNotifySeverity(ctx)
		if err != nil {
			// Fail open: a broken threshold read must never silently drop a
			// real notification. severityRank("") is 0 (the lowest tier),
			// so this always passes the threshold below.
			s.logger.Error("get min notify severity failed, notifying anyway", "error", err, "alert_id", alertID)
			minSeverity = ""
		}
		if severityRank(c.Severity) >= severityRank(minSeverity) {
			if err := s.notifier.Publish(ctx, c.Title, c.Body, severityToPriority(c.Severity)); err != nil {
				s.logger.Error("ntfy publish failed", "error", err, "alert_id", alertID)
			}
		}
	}
```

Replace `severityToPriority` and add `severityRank` (the real alert-severity vocabulary is `critical`/`warning`/`info` — `high`/`medium` never occur, see Global Constraints):

```go
func severityToPriority(severity string) string {
	switch severity {
	case "critical":
		return "urgent"
	case "warning":
		return "default"
	default: // "info", or anything unrecognized
		return "low"
	}
}

func severityRank(severity string) int {
	switch severity {
	case "critical":
		return 2
	case "warning":
		return 1
	default: // "info", or anything unrecognized
		return 0
	}
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd siem-api && go test ./internal/alerts/... -v`
Expected: All PASS, including every pre-existing test in this file (confirm nothing regressed — `fakeAlertStore`'s zero-value `minSeverity` of `""` ranks 0, so every existing test's `"critical"` candidates still pass the default "notify everything" threshold unchanged).

- [ ] **Step 5: Run the full siem-api test suite**

Run: `cd siem-api && go build ./... && go vet ./... && go test ./...`
Expected: All PASS.

- [ ] **Step 6: Commit**

```bash
git add siem-api/internal/alerts/service.go siem-api/internal/alerts/service_test.go
git commit -m "Gate ntfy push on a minimum-severity threshold; fix dead severityToPriority cases"
```

---

### Task 3: `siem-api` HTTP layer — Settings → Notifications endpoints

**Files:**
- Modify: `siem-api/internal/api/server.go`
- Create: `siem-api/internal/api/settings_notifications.go`
- Create: `siem-api/internal/api/settings_notifications_test.go`
- Modify: `siem-api/cmd/siem-api/main.go`

**Interfaces:**
- Consumes: `store.Store.GetMinNotifySeverity`/`SetMinNotifySeverity` (Task 1).
- Produces: `GET`/`PUT /settings/notifications`, `POST /settings/notifications/test` — response/request shapes below. Task 4 (siem-web) consumes these exact JSON shapes.

- [ ] **Step 1: Write the failing tests**

Create `siem-api/internal/api/settings_notifications_test.go`:

```go
package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hibikipr/homeSIEM/siem-api/internal/ntfy"
)

func TestGetNotificationSettings_RequiresAdmin(t *testing.T) {
	s, st := newTestServer(t)
	token := authToken(t, st, "analyst", 50)

	req := httptest.NewRequest(http.MethodGet, "/settings/notifications", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestGetNotificationSettings_NotConfiguredByDefault(t *testing.T) {
	s, st := newTestServer(t)
	token := authToken(t, st, "admin", 5)

	req := httptest.NewRequest(http.MethodGet, "/settings/notifications", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	var resp notificationSettingsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if resp.NtfyConfigured {
		t.Error("NtfyConfigured = true, want false when NtfyURL/NtfyTopic are unset")
	}
	if resp.MinSeverity != "info" {
		t.Errorf("MinSeverity = %q, want info (the default)", resp.MinSeverity)
	}
}

func TestGetNotificationSettings_ConfiguredWhenUrlAndTopicSet(t *testing.T) {
	s, st := newTestServer(t)
	s.deps.NtfyURL = "https://ntfy.townsville.cc"
	s.deps.NtfyTopic = "homesiem"
	token := authToken(t, st, "admin", 5)

	req := httptest.NewRequest(http.MethodGet, "/settings/notifications", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	var resp notificationSettingsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if !resp.NtfyConfigured {
		t.Error("NtfyConfigured = false, want true when NtfyURL and NtfyTopic are both set")
	}
}

func TestUpdateNotificationSettings_PersistsValidSeverity(t *testing.T) {
	s, st := newTestServer(t)
	token := authToken(t, st, "admin", 5)

	body := `{"min_severity":"critical"}`
	req := httptest.NewRequest(http.MethodPut, "/settings/notifications", bytes.NewReader([]byte(body)))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204, body=%s", rec.Code, rec.Body.String())
	}

	got, err := st.GetMinNotifySeverity(req.Context())
	if err != nil {
		t.Fatalf("GetMinNotifySeverity() error = %v", err)
	}
	if got != "critical" {
		t.Errorf("min_severity = %q, want critical", got)
	}
}

func TestUpdateNotificationSettings_RejectsInvalidSeverity(t *testing.T) {
	s, st := newTestServer(t)
	token := authToken(t, st, "admin", 5)

	body := `{"min_severity":"apocalyptic"}`
	req := httptest.NewRequest(http.MethodPut, "/settings/notifications", bytes.NewReader([]byte(body)))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body=%s", rec.Code, rec.Body.String())
	}
}

func TestTestNotification_NotConfigured(t *testing.T) {
	s, st := newTestServer(t)
	token := authToken(t, st, "admin", 5)

	req := httptest.NewRequest(http.MethodPost, "/settings/notifications/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body=%s", rec.Code, rec.Body.String())
	}
}

func TestTestNotification_Success(t *testing.T) {
	var published bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		published = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	s, st := newTestServer(t)
	s.deps.NtfyURL = srv.URL
	s.deps.NtfyTopic = "homesiem"
	s.deps.Ntfy = ntfy.New(srv.URL, "homesiem", "", srv.Client())
	token := authToken(t, st, "admin", 5)

	req := httptest.NewRequest(http.MethodPost, "/settings/notifications/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	if !published {
		t.Error("expected the fake ntfy server to receive a publish request")
	}
}

func TestTestNotification_PublishFailureReturns502(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	s, st := newTestServer(t)
	s.deps.NtfyURL = srv.URL
	s.deps.NtfyTopic = "homesiem"
	s.deps.Ntfy = ntfy.New(srv.URL, "homesiem", "", srv.Client())
	token := authToken(t, st, "admin", 5)

	req := httptest.NewRequest(http.MethodPost, "/settings/notifications/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502, body=%s", rec.Code, rec.Body.String())
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd siem-api && go test ./internal/api/... -run "NotificationSettings|TestNotification" -v`
Expected: FAIL to compile — `s.deps.NtfyURL`, `s.deps.NtfyTopic`, `s.deps.Ntfy`, `notificationSettingsResponse`, and the routes don't exist yet.

- [ ] **Step 3: Add `Ntfy`/`NtfyURL`/`NtfyTopic` to `Deps` and register the routes**

In `siem-api/internal/api/server.go`, add the import:

```go
	"github.com/hibikipr/homeSIEM/siem-api/internal/ntfy"
```

Add three fields to `Deps` (anywhere in the struct; grouping near `OIDCIssuer`/`OIDCClientID`/`OIDCGroupsScope` keeps deployment-config fields together):

```go
	NtfyURL         string
	NtfyTopic       string
	Ntfy            *ntfy.Client
```

Register the three routes, directly after the existing `/settings/auth` lines:

```go
	s.mux.Handle("GET /settings/notifications", protect(s.deps.Verifier, s.deps.Store, auth.RoleAdmin, http.HandlerFunc(s.handleGetNotificationSettings)))
	s.mux.Handle("PUT /settings/notifications", protect(s.deps.Verifier, s.deps.Store, auth.RoleAdmin, http.HandlerFunc(s.handleUpdateNotificationSettings)))
	s.mux.Handle("POST /settings/notifications/test", protect(s.deps.Verifier, s.deps.Store, auth.RoleAdmin, http.HandlerFunc(s.handleTestNotification)))
```

- [ ] **Step 4: Add the handlers**

Create `siem-api/internal/api/settings_notifications.go`:

```go
package api

import (
	"encoding/json"
	"net/http"
	"time"
)

type notificationSettingsResponse struct {
	NtfyConfigured bool   `json:"ntfy_configured"`
	MinSeverity    string `json:"min_severity"`
}

func (s *Server) handleGetNotificationSettings(w http.ResponseWriter, r *http.Request) {
	minSeverity, err := s.deps.Store.GetMinNotifySeverity(r.Context())
	if err != nil {
		s.deps.Logger.Error("get min notify severity failed", "error", err)
		http.Error(w, "get notification settings failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(notificationSettingsResponse{
		NtfyConfigured: s.deps.NtfyURL != "" && s.deps.NtfyTopic != "",
		MinSeverity:    minSeverity,
	})
}

var validMinSeverities = map[string]bool{"info": true, "warning": true, "critical": true}

type updateNotificationSettingsRequest struct {
	MinSeverity string `json:"min_severity"`
}

func (s *Server) handleUpdateNotificationSettings(w http.ResponseWriter, r *http.Request) {
	var req updateNotificationSettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}
	if !validMinSeverities[req.MinSeverity] {
		http.Error(w, "min_severity must be one of info, warning, critical", http.StatusBadRequest)
		return
	}

	if err := s.deps.Store.SetMinNotifySeverity(r.Context(), req.MinSeverity); err != nil {
		s.deps.Logger.Error("set min notify severity failed", "error", err)
		http.Error(w, "update notification settings failed", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleTestNotification(w http.ResponseWriter, r *http.Request) {
	if s.deps.NtfyURL == "" || s.deps.NtfyTopic == "" || s.deps.Ntfy == nil {
		http.Error(w, "ntfy is not configured", http.StatusBadRequest)
		return
	}

	title := "homeSIEM test notification"
	body := "Sent from Settings at " + time.Now().UTC().Format(time.RFC3339) + " to confirm ntfy is reachable."

	if err := s.deps.Ntfy.Publish(r.Context(), title, body, "default"); err != nil {
		s.deps.Logger.Error("test notification publish failed", "error", err)
		http.Error(w, "test notification failed", http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}
```

- [ ] **Step 5: Wire `Deps` in `main.go`**

In `siem-api/cmd/siem-api/main.go`, extend the `api.Deps{...}` construction (the `ntfyClient` variable already exists — this just also passes it, plus the raw URL/topic, into `Deps`):

```go
	server := api.NewServer(api.Deps{
		Store: st, Loki: lokiClient, Vector: vectorClient, JobLabel: cfg.LokiJobLabel, Hub: hub,
		Alerts: alertsSvc, Scheduler: scheduler, SchedulerCtx: appCtx,
		Verifier: verifier, SessionEst: sessionEst, LocalAuth: localAuth,
		FastpathToken: cfg.FastpathToken,
		OIDCIssuer:    cfg.OIDCIssuer, OIDCClientID: cfg.OIDCClientID, OIDCGroupsScope: cfg.OIDCGroupsScope,
		NtfyURL: cfg.NtfyURL, NtfyTopic: cfg.NtfyTopic, Ntfy: ntfyClient,
		Logger: logger,
	})
```

- [ ] **Step 6: Run the tests to verify they pass**

Run: `cd siem-api && go test ./internal/api/... -v`
Expected: All PASS, including every pre-existing test in this package.

- [ ] **Step 7: Run the full siem-api build, vet, and test suite**

Run: `cd siem-api && go build ./... && go vet ./... && go test ./...`
Expected: All PASS.

- [ ] **Step 8: Commit**

```bash
git add siem-api/internal/api/server.go siem-api/internal/api/settings_notifications.go \
  siem-api/internal/api/settings_notifications_test.go siem-api/cmd/siem-api/main.go
git commit -m "Add GET/PUT /settings/notifications and POST /settings/notifications/test"
```

---

### Task 4: `siem-web` — client methods, proxy routes, and the real Settings UI

**Files:**
- Modify: `siem-web/src/lib/server/siemApiClient.ts`
- Modify: `siem-web/src/lib/server/siemApiClient.test.ts`
- Create: `siem-web/src/routes/api/settings/notifications/+server.ts`
- Create: `siem-web/src/routes/api/settings/notifications/test/+server.ts`
- Modify: `siem-web/src/routes/settings/+page.server.ts`
- Modify: `siem-web/src/routes/settings/page.server.test.ts`
- Modify: `siem-web/src/routes/settings/+page.svelte`

**Interfaces:**
- Consumes: `GET`/`PUT /settings/notifications`, `POST /settings/notifications/test` (Task 3) — exact JSON shapes: `{"ntfy_configured": boolean, "min_severity": string}` for GET; PUT body `{"min_severity": string}`; test endpoint returns `{"ok": true}` on success.

- [ ] **Step 1: Write the failing `siemApiClient` tests**

In `siem-web/src/lib/server/siemApiClient.test.ts`, add before the closing `});` of the `describe('SiemApiClient', ...)` block:

```ts
	it('getNotificationSettings attaches Authorization and parses the response', async () => {
		const fetchFn = fakeFetch({ ntfy_configured: true, min_severity: 'warning' });
		const client = new SiemApiClient({ baseUrl: 'http://siem-api:8080' }, fetchFn);

		const result = await client.getNotificationSettings('token-123');

		expect(result).toEqual({ ntfy_configured: true, min_severity: 'warning' });
		const [url, init] = fetchFn.mock.calls[0];
		expect(url).toBe('http://siem-api:8080/settings/notifications');
		expect((init?.headers as Record<string, string>).Authorization).toBe('Bearer token-123');
	});

	it('updateNotificationSettings PUTs to /settings/notifications with Authorization and a JSON body', async () => {
		const fetchFn = fakeFetch(null, 204);
		const client = new SiemApiClient({ baseUrl: 'http://siem-api:8080' }, fetchFn);

		await client.updateNotificationSettings('token-123', 'critical');

		const [url, init] = fetchFn.mock.calls[0];
		expect(url).toBe('http://siem-api:8080/settings/notifications');
		expect(init?.method).toBe('PUT');
		expect((init?.headers as Record<string, string>).Authorization).toBe('Bearer token-123');
		expect(JSON.parse(init?.body as string)).toEqual({ min_severity: 'critical' });
	});

	it('testNotification POSTs to /settings/notifications/test with Authorization', async () => {
		const fetchFn = fakeFetch({ ok: true });
		const client = new SiemApiClient({ baseUrl: 'http://siem-api:8080' }, fetchFn);

		await client.testNotification('token-123');

		const [url, init] = fetchFn.mock.calls[0];
		expect(url).toBe('http://siem-api:8080/settings/notifications/test');
		expect(init?.method).toBe('POST');
		expect((init?.headers as Record<string, string>).Authorization).toBe('Bearer token-123');
	});
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd siem-web && pnpm exec vitest run src/lib/server/siemApiClient.test.ts`
Expected: FAIL — the three methods don't exist yet.

- [ ] **Step 3: Add the interface and client methods**

In `siem-web/src/lib/server/siemApiClient.ts`, add near `AuthSettingsResponse`:

```ts
export interface NotificationSettingsResponse {
	ntfy_configured: boolean;
	min_severity: string;
}
```

Add three methods to the `SiemApiClient` class, near `getAuthSettings`/`updateRoleMappings`:

```ts
	async getNotificationSettings(sessionToken: string): Promise<NotificationSettingsResponse> {
		return this.request<NotificationSettingsResponse>(
			'/settings/notifications',
			this.authInit(sessionToken)
		);
	}

	async updateNotificationSettings(sessionToken: string, minSeverity: string): Promise<void> {
		return this.requestNoContent('/settings/notifications', {
			method: 'PUT',
			headers: { 'Content-Type': 'application/json', ...this.authInit(sessionToken).headers },
			body: JSON.stringify({ min_severity: minSeverity })
		});
	}

	async testNotification(sessionToken: string): Promise<void> {
		await this.request('/settings/notifications/test', {
			method: 'POST',
			...this.authInit(sessionToken)
		});
	}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd siem-web && pnpm exec vitest run src/lib/server/siemApiClient.test.ts`
Expected: PASS

- [ ] **Step 5: Add the proxy routes**

Create `siem-web/src/routes/api/settings/notifications/+server.ts`:

```ts
import { json } from '@sveltejs/kit';
import { env } from '$env/dynamic/private';
import type { RequestHandler } from './$types';
import { SiemApiClient, SiemApiError } from '$lib/server/siemApiClient';

export const GET: RequestHandler = async ({ locals }) => {
	const client = new SiemApiClient({ baseUrl: env.API_URL as string });
	const token = locals.sessionToken as string;

	try {
		const settings = await client.getNotificationSettings(token);
		return json(settings);
	} catch (err) {
		if (err instanceof SiemApiError) {
			return json({ error: err.message }, { status: err.status });
		}
		throw err;
	}
};

export const PUT: RequestHandler = async ({ request, locals }) => {
	const client = new SiemApiClient({ baseUrl: env.API_URL as string });
	const token = locals.sessionToken as string;
	const body = (await request.json()) as { min_severity: string };

	try {
		await client.updateNotificationSettings(token, body.min_severity);
	} catch (err) {
		if (err instanceof SiemApiError) {
			return json({ error: err.message }, { status: err.status });
		}
		throw err;
	}

	return new Response(null, { status: 204 });
};
```

Create `siem-web/src/routes/api/settings/notifications/test/+server.ts`:

```ts
import { json } from '@sveltejs/kit';
import { env } from '$env/dynamic/private';
import type { RequestHandler } from './$types';
import { SiemApiClient, SiemApiError } from '$lib/server/siemApiClient';

export const POST: RequestHandler = async ({ locals }) => {
	const client = new SiemApiClient({ baseUrl: env.API_URL as string });
	const token = locals.sessionToken as string;

	try {
		await client.testNotification(token);
	} catch (err) {
		if (err instanceof SiemApiError) {
			return json({ error: err.message }, { status: err.status });
		}
		throw err;
	}

	return json({ ok: true });
};
```

- [ ] **Step 6: Write the failing `+page.server.ts` load tests**

In `siem-web/src/routes/settings/page.server.test.ts`, add `getNotificationSettings: vi.fn().mockResolvedValue({ ntfy_configured: false, min_severity: 'info' })` to the mock implementation objects in the first two tests (`'returns the real role mappings from siem-api'` and `'returns an empty array when siem-api sends role_mappings: null'` — the two success-path tests; the 401/403/502 tests reject on `getAuthSettings` before `getNotificationSettings` would ever be called, so they don't need this addition). Also add an assertion in the first test:

```ts
			expect(result.notificationSettings).toEqual({ ntfy_configured: false, min_severity: 'info' });
```

- [ ] **Step 7: Run the tests to verify they fail**

Run: `cd siem-web && pnpm exec vitest run src/routes/settings/page.server.test.ts`
Expected: FAIL — `load` doesn't call `getNotificationSettings` yet, so `result.notificationSettings` is `undefined`.

- [ ] **Step 8: Update the load function**

In `siem-web/src/routes/settings/+page.server.ts`:

```ts
import { error, redirect } from '@sveltejs/kit';
import { env } from '$env/dynamic/private';
import type { PageServerLoad } from './$types';
import { SiemApiClient, SiemApiError } from '$lib/server/siemApiClient';

export const load: PageServerLoad = async ({ locals }) => {
	const client = new SiemApiClient({ baseUrl: env.API_URL as string });
	const token = locals.sessionToken as string;

	let settings;
	let notificationSettings;
	try {
		settings = await client.getAuthSettings(token);
		notificationSettings = await client.getNotificationSettings(token);
	} catch (err) {
		if (err instanceof SiemApiError) {
			if (err.status === 401) {
				redirect(302, '/auth/logout');
			}
			if (err.status === 403) {
				error(403, 'Settings is only available to admins.');
			}
			error(502, 'siem-api unavailable');
		}
		throw err;
	}

	return { roleMappings: settings.role_mappings ?? [], notificationSettings };
};
```

- [ ] **Step 9: Run the tests to verify they pass**

Run: `cd siem-web && pnpm exec vitest run src/routes/settings/page.server.test.ts`
Expected: PASS

- [ ] **Step 10: Build the real Notifications section UI**

In `siem-web/src/routes/settings/+page.svelte`, add to the script block, after the existing `formSeed` state:

```ts
	let minSeverity = $state<'info' | 'warning' | 'critical'>(
		(data.notificationSettings.min_severity as 'info' | 'warning' | 'critical') ?? 'info'
	);
	let savingSeverity = $state(false);
	let severitySaveError = $state<string | null>(null);
	let testSending = $state(false);
	let testResult = $state<'success' | 'error' | null>(null);

	async function saveMinSeverity() {
		savingSeverity = true;
		severitySaveError = null;
		try {
			const res = await fetch('/api/settings/notifications', {
				method: 'PUT',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ min_severity: minSeverity })
			});
			if (!res.ok) throw new Error('save failed');
		} catch {
			severitySaveError = 'Could not save — try again.';
		} finally {
			savingSeverity = false;
		}
	}

	async function sendTestNotification() {
		testSending = true;
		testResult = null;
		try {
			const res = await fetch('/api/settings/notifications/test', { method: 'POST' });
			testResult = res.ok ? 'success' : 'error';
		} catch {
			testResult = 'error';
		} finally {
			testSending = false;
		}
	}
```

Replace the `{:else}` generic-placeholder branch's condition — change:

```svelte
		{:else}
			<div class="hero">
				<h1>{sections.find((section) => section.key === selectedSection)?.label}</h1>
				<p>This section is ready for the next set of settings content.</p>
			</div>
		{/if}
```

to:

```svelte
		{:else if selectedSection === 'notifications'}
			<div class="hero">
				<h1>Notifications</h1>
				<p>
					homeSIEM pushes new alerts through ntfy. The server URL and topic are set at deploy
					time; this page controls how loud it is.
				</p>
			</div>

			<div class="panel">
				<div class="panel-head">
					<span class="panel-title">ntfy status</span>
				</div>
				<p class="status-line">
					{#if data.notificationSettings.ntfy_configured}
						<span class="ok">Configured</span> — NTFY_URL and NTFY_TOPIC are set.
					{:else}
						<span class="warn">Not configured</span> — set NTFY_URL and NTFY_TOPIC to enable notifications.
					{/if}
				</p>
				<button
					class="btn ghost"
					type="button"
					disabled={!data.notificationSettings.ntfy_configured || testSending}
					onclick={sendTestNotification}
				>
					{testSending ? 'Sending…' : 'Send test notification'}
				</button>
				{#if testResult === 'success'}
					<p class="status-line ok">Test notification sent.</p>
				{:else if testResult === 'error'}
					<p class="status-line warn">Could not send the test notification.</p>
				{/if}
			</div>

			<div class="panel">
				<div class="panel-head">
					<span class="panel-title">Minimum severity to notify</span>
					<span class="muted">alerts below this severity still appear in-app, just don't push</span>
				</div>
				<select bind:value={minSeverity} onchange={saveMinSeverity}>
					<option value="info">info — notify on everything</option>
					<option value="warning">warning — skip info-level alerts</option>
					<option value="critical">critical — only the most severe</option>
				</select>
				{#if savingSeverity}
					<span class="muted">Saving…</span>
				{:else if severitySaveError}
					<span class="status-line warn">{severitySaveError}</span>
				{/if}
			</div>
		{:else}
			<div class="hero">
				<h1>{sections.find((section) => section.key === selectedSection)?.label}</h1>
				<p>This section is ready for the next set of settings content.</p>
			</div>
		{/if}
```

Add to the `<style>` block:

```css
	.status-line {
		font-size: var(--text-table);
		margin: 0;
	}
	.status-line .ok,
	.status-line.ok {
		color: var(--color-severity-healthy);
	}
	.status-line .warn,
	.status-line.warn {
		color: var(--color-severity-warning);
	}
	select {
		background: var(--color-surface-3);
		color: var(--color-text);
		border: 1px solid var(--color-line-2);
		border-radius: var(--radius-sm);
		padding: 4px 8px;
		font-size: var(--text-table);
	}
```

- [ ] **Step 11: Manually verify in a real browser**

Per Global Constraints, no component test infrastructure — verify by hand, using the same minted-session-cookie technique as the Nav-avatar-picture plan's Task 2 (real OIDC login isn't available in this sandbox):

1. Start the dev server: `cd siem-web && pnpm dev`.
2. Mint a valid session cookie directly via `mintSessionToken` (see `siem-web/src/lib/server/session.ts`) with a `role: 'admin'` claim, matching the dev server's `SESSION_SECRET`, and set it as the `siem_session` cookie via Playwright (already configured in this repo).
3. Navigate to `/settings`, click "Notifications" in the sidebar.
4. Confirm it shows "Not configured" status (no `NTFY_URL` set in this dev environment) and the "Send test notification" button is disabled.
5. Change the minimum-severity dropdown and confirm the PUT request fires (check via Playwright's network inspection or a temporary console log) and no error message appears.
6. Confirm the other sidebar sections (Retention, Parsers, Backups, About) still show the generic placeholder unchanged.

- [ ] **Step 12: Run the full siem-web test suite, lint, and type-check**

Run: `cd siem-web && pnpm exec vitest run && pnpm lint && pnpm exec svelte-check`
Expected: All PASS, no new type errors, lint clean.

- [ ] **Step 13: Commit**

```bash
git add siem-web/src/lib/server/siemApiClient.ts siem-web/src/lib/server/siemApiClient.test.ts \
  siem-web/src/routes/api/settings/notifications/+server.ts \
  siem-web/src/routes/api/settings/notifications/test/+server.ts \
  siem-web/src/routes/settings/+page.server.ts siem-web/src/routes/settings/page.server.test.ts \
  siem-web/src/routes/settings/+page.svelte
git commit -m "Build the real Settings -> Notifications UI"
```
