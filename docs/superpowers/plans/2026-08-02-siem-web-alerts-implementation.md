# siem-web Alerts Screen Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the Alerts screen (inbox + detail, read-only Rules tab), two small siem-api
additions it needs (`GET /alerts/{id}/samples`, `POST /alerts/{id}/mute`), and wire up
Wall's previously-inert `TriageCard` actions.

**Architecture:** Same thin-BFF pattern as the Wall sub-project — SvelteKit server routes
do all siem-api calls and hold the session token; Svelte components are pure presentation.
Two new small siem-api endpoints get added to the still-open siem-api PR #1, mirroring how
`GET /events/stats` was added during the Wall sub-project.

**Tech Stack:** Go (siem-api additions), SvelteKit/TypeScript (siem-web), Vitest, pnpm.

## Global Constraints

- **Two worktrees.** Tasks 1–2 run in
  `/Users/hibikipr/Documents/GitHub/homeSIEM/.claude/worktrees/siem-api-implementation/siem-api`
  (Go, branch `worktree-siem-api-implementation`, the still-open PR #1). Tasks 3–9 run in
  `/Users/hibikipr/Documents/GitHub/homeSIEM/.claude/worktrees/siem-web-console/siem-web`
  (SvelteKit, branch `worktree-siem-web-console`, PR #2). Every task states which one.
- pnpm exclusively for siem-web tasks (never npm/yarn). Standard `go build`/`go vet`/`go test ./...`
  for siem-api tasks.
- No token/credential ever reaches client JS: alert mutations (ack, mute) go through a
  thin siem-web passthrough route, never called directly from the browser to siem-api —
  same boundary as the SSE tail proxy from the Wall sub-project.
- TDD for logic (store/service/handler changes, the siem-api client extension, data-shaping
  helpers, passthrough/proxy routes, `+page.server.ts` loads). No unit tests for
  presentational Svelte components, per the established testing split — verify those via
  `pnpm check`/`pnpm build` instead.
- Rules tab is read-only this pass — no create/edit/delete/enable-toggle UI. "Block at
  gateway" ships as a disabled button with an explanatory tooltip, not wired to any
  backend (SOAR-style automated response is out of scope for v1 per the design handoff).
  "Reputation" renders as a static `"unknown"` placeholder — nothing populates real
  threat-intel data yet.
- No AI attribution in commit messages.
- After Task 2 (the last siem-api task), push the siem-api worktree's new commits to
  `origin` to update the already-open PR #1 — matches how `GET /events/stats` was added
  directly to that PR during the Wall sub-project, rather than waiting for a separate
  siem-api PR cycle.

---

### Task 1: siem-api — mute lifecycle (store layer + `Raise()` change)

**Worktree:** `siem-api-implementation/siem-api`

**Files:**
- Modify: `schema.sql`
- Modify: `internal/store/alerts.go`
- Modify: `internal/alerts/service.go`
- Test: `internal/store/alerts_test.go` (add to existing file)
- Test: `internal/alerts/service_test.go` (add to existing file)

**Interfaces:**
- Produces: `store.Alert.MutedUntil *time.Time` (new field), `Store.MuteAlert(ctx context.Context, id int64, userID int64, until time.Time) error` — consumed by Task 2's HTTP handler.

`store.Alert` already has an `AckedBy`/`AckedAt` pair using `scanNullTime`/`formatTime` —
`MutedUntil` follows the identical pattern. `Raise()`'s existing switch already has a branch
for "existing open alert within cooldown → touch only, no notify"; this task adds a sibling
branch for "existing muted alert with unexpired `MutedUntil` → touch only, no notify",
so muting an alert actually suppresses re-notification until it expires, at which point the
existing fallback branch (`case existing != nil:` → `ReopenAlert` + notify) already handles
the "mute expired, rule fired again" case correctly with no further changes needed.

- [ ] **Step 1: Write the failing store test**

Add to `internal/store/alerts_test.go`:
```go
func TestMuteAlert(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	rule := createTestRule(t, s)
	now := time.Now().UTC()

	inserted, err := s.InsertAlert(ctx, Alert{
		RuleID: rule.ID, GroupKey: "10.0.0.5", Severity: "critical", Title: "t", Body: "b",
		EventCount: 1, Context: "{}", State: "open", FirstSeenAt: now, LastSeenAt: now,
	})
	if err != nil {
		t.Fatalf("InsertAlert() error = %v", err)
	}

	until := now.Add(time.Hour)
	if err := s.MuteAlert(ctx, inserted.ID, 1, until); err != nil {
		t.Fatalf("MuteAlert() error = %v", err)
	}

	got, err := s.GetAlert(ctx, inserted.ID)
	if err != nil {
		t.Fatalf("GetAlert() error = %v", err)
	}
	if got.State != "muted" {
		t.Errorf("State = %q, want muted", got.State)
	}
	if got.MutedUntil == nil || !got.MutedUntil.Equal(until) {
		t.Errorf("MutedUntil = %v, want %v", got.MutedUntil, until)
	}

	entries, err := s.ListAudit(ctx, 10)
	if err != nil {
		t.Fatalf("ListAudit() error = %v", err)
	}
	found := false
	for _, e := range entries {
		if e.Action == "alert.mute" {
			found = true
		}
	}
	if !found {
		t.Error("no alert.mute audit entry found")
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./internal/store/... -run TestMuteAlert -v`
Expected: FAIL — `MuteAlert` and `MutedUntil` don't exist yet.

- [ ] **Step 3: Write the failing service tests**

Add to `internal/alerts/service_test.go`:
```go
func TestRaise_MutedAndUnexpired_TouchesOnlyNoNotify(t *testing.T) {
	fs := newFakeAlertStore()
	fs.rules[1] = store.Rule{ID: 1, CooldownSec: 60}
	now := time.Now().UTC()
	until := now.Add(30 * time.Minute)
	fs.openAlerts[key(1, "10.0.0.5")] = &store.Alert{
		ID: 99, RuleID: 1, GroupKey: "10.0.0.5", State: "muted",
		LastSeenAt: now.Add(-time.Hour), MutedUntil: &until,
	}
	hub := sse.NewHub()
	ch, cancel := hub.Subscribe("alerts")
	defer cancel()
	notifier := &fakeNotifier{}

	svc := NewService(fs, hub, notifier, testLogger())
	err := svc.Raise(context.Background(), Candidate{RuleID: 1, GroupKey: "10.0.0.5", Severity: "critical", Title: "t", Body: "b"})
	if err != nil {
		t.Fatalf("Raise() error = %v", err)
	}

	if len(fs.touched) != 1 || fs.touched[0] != 99 {
		t.Errorf("touched = %v, want [99]", fs.touched)
	}
	if len(fs.reopened) != 0 {
		t.Error("expected no reopen while muted and unexpired")
	}
	if notifier.calls != 0 {
		t.Errorf("notifier.calls = %d, want 0 while muted", notifier.calls)
	}

	select {
	case msg := <-ch:
		t.Fatalf("expected no SSE publish while muted, got %q", msg)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestRaise_MutedAndExpired_ReopensAndNotifies(t *testing.T) {
	fs := newFakeAlertStore()
	fs.rules[1] = store.Rule{ID: 1, CooldownSec: 60}
	now := time.Now().UTC()
	expired := now.Add(-time.Minute)
	fs.openAlerts[key(1, "10.0.0.5")] = &store.Alert{
		ID: 99, RuleID: 1, GroupKey: "10.0.0.5", State: "muted",
		LastSeenAt: now.Add(-2 * time.Hour), MutedUntil: &expired,
	}
	hub := sse.NewHub()
	notifier := &fakeNotifier{}

	svc := NewService(fs, hub, notifier, testLogger())
	err := svc.Raise(context.Background(), Candidate{RuleID: 1, GroupKey: "10.0.0.5", Severity: "critical", Title: "t", Body: "b"})
	if err != nil {
		t.Fatalf("Raise() error = %v", err)
	}

	if len(fs.reopened) != 1 || fs.reopened[0] != 99 {
		t.Errorf("reopened = %v, want [99]", fs.reopened)
	}
	if notifier.calls != 1 {
		t.Errorf("notifier.calls = %d, want 1 after mute expiry", notifier.calls)
	}
}
```

- [ ] **Step 4: Run them to verify they fail**

Run: `go test ./internal/alerts/... -run TestRaise_Muted -v`
Expected: FAIL — `store.Alert` has no `MutedUntil` field yet, so this won't compile.

- [ ] **Step 5: Add the `muted_until` column**

In `schema.sql`, change the `alerts` table definition from:
```sql
CREATE TABLE alerts (
  id            INTEGER PRIMARY KEY,
  rule_id       INTEGER NOT NULL REFERENCES rules(id) ON DELETE CASCADE,
  group_key     TEXT NOT NULL,              -- dedupe key, e.g. src_ip value
  severity      TEXT NOT NULL,
  title         TEXT NOT NULL,
  body          TEXT NOT NULL,
  event_count   INTEGER NOT NULL DEFAULT 1,
  context       TEXT,                       -- JSON: ports, geoip, intel
  state         TEXT NOT NULL DEFAULT 'open', -- open|acked|muted|closed
  first_seen_at TEXT NOT NULL DEFAULT (datetime('now')),
  last_seen_at  TEXT NOT NULL DEFAULT (datetime('now')),
  acked_by      INTEGER REFERENCES users(id),
  acked_at      TEXT,
  UNIQUE (rule_id, group_key, state)
);
```
to:
```sql
CREATE TABLE alerts (
  id            INTEGER PRIMARY KEY,
  rule_id       INTEGER NOT NULL REFERENCES rules(id) ON DELETE CASCADE,
  group_key     TEXT NOT NULL,              -- dedupe key, e.g. src_ip value
  severity      TEXT NOT NULL,
  title         TEXT NOT NULL,
  body          TEXT NOT NULL,
  event_count   INTEGER NOT NULL DEFAULT 1,
  context       TEXT,                       -- JSON: ports, geoip, intel
  state         TEXT NOT NULL DEFAULT 'open', -- open|acked|muted|closed
  first_seen_at TEXT NOT NULL DEFAULT (datetime('now')),
  last_seen_at  TEXT NOT NULL DEFAULT (datetime('now')),
  acked_by      INTEGER REFERENCES users(id),
  acked_at      TEXT,
  muted_until   TEXT,                       -- set when state = 'muted'
  UNIQUE (rule_id, group_key, state)
);
```

- [ ] **Step 6: Update the store layer**

In `internal/store/alerts.go`, add `MutedUntil` to the `Alert` struct:
```go
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
```

Update `alertSelect` and `scanAlert`:
```go
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
```

Update `ReopenAlert` to also clear `muted_until` (so a reopened alert never carries a stale
value forward):
```go
func (s *Store) ReopenAlert(ctx context.Context, id int64, at time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE alerts SET state = 'open', last_seen_at = ?, event_count = 1, acked_by = NULL, acked_at = NULL, muted_until = NULL WHERE id = ?`,
		formatTime(at), id)
	return err
}
```

Add `MuteAlert`, right after `AckAlert`:
```go
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
```

- [ ] **Step 7: Update `Raise()`'s lifecycle switch**

In `internal/alerts/service.go`, add a new case between the existing "open within cooldown"
case and the general "existing != nil" fallback:
```go
	switch {
	case existing != nil && existing.State == "open" && now.Sub(existing.LastSeenAt) < time.Duration(rule.CooldownSec)*time.Second:
		if err := s.store.TouchAlert(ctx, existing.ID, now); err != nil {
			return err
		}
		alertID = existing.ID

	case existing != nil && existing.State == "muted" && existing.MutedUntil != nil && now.Before(*existing.MutedUntil):
		if err := s.store.TouchAlert(ctx, existing.ID, now); err != nil {
			return err
		}
		alertID = existing.ID

	case existing != nil:
		// Either still open but cooldown lapsed, previously acked/closed and
		// firing again, or muted with an expired MutedUntil — either way,
		// reuse the same row (never insert a second row for this
		// rule_id+group_key, which would violate the schema's
		// UNIQUE(rule_id, group_key, state) once it's later acked).
		if err := s.store.ReopenAlert(ctx, existing.ID, now); err != nil {
			return err
		}
		alertID = existing.ID
		notify = true

	default:
```
(the `default:` branch and everything below it is unchanged)

- [ ] **Step 8: Run both test files to verify they pass**

Run: `go test ./internal/store/... ./internal/alerts/... -v`
Expected: PASS, including the two new `TestRaise_Muted*` tests and `TestMuteAlert`.

- [ ] **Step 9: Commit**

```bash
git add schema.sql internal/store/alerts.go internal/store/alerts_test.go internal/alerts/service.go internal/alerts/service_test.go
git commit -m "Add alert mute lifecycle: muted_until column, MuteAlert, Raise() suppression"
```

---

### Task 2: siem-api — `POST /alerts/{id}/mute` and `GET /alerts/{id}/samples`

**Worktree:** `siem-api-implementation/siem-api`

**Files:**
- Modify: `internal/api/alerts.go`
- Modify: `internal/api/server.go`
- Test: `internal/api/alerts_test.go` (add to existing file)

**Interfaces:**
- Consumes: `store.MuteAlert` (Task 1), existing `store.ListAlertSamples`/`store.GetAlert`.
- Produces: `POST /alerts/{id}/mute` (analyst+, 204), `GET /alerts/{id}/samples` (viewer+,
  `[{id, ts, line}]`) — consumed by Task 3's siem-web client extension.

- [ ] **Step 1: Write the failing tests**

Add to `internal/api/alerts_test.go`:
```go
func TestMuteAlert_ViewerForbidden(t *testing.T) {
	s, st := newTestServer(t)
	ctx := context.Background()
	rule, err := st.CreateRule(ctx, store.Rule{Name: "r", Shape: "absence", Severity: "low",
		Destinations: []string{"inapp"}, CooldownSec: 60, IntervalSec: 60, Enabled: true}, nil)
	if err != nil {
		t.Fatalf("CreateRule() error = %v", err)
	}
	now := time.Now().UTC()
	alert, err := st.InsertAlert(ctx, store.Alert{RuleID: rule.ID, GroupKey: "a", Severity: "low",
		Title: "t", Body: "b", EventCount: 1, Context: "{}", State: "open", FirstSeenAt: now, LastSeenAt: now})
	if err != nil {
		t.Fatalf("InsertAlert() error = %v", err)
	}

	token := authToken(t, st, "viewer", 100)
	req := httptest.NewRequest(http.MethodPost, "/alerts/"+itoa(alert.ID)+"/mute", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestMuteAlert_AnalystSucceedsAndAudits(t *testing.T) {
	s, st := newTestServer(t)
	ctx := context.Background()
	rule, err := st.CreateRule(ctx, store.Rule{Name: "r", Shape: "absence", Severity: "low",
		Destinations: []string{"inapp"}, CooldownSec: 60, IntervalSec: 60, Enabled: true}, nil)
	if err != nil {
		t.Fatalf("CreateRule() error = %v", err)
	}
	now := time.Now().UTC()
	alert, err := st.InsertAlert(ctx, store.Alert{RuleID: rule.ID, GroupKey: "a", Severity: "low",
		Title: "t", Body: "b", EventCount: 1, Context: "{}", State: "open", FirstSeenAt: now, LastSeenAt: now})
	if err != nil {
		t.Fatalf("InsertAlert() error = %v", err)
	}

	token := authToken(t, st, "analyst", 50)
	req := httptest.NewRequest(http.MethodPost, "/alerts/"+itoa(alert.ID)+"/mute", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204, body=%s", rec.Code, rec.Body.String())
	}

	got, err := st.GetAlert(ctx, alert.ID)
	if err != nil {
		t.Fatalf("GetAlert() error = %v", err)
	}
	if got.State != "muted" {
		t.Errorf("State = %q, want muted", got.State)
	}
	if got.MutedUntil == nil || !got.MutedUntil.After(now) {
		t.Errorf("MutedUntil = %v, want a time after %v", got.MutedUntil, now)
	}
}

func TestMuteAlert_NotFound(t *testing.T) {
	s, st := newTestServer(t)
	token := authToken(t, st, "analyst", 50)
	req := httptest.NewRequest(http.MethodPost, "/alerts/9999/mute", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body=%s", rec.Code, rec.Body.String())
	}
}

func TestListAlertSamples_ReturnsStoredSamples(t *testing.T) {
	s, st := newTestServer(t)
	ctx := context.Background()
	rule, err := st.CreateRule(ctx, store.Rule{Name: "r", Shape: "absence", Severity: "low",
		Destinations: []string{"inapp"}, CooldownSec: 60, IntervalSec: 60, Enabled: true}, nil)
	if err != nil {
		t.Fatalf("CreateRule() error = %v", err)
	}
	now := time.Now().UTC()
	alert, err := st.InsertAlert(ctx, store.Alert{RuleID: rule.ID, GroupKey: "a", Severity: "low",
		Title: "t", Body: "b", EventCount: 1, Context: "{}", State: "open", FirstSeenAt: now, LastSeenAt: now})
	if err != nil {
		t.Fatalf("InsertAlert() error = %v", err)
	}
	if err := st.AddAlertSample(ctx, alert.ID, now, `{"src_ip":"10.0.0.5","dst_port":443}`); err != nil {
		t.Fatalf("AddAlertSample() error = %v", err)
	}

	token := authToken(t, st, "viewer", 100)
	req := httptest.NewRequest(http.MethodGet, "/alerts/"+itoa(alert.ID)+"/samples", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}

	var resp []alertSampleResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if len(resp) != 1 {
		t.Fatalf("len(resp) = %d, want 1", len(resp))
	}
	if resp[0].Line != `{"src_ip":"10.0.0.5","dst_port":443}` {
		t.Errorf("Line = %q, want the inserted sample", resp[0].Line)
	}
}

func TestListAlertSamples_NotFound(t *testing.T) {
	s, st := newTestServer(t)
	token := authToken(t, st, "viewer", 100)
	req := httptest.NewRequest(http.MethodGet, "/alerts/9999/samples", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body=%s", rec.Code, rec.Body.String())
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./internal/api/... -run 'TestMuteAlert|TestListAlertSamples' -v`
Expected: FAIL — the handlers and routes don't exist yet.

- [ ] **Step 3: Add the handlers**

In `internal/api/alerts.go`, add after `handleAckAlert`:
```go
func (s *Server) handleMuteAlert(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid alert id", http.StatusBadRequest)
		return
	}
	userID, _, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthenticated", http.StatusUnauthorized)
		return
	}

	until := time.Now().UTC().Add(time.Hour)
	if err := s.deps.Store.MuteAlert(r.Context(), id, userID, until); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "alert not found", http.StatusNotFound)
			return
		}
		s.deps.Logger.Error("mute alert failed", "alert_id", id, "error", err)
		http.Error(w, "mute failed", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type alertSampleResponse struct {
	ID   int64     `json:"id"`
	TS   time.Time `json:"ts"`
	Line string    `json:"line"`
}

func (s *Server) handleListAlertSamples(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid alert id", http.StatusBadRequest)
		return
	}

	if _, err := s.deps.Store.GetAlert(r.Context(), id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "alert not found", http.StatusNotFound)
			return
		}
		s.deps.Logger.Error("get alert failed", "alert_id", id, "error", err)
		http.Error(w, "get alert failed", http.StatusInternalServerError)
		return
	}

	samples, err := s.deps.Store.ListAlertSamples(r.Context(), id)
	if err != nil {
		s.deps.Logger.Error("list alert samples failed", "alert_id", id, "error", err)
		http.Error(w, "list samples failed", http.StatusInternalServerError)
		return
	}

	resp := make([]alertSampleResponse, len(samples))
	for i, sample := range samples {
		resp[i] = alertSampleResponse{ID: sample.ID, TS: sample.TS, Line: sample.Line}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
```

- [ ] **Step 4: Register the routes**

In `internal/api/server.go`'s `routes()`, add right after the existing
`POST /alerts/{id}/ack` line:
```go
	s.mux.Handle("POST /alerts/{id}/mute", protect(s.deps.Verifier, s.deps.Store, auth.RoleAnalyst, http.HandlerFunc(s.handleMuteAlert)))
	s.mux.Handle("GET /alerts/{id}/samples", protect(s.deps.Verifier, s.deps.Store, auth.RoleViewer, http.HandlerFunc(s.handleListAlertSamples)))
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/api/... -v`
Expected: PASS, including all 5 new tests, no regressions in the existing suite.

- [ ] **Step 6: Run the full suite**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: clean build, no vet issues, all tests passing.

- [ ] **Step 7: Commit and push**

```bash
git add internal/api/alerts.go internal/api/server.go internal/api/alerts_test.go
git commit -m "Add POST /alerts/{id}/mute and GET /alerts/{id}/samples endpoints"
git push
```

This pushes directly to the still-open PR #1 branch, per the Global Constraints.

---

### Task 3: siem-web — extend `siemApiClient.ts`

**Worktree:** `siem-web-console/siem-web`

**Files:**
- Modify: `src/lib/server/siemApiClient.ts`
- Modify: `src/lib/server/siemApiClient.test.ts`

**Interfaces:**
- Consumes: `POST /alerts/{id}/mute`, `GET /alerts/{id}/samples`, `POST /alerts/{id}/ack`,
  `GET /rules` (Task 2, plus siem-api's already-existing ack/rules endpoints from earlier
  sub-projects).
- Produces: `AlertSample`, `RuleResponse` types; `ackAlert`, `muteAlert`, `getAlertSamples`,
  `getRules` client methods — consumed by Tasks 4–6.

The existing `request<T>` helper always calls `res.json()`, which would throw on the
`204 No Content` responses `ack`/`mute` return. This task adds a sibling
`requestNoContent` helper for those two calls, alongside `request<T>` (unchanged) for the
two new GETs.

- [ ] **Step 1: Write the failing tests**

Add to `src/lib/server/siemApiClient.test.ts`:
```ts
it('ackAlert POSTs with Authorization and no body', async () => {
	const fetchFn = vi.fn(async () => new Response(null, { status: 204 }));
	const client = new SiemApiClient({ baseUrl: 'http://siem-api:8080' }, fetchFn);

	await client.ackAlert('token-123', 42);

	const [url, init] = fetchFn.mock.calls[0];
	expect(url).toBe('http://siem-api:8080/alerts/42/ack');
	expect(init?.method).toBe('POST');
	expect((init?.headers as Record<string, string>).Authorization).toBe('Bearer token-123');
});

it('muteAlert POSTs with Authorization and no body', async () => {
	const fetchFn = vi.fn(async () => new Response(null, { status: 204 }));
	const client = new SiemApiClient({ baseUrl: 'http://siem-api:8080' }, fetchFn);

	await client.muteAlert('token-123', 42);

	const [url, init] = fetchFn.mock.calls[0];
	expect(url).toBe('http://siem-api:8080/alerts/42/mute');
	expect(init?.method).toBe('POST');
	expect((init?.headers as Record<string, string>).Authorization).toBe('Bearer token-123');
});

it('getAlertSamples attaches Authorization and parses the response', async () => {
	const fetchFn = fakeFetch([{ id: 1, ts: '2026-08-02T00:00:00Z', line: '{"src_ip":"10.0.0.5"}' }]);
	const client = new SiemApiClient({ baseUrl: 'http://siem-api:8080' }, fetchFn);

	const result = await client.getAlertSamples('token-123', 42);

	expect(result).toHaveLength(1);
	const [url, init] = fetchFn.mock.calls[0];
	expect(url).toBe('http://siem-api:8080/alerts/42/samples');
	expect((init?.headers as Record<string, string>).Authorization).toBe('Bearer token-123');
});

it('getRules attaches Authorization and parses the response', async () => {
	const fetchFn = fakeFetch([
		{
			id: 1,
			name: 'wan-portscan',
			shape: 'threshold',
			logql: '{job="siem"}',
			window_sec: 60,
			group_by: [],
			severity: 'critical',
			destinations: ['inapp'],
			cooldown_sec: 3600,
			interval_sec: 60,
			enabled: true
		}
	]);
	const client = new SiemApiClient({ baseUrl: 'http://siem-api:8080' }, fetchFn);

	const result = await client.getRules('token-123');

	expect(result).toHaveLength(1);
	const [url] = fetchFn.mock.calls[0];
	expect(url).toBe('http://siem-api:8080/rules');
});
```

- [ ] **Step 2: Run them to verify they fail**

Run: `pnpm vitest run src/lib/server/siemApiClient.test.ts`
Expected: FAIL — `ackAlert`/`muteAlert`/`getAlertSamples`/`getRules` don't exist yet.

- [ ] **Step 3: Add the types and methods**

In `src/lib/server/siemApiClient.ts`, add after the `AlertResponse` interface:
```ts
export interface AlertSample {
	id: number;
	ts: string;
	line: string;
}

export interface RuleResponse {
	id: number;
	name: string;
	shape: string;
	logql: string;
	window_sec: number;
	threshold?: number;
	group_by: string[];
	severity: string;
	destinations: string[];
	cooldown_sec: number;
	interval_sec: number;
	enabled: boolean;
	last_run_at?: string;
}
```

Add methods to `SiemApiClient`, after `getAlerts`:
```ts
	async ackAlert(sessionToken: string, id: number): Promise<void> {
		return this.requestNoContent(`/alerts/${id}/ack`, { method: 'POST', ...this.authInit(sessionToken) });
	}

	async muteAlert(sessionToken: string, id: number): Promise<void> {
		return this.requestNoContent(`/alerts/${id}/mute`, { method: 'POST', ...this.authInit(sessionToken) });
	}

	async getAlertSamples(sessionToken: string, id: number): Promise<AlertSample[]> {
		return this.request<AlertSample[]>(`/alerts/${id}/samples`, this.authInit(sessionToken));
	}

	async getRules(sessionToken: string): Promise<RuleResponse[]> {
		return this.request<RuleResponse[]>('/rules', this.authInit(sessionToken));
	}
```

Add the `requestNoContent` helper, right after the existing `request<T>` method:
```ts
	private async requestNoContent(path: string, init: RequestInit): Promise<void> {
		const res = await this.fetchFn(`${this.baseUrl}${path}`, init);
		if (!res.ok) {
			throw new SiemApiError(res.status, await res.text());
		}
	}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `pnpm vitest run src/lib/server/siemApiClient.test.ts`
Expected: PASS (all tests, old and new).

- [ ] **Step 5: Run the full suite and typecheck**

Run: `pnpm vitest run && pnpm check`
Expected: all clean, no regressions.

- [ ] **Step 6: Commit**

```bash
git add src/lib/server/siemApiClient.ts src/lib/server/siemApiClient.test.ts
git commit -m "Extend siemApiClient with ack/mute/samples/rules methods"
```

---

### Task 4: siem-web — `src/lib/alerts.ts` data-shaping helpers

**Worktree:** `siem-web-console/siem-web`

**Files:**
- Create: `src/lib/alerts.ts`
- Test: `src/lib/alerts.test.ts`

**Interfaces:**
- Consumes: `AlertSample` (Task 3).
- Produces: `AlertStats`, `deriveAlertStats(samples: AlertSample[]): AlertStats` — consumed
  by Task 6 (`+page.server.ts`) and Task 7 (`AlertDetail.svelte`).

Mirrors `wall.ts`'s `deriveCountryBreakdown` pattern exactly: parse each sample's `line` as
JSON, read structured fields (`src_ip`, `dst_port` — per the design's label-discipline
section, these are structured metadata in the log line, never labels), skip malformed or
missing data silently.

- [ ] **Step 1: Write the failing test**

`src/lib/alerts.test.ts`:
```ts
import { describe, it, expect } from 'vitest';
import { deriveAlertStats } from './alerts';
import type { AlertSample } from './server/siemApiClient';

function sample(line: string): AlertSample {
	return { id: 1, ts: '2026-08-02T00:00:00Z', line };
}

describe('deriveAlertStats', () => {
	it('extracts distinct ports and source IPs from sample JSON', () => {
		const samples = [
			sample('{"src_ip":"10.0.0.5","dst_port":443}'),
			sample('{"src_ip":"10.0.0.5","dst_port":22}'),
			sample('{"src_ip":"10.0.0.9","dst_port":443}')
		];

		const stats = deriveAlertStats(samples);

		expect(stats.matchedEvents).toBe(3);
		expect(stats.distinctPorts).toEqual([22, 443]);
		expect(stats.sourceIps).toEqual(['10.0.0.5', '10.0.0.9']);
		expect(stats.reputation).toBe('unknown');
	});

	it('skips malformed JSON and entries with no src_ip/dst_port', () => {
		const samples = [sample('not json'), sample('{}'), sample('{"src_ip":"10.0.0.5"}')];

		const stats = deriveAlertStats(samples);

		expect(stats.matchedEvents).toBe(3);
		expect(stats.distinctPorts).toEqual([]);
		expect(stats.sourceIps).toEqual(['10.0.0.5']);
	});
});
```

- [ ] **Step 2: Run it to verify it fails**

Run: `pnpm vitest run src/lib/alerts.test.ts`
Expected: FAIL — `alerts.ts` doesn't exist yet.

- [ ] **Step 3: Write the implementation**

`src/lib/alerts.ts`:
```ts
import type { AlertSample } from './server/siemApiClient';

export interface AlertStats {
	matchedEvents: number;
	distinctPorts: number[];
	sourceIps: string[];
	reputation: string;
}

export function deriveAlertStats(samples: AlertSample[]): AlertStats {
	const ports = new Set<number>();
	const ips = new Set<string>();

	for (const sample of samples) {
		let parsed: unknown;
		try {
			parsed = JSON.parse(sample.line);
		} catch {
			continue;
		}
		if (typeof parsed !== 'object' || parsed === null) continue;

		const obj = parsed as Record<string, unknown>;
		if (typeof obj.dst_port === 'number') ports.add(obj.dst_port);
		if (typeof obj.src_ip === 'string' && obj.src_ip !== '') ips.add(obj.src_ip);
	}

	return {
		matchedEvents: samples.length,
		distinctPorts: [...ports].sort((a, b) => a - b),
		sourceIps: [...ips],
		reputation: 'unknown'
	};
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `pnpm vitest run src/lib/alerts.test.ts`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add src/lib/alerts.ts src/lib/alerts.test.ts
git commit -m "Add siem-web alerts: port/source-IP stat derivation from samples"
```

---

### Task 5: siem-web — SSE proxy + ack/mute passthrough routes

**Worktree:** `siem-web-console/siem-web`

**Files:**
- Create: `src/routes/api/alerts-proxy/+server.ts`
- Test: `src/routes/api/alerts-proxy/server.test.ts`
- Create: `src/routes/api/alerts/[id]/ack/+server.ts`
- Test: `src/routes/api/alerts/[id]/ack/server.test.ts`
- Create: `src/routes/api/alerts/[id]/mute/+server.ts`
- Test: `src/routes/api/alerts/[id]/mute/server.test.ts`

**Interfaces:**
- Consumes: `SiemApiClient.ackAlert`/`muteAlert` (Task 3), `locals.sessionToken` (auth gate,
  already built).
- Produces: same-origin `GET /api/alerts-proxy` (SSE) and `POST /api/alerts/{id}/ack`,
  `POST /api/alerts/{id}/mute` — consumed by Task 6 and 7's components, and Task 8's
  `TriageCard` wiring.

`api/alerts-proxy` mirrors `api/tail-proxy` from the Wall sub-project exactly, targeting
siem-api's `/alerts/stream` instead of `/events/tail`. The ack/mute routes are thin
passthroughs — the bearer token never reaches the browser, matching the same boundary as
every other siem-api call in this app.

- [ ] **Step 1: Write the failing tests**

`src/routes/api/alerts-proxy/server.test.ts`:
```ts
import { describe, it, expect, vi } from 'vitest';
import { GET } from './+server';

vi.mock('$env/dynamic/private', () => ({ env: { API_URL: 'http://siem-api:8080' } }));

describe('GET /api/alerts-proxy', () => {
	it('forwards the Authorization header to siem-api and streams the response', async () => {
		const fetchFn = vi.fn().mockResolvedValue(new Response(new ReadableStream(), { status: 200 }));

		const response = await GET({ locals: { sessionToken: 'token-123' }, fetch: fetchFn } as never);

		expect(fetchFn).toHaveBeenCalledWith(
			'http://siem-api:8080/alerts/stream',
			expect.objectContaining({ headers: { Authorization: 'Bearer token-123' } })
		);
		expect(response.headers.get('Content-Type')).toBe('text/event-stream');
		expect(response.status).toBe(200);
	});
});
```

`src/routes/api/alerts/[id]/ack/server.test.ts`:
```ts
import { describe, it, expect, vi } from 'vitest';
import { POST } from './+server';
import * as siemApiClientModule from '$lib/server/siemApiClient';

vi.mock('$env/dynamic/private', () => ({ env: { API_URL: 'http://siem-api:8080' } }));

vi.mock('$lib/server/siemApiClient', async (importOriginal) => {
	const actual = await importOriginal<typeof import('$lib/server/siemApiClient')>();
	return { ...actual, SiemApiClient: vi.fn() };
});

describe('POST /api/alerts/[id]/ack', () => {
	it('calls ackAlert with the session token and returns 204', async () => {
		const ackAlertMock = vi.fn().mockResolvedValue(undefined);
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return { ackAlert: ackAlertMock };
		});

		const response = await POST({
			params: { id: '42' },
			locals: { sessionToken: 'token-123' }
		} as never);

		expect(ackAlertMock).toHaveBeenCalledWith('token-123', 42);
		expect(response.status).toBe(204);
	});

	it('propagates a SiemApiError status code as a JSON error response', async () => {
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return {
				ackAlert: vi.fn().mockRejectedValue(new siemApiClientModule.SiemApiError(403, 'denied'))
			};
		});

		const response = await POST({
			params: { id: '42' },
			locals: { sessionToken: 'token-123' }
		} as never);

		expect(response.status).toBe(403);
	});
});
```

`src/routes/api/alerts/[id]/mute/server.test.ts`:
```ts
import { describe, it, expect, vi } from 'vitest';
import { POST } from './+server';
import * as siemApiClientModule from '$lib/server/siemApiClient';

vi.mock('$env/dynamic/private', () => ({ env: { API_URL: 'http://siem-api:8080' } }));

vi.mock('$lib/server/siemApiClient', async (importOriginal) => {
	const actual = await importOriginal<typeof import('$lib/server/siemApiClient')>();
	return { ...actual, SiemApiClient: vi.fn() };
});

describe('POST /api/alerts/[id]/mute', () => {
	it('calls muteAlert with the session token and returns 204', async () => {
		const muteAlertMock = vi.fn().mockResolvedValue(undefined);
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return { muteAlert: muteAlertMock };
		});

		const response = await POST({
			params: { id: '42' },
			locals: { sessionToken: 'token-123' }
		} as never);

		expect(muteAlertMock).toHaveBeenCalledWith('token-123', 42);
		expect(response.status).toBe(204);
	});

	it('propagates a SiemApiError status code as a JSON error response', async () => {
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return {
				muteAlert: vi.fn().mockRejectedValue(new siemApiClientModule.SiemApiError(403, 'denied'))
			};
		});

		const response = await POST({
			params: { id: '42' },
			locals: { sessionToken: 'token-123' }
		} as never);

		expect(response.status).toBe(403);
	});
});
```

- [ ] **Step 2: Run them to verify they fail**

Run: `pnpm vitest run src/routes/api/alerts-proxy/server.test.ts src/routes/api/alerts/`
Expected: FAIL — none of the three `+server.ts` files exist yet.

- [ ] **Step 3: Write the implementations**

`src/routes/api/alerts-proxy/+server.ts`:
```ts
import { env } from '$env/dynamic/private';
import type { RequestHandler } from './$types';

export const GET: RequestHandler = async ({ locals, fetch }) => {
	const token = locals.sessionToken as string;

	const upstream = await fetch(`${env.API_URL}/alerts/stream`, {
		headers: { Authorization: `Bearer ${token}` }
	});

	return new Response(upstream.body, {
		status: upstream.status,
		headers: {
			'Content-Type': 'text/event-stream',
			'Cache-Control': 'no-cache'
		}
	});
};
```

`src/routes/api/alerts/[id]/ack/+server.ts`:
```ts
import { json } from '@sveltejs/kit';
import { env } from '$env/dynamic/private';
import type { RequestHandler } from './$types';
import { SiemApiClient, SiemApiError } from '$lib/server/siemApiClient';

export const POST: RequestHandler = async ({ params, locals }) => {
	const client = new SiemApiClient({ baseUrl: env.API_URL as string });
	const token = locals.sessionToken as string;
	const id = Number(params.id);

	try {
		await client.ackAlert(token, id);
	} catch (err) {
		if (err instanceof SiemApiError) {
			return json({ error: err.message }, { status: err.status });
		}
		throw err;
	}

	return new Response(null, { status: 204 });
};
```

`src/routes/api/alerts/[id]/mute/+server.ts`:
```ts
import { json } from '@sveltejs/kit';
import { env } from '$env/dynamic/private';
import type { RequestHandler } from './$types';
import { SiemApiClient, SiemApiError } from '$lib/server/siemApiClient';

export const POST: RequestHandler = async ({ params, locals }) => {
	const client = new SiemApiClient({ baseUrl: env.API_URL as string });
	const token = locals.sessionToken as string;
	const id = Number(params.id);

	try {
		await client.muteAlert(token, id);
	} catch (err) {
		if (err instanceof SiemApiError) {
			return json({ error: err.message }, { status: err.status });
		}
		throw err;
	}

	return new Response(null, { status: 204 });
};
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `pnpm vitest run src/routes/api/alerts-proxy/server.test.ts src/routes/api/alerts/`
Expected: PASS.

- [ ] **Step 5: Run the full suite and typecheck**

Run: `pnpm vitest run && pnpm check`
Expected: all clean.

- [ ] **Step 6: Commit**

```bash
git add src/routes/api/alerts-proxy src/routes/api/alerts
git commit -m "Add siem-web alerts SSE proxy and ack/mute passthrough routes"
```

---

### Task 6: siem-web — Alerts screen `+page.server.ts`

**Worktree:** `siem-web-console/siem-web`

**Files:**
- Create: `src/routes/alerts/+page.server.ts`
- Test: `src/routes/alerts/page.server.test.ts`

**Interfaces:**
- Consumes: `SiemApiClient.getAlerts`/`getRules`/`getAlertSamples` (Task 3),
  `deriveAlertStats` (Task 4), `locals.sessionToken` (auth gate).
- Produces: the `load` function's return shape — consumed by Task 7's `+page.svelte`:
  `{ tab: 'open'|'acked'|'rules', alerts: AlertResponse[], rules: RuleResponse[], selectedAlert: AlertResponse|null, selectedSamples: AlertSample[], stats: AlertStats|null, selectedRule: RuleResponse|null }`.

Reads `?state=` (default `open`) and `?id=` from the URL. Rules are always fetched
alongside alerts (not only for the Rules tab) so alert rows can show the firing rule's real
name rather than just its numeric id. When `tab === 'rules'` and `?id=` is given,
`selectedRule` is resolved from the same `rules` list — the spec requires that "selecting a
rule row shows its LogQL and destinations in the detail pane" (Task 7 renders this). Same
`SiemApiError` → redirect-to-logout (401/403) or 502 (other) error handling as Wall's
`+page.server.ts`.

- [ ] **Step 1: Write the failing tests**

`src/routes/alerts/page.server.test.ts`:
```ts
import { describe, it, expect, vi } from 'vitest';
import { load } from './+page.server';
import * as siemApiClientModule from '$lib/server/siemApiClient';
import { SiemApiError } from '$lib/server/siemApiClient';

vi.mock('$env/dynamic/private', () => ({ env: { API_URL: 'http://siem-api:8080' } }));

vi.mock('$lib/server/siemApiClient', async (importOriginal) => {
	const actual = await importOriginal<typeof import('$lib/server/siemApiClient')>();
	return { ...actual, SiemApiClient: vi.fn() };
});

function fakeAlert(overrides: Partial<{ id: number; state: string; rule_id: number }> = {}) {
	return {
		id: 1,
		rule_id: 1,
		group_key: 'a',
		severity: 'critical',
		title: 't',
		body: 'b',
		event_count: 1,
		state: 'open',
		first_seen_at: '2026-08-02T00:00:00Z',
		last_seen_at: '2026-08-02T00:00:00Z',
		...overrides
	};
}

describe('Alerts load', () => {
	it('defaults to the open tab and loads alerts plus rules for it', async () => {
		const getAlertsMock = vi.fn().mockResolvedValue([fakeAlert()]);
		const getRulesMock = vi.fn().mockResolvedValue([]);
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return { getAlerts: getAlertsMock, getRules: getRulesMock, getAlertSamples: vi.fn() };
		});

		const result = (await load({
			locals: { sessionToken: 'token-123' },
			url: new URL('https://siem.townsville.cc/alerts')
		} as never)) as Exclude<Awaited<ReturnType<typeof load>>, void>;

		expect(result.tab).toBe('open');
		expect(result.alerts).toHaveLength(1);
		expect(getAlertsMock).toHaveBeenCalledWith('token-123', 'open');
		expect(getRulesMock).toHaveBeenCalledWith('token-123');
	});

	it('loads rules and no alerts when state=rules', async () => {
		const getAlertsMock = vi.fn();
		const getRulesMock = vi.fn().mockResolvedValue([
			{
				id: 1,
				name: 'wan-portscan',
				shape: 'threshold',
				logql: '{job="siem"}',
				window_sec: 60,
				group_by: [],
				severity: 'critical',
				destinations: ['inapp'],
				cooldown_sec: 3600,
				interval_sec: 60,
				enabled: true
			}
		]);
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return { getAlerts: getAlertsMock, getRules: getRulesMock, getAlertSamples: vi.fn() };
		});

		const result = (await load({
			locals: { sessionToken: 'token-123' },
			url: new URL('https://siem.townsville.cc/alerts?state=rules')
		} as never)) as Exclude<Awaited<ReturnType<typeof load>>, void>;

		expect(result.tab).toBe('rules');
		expect(result.rules).toHaveLength(1);
		expect(result.alerts).toEqual([]);
		expect(getAlertsMock).not.toHaveBeenCalled();
	});

	it('resolves selectedRule from the rules list when state=rules and id is given', async () => {
		const rule = {
			id: 5,
			name: 'wan-portscan',
			shape: 'threshold',
			logql: '{job="siem"}',
			window_sec: 60,
			group_by: [],
			severity: 'critical',
			destinations: ['inapp'],
			cooldown_sec: 3600,
			interval_sec: 60,
			enabled: true
		};
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return { getAlerts: vi.fn(), getRules: vi.fn().mockResolvedValue([rule]), getAlertSamples: vi.fn() };
		});

		const result = (await load({
			locals: { sessionToken: 'token-123' },
			url: new URL('https://siem.townsville.cc/alerts?state=rules&id=5')
		} as never)) as Exclude<Awaited<ReturnType<typeof load>>, void>;

		expect(result.selectedRule?.id).toBe(5);
		expect(result.selectedAlert).toBeNull();
	});

	it('loads samples and stats for the selected alert when id is given', async () => {
		const alert = fakeAlert({ id: 7 });
		const getAlertSamplesMock = vi
			.fn()
			.mockResolvedValue([{ id: 1, ts: '2026-08-02T00:00:00Z', line: '{"src_ip":"10.0.0.5","dst_port":443}' }]);
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return {
				getAlerts: vi.fn().mockResolvedValue([alert]),
				getRules: vi.fn().mockResolvedValue([]),
				getAlertSamples: getAlertSamplesMock
			};
		});

		const result = (await load({
			locals: { sessionToken: 'token-123' },
			url: new URL('https://siem.townsville.cc/alerts?id=7')
		} as never)) as Exclude<Awaited<ReturnType<typeof load>>, void>;

		expect(result.selectedAlert?.id).toBe(7);
		expect(result.selectedSamples).toHaveLength(1);
		expect(result.stats?.distinctPorts).toEqual([443]);
		expect(getAlertSamplesMock).toHaveBeenCalledWith('token-123', 7);
	});

	it('redirects to /auth/logout on a 401/403 from siem-api', async () => {
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return {
				getAlerts: vi.fn().mockRejectedValue(new SiemApiError(401, 'invalid session')),
				getRules: vi.fn().mockResolvedValue([]),
				getAlertSamples: vi.fn()
			};
		});

		await expect(
			load({
				locals: { sessionToken: 'stale-token' },
				url: new URL('https://siem.townsville.cc/alerts')
			} as never)
		).rejects.toMatchObject({ status: 302, location: '/auth/logout' });
	});

	it('surfaces a 502 when siem-api fails for a reason other than auth', async () => {
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return {
				getAlerts: vi.fn().mockRejectedValue(new SiemApiError(500, 'boom')),
				getRules: vi.fn().mockResolvedValue([]),
				getAlertSamples: vi.fn()
			};
		});

		await expect(
			load({
				locals: { sessionToken: 'token-123' },
				url: new URL('https://siem.townsville.cc/alerts')
			} as never)
		).rejects.toMatchObject({ status: 502 });
	});
});
```

- [ ] **Step 2: Run them to verify they fail**

Run: `pnpm vitest run src/routes/alerts/page.server.test.ts`
Expected: FAIL — `+page.server.ts` doesn't exist yet.

- [ ] **Step 3: Write the implementation**

`src/routes/alerts/+page.server.ts`:
```ts
import { error, redirect } from '@sveltejs/kit';
import { env } from '$env/dynamic/private';
import type { PageServerLoad } from './$types';
import { SiemApiClient, SiemApiError } from '$lib/server/siemApiClient';
import { deriveAlertStats } from '$lib/alerts';

export const load: PageServerLoad = async ({ locals, url }) => {
	const client = new SiemApiClient({ baseUrl: env.API_URL as string });
	const token = locals.sessionToken as string;

	const tabParam = url.searchParams.get('state');
	const tab: 'open' | 'acked' | 'rules' =
		tabParam === 'acked' || tabParam === 'rules' ? tabParam : 'open';
	const selectedId = url.searchParams.get('id');

	let alerts, rules;
	try {
		[alerts, rules] = await Promise.all([
			tab === 'rules' ? Promise.resolve([]) : client.getAlerts(token, tab),
			client.getRules(token)
		]);
	} catch (err) {
		if (err instanceof SiemApiError) {
			if (err.status === 401 || err.status === 403) {
				redirect(302, '/auth/logout');
			}
			error(502, 'siem-api unavailable');
		}
		throw err;
	}

	const selectedAlert =
		tab !== 'rules' && selectedId
			? (alerts.find((a) => a.id === Number(selectedId)) ?? null)
			: null;
	const selectedRule =
		tab === 'rules' && selectedId ? (rules.find((r) => r.id === Number(selectedId)) ?? null) : null;

	let selectedSamples;
	try {
		selectedSamples = selectedAlert ? await client.getAlertSamples(token, selectedAlert.id) : [];
	} catch (err) {
		if (err instanceof SiemApiError) {
			if (err.status === 401 || err.status === 403) {
				redirect(302, '/auth/logout');
			}
			error(502, 'siem-api unavailable');
		}
		throw err;
	}

	return {
		tab,
		alerts,
		rules,
		selectedAlert,
		selectedSamples,
		stats: selectedAlert ? deriveAlertStats(selectedSamples) : null,
		selectedRule
	};
};
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `pnpm vitest run src/routes/alerts/page.server.test.ts`
Expected: PASS.

- [ ] **Step 5: Run the full suite and typecheck**

Run: `pnpm vitest run && pnpm check`
Expected: all clean.

- [ ] **Step 6: Commit**

```bash
git add src/routes/alerts/+page.server.ts src/routes/alerts/page.server.test.ts
git commit -m "Add siem-web Alerts screen data load"
```

---

### Task 7: siem-web — Alerts screen components + `+page.svelte`

**Worktree:** `siem-web-console/siem-web`

**Files:**
- Create: `src/lib/components/AlertRow.svelte`
- Create: `src/lib/components/RuleRow.svelte`
- Create: `src/lib/components/AlertInbox.svelte`
- Create: `src/lib/components/AlertDetail.svelte`
- Create: `src/lib/components/RuleDetail.svelte`
- Create: `src/routes/alerts/+page.svelte`

**Interfaces:**
- Consumes: Task 6's `load` return shape (including `selectedRule`), `AlertStats` (Task 4).
- Produces: the rendered Alerts screen.

`+page.svelte` has three states depending on `data`: an alert selected (renders
`AlertDetail`), a rule selected on the Rules tab (renders the new `RuleDetail` — separate
from `AlertDetail` since a rule and an alert are different shapes with no meaningful shared
markup), or nothing selected (empty-state message).

Presentational, no unit tests, per the Global Constraints testing split. This is a first
pass at the mockup's visual fidelity (unlike the Wall sub-project's components, this screen
has no verbatim reference markup available to transcribe) — layout, spacing, and colors
follow the design tokens and the handoff's Screen 4 prose description; refine visually
against `design_handoff_homesiem/designs/homeSIEM Console - Build.dc.html` during Task 9's
final verification pass.

- [ ] **Step 1: Write `AlertRow.svelte`**

`src/lib/components/AlertRow.svelte`:
```svelte
<script lang="ts">
	import type { AlertResponse } from '$lib/server/siemApiClient';

	let {
		alert,
		ruleName,
		selected
	}: { alert: AlertResponse; ruleName: string; selected: boolean } = $props();

	function ageLabel(iso: string): string {
		const ms = Date.now() - new Date(iso).getTime();
		const minutes = Math.floor(ms / 60000);
		if (minutes < 60) return `${minutes}m`;
		return `${Math.floor(minutes / 60)}h`;
	}
</script>

<a
	class="row severity-{alert.severity}"
	class:selected
	href="?state={alert.state === 'acked' ? 'acked' : 'open'}&id={alert.id}"
>
	<div class="header">
		<span class="eyebrow">{alert.severity}</span>
		<span class="age">{ageLabel(alert.first_seen_at)}</span>
	</div>
	<div class="title">{alert.title}</div>
	<div class="body">{alert.body}</div>
	<div class="rule">{ruleName}</div>
</a>

<style>
	.row {
		display: block;
		background: var(--color-surface-2);
		border-radius: var(--radius-default);
		padding: var(--space-4);
		border-left: 3px solid var(--color-severity-critical);
		text-decoration: none;
		color: inherit;
	}
	.row.severity-warning {
		border-left-color: var(--color-severity-warning);
	}
	.row.severity-low,
	.row.severity-medium {
		border-left-color: var(--color-severity-info);
	}
	.row.selected {
		background: var(--color-accent-tint);
		box-shadow: 0 0 0 1px var(--color-accent-deep);
	}
	.header {
		display: flex;
		justify-content: space-between;
		font-size: var(--text-eyebrow);
		text-transform: uppercase;
		color: var(--color-severity-critical);
	}
	.age {
		color: var(--color-muted);
		text-transform: none;
	}
	.title {
		font-size: 13.5px;
		font-weight: 500;
		margin-top: var(--space-2);
	}
	.body {
		font-size: var(--text-table);
		color: var(--color-muted);
		margin-top: var(--space-1);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.rule {
		font-family: var(--font-mono);
		font-size: 10.5px;
		color: var(--color-muted-2);
		margin-top: var(--space-2);
	}
</style>
```

- [ ] **Step 2: Write `RuleRow.svelte`**

`src/lib/components/RuleRow.svelte`:
```svelte
<script lang="ts">
	import type { RuleResponse } from '$lib/server/siemApiClient';

	let { rule, selected }: { rule: RuleResponse; selected: boolean } = $props();
</script>

<a class="row" class:selected href="?state=rules&id={rule.id}">
	<div class="header">
		<span class="name">{rule.name}</span>
		<span class="enabled" class:off={!rule.enabled}>{rule.enabled ? 'enabled' : 'disabled'}</span>
	</div>
	<div class="shape">{rule.shape}</div>
	<div class="logql">{rule.logql}</div>
</a>

<style>
	.row {
		display: block;
		background: var(--color-surface-2);
		border-radius: var(--radius-default);
		padding: var(--space-4);
		text-decoration: none;
		color: inherit;
	}
	.row.selected {
		background: var(--color-accent-tint);
		box-shadow: 0 0 0 1px var(--color-accent-deep);
	}
	.header {
		display: flex;
		justify-content: space-between;
	}
	.name {
		font-size: 13.5px;
		font-weight: 500;
	}
	.enabled {
		font-size: var(--text-eyebrow);
		text-transform: uppercase;
		color: var(--color-severity-healthy);
	}
	.enabled.off {
		color: var(--color-muted-2);
	}
	.shape {
		font-size: var(--text-eyebrow);
		text-transform: uppercase;
		color: var(--color-muted-2);
		margin-top: var(--space-1);
	}
	.logql {
		font-family: var(--font-mono);
		font-size: var(--text-label);
		color: var(--color-muted);
		margin-top: var(--space-2);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
</style>
```

- [ ] **Step 3: Write `AlertInbox.svelte`**

`src/lib/components/AlertInbox.svelte`:
```svelte
<script lang="ts">
	import type { AlertResponse, RuleResponse } from '$lib/server/siemApiClient';
	import AlertRow from './AlertRow.svelte';
	import RuleRow from './RuleRow.svelte';

	let {
		tab,
		alerts,
		rules,
		selectedId
	}: {
		tab: 'open' | 'acked' | 'rules';
		alerts: AlertResponse[];
		rules: RuleResponse[];
		selectedId: number | null;
	} = $props();

	const ruleNames = $derived(new Map(rules.map((r) => [r.id, r.name])));
	const tabs: { label: string; value: 'open' | 'acked' | 'rules' }[] = [
		{ label: 'Open', value: 'open' },
		{ label: 'Acked', value: 'acked' },
		{ label: 'Rules', value: 'rules' }
	];
</script>

<div class="inbox">
	<div class="header">
		<span class="title">Alerts</span>
		<div class="tabs">
			{#each tabs as t (t.value)}
				<a href="?state={t.value}" class:active={tab === t.value}>{t.label}</a>
			{/each}
		</div>
	</div>
	<div class="rows">
		{#if tab === 'rules'}
			{#each rules as rule (rule.id)}
				<RuleRow {rule} selected={selectedId === rule.id} />
			{/each}
		{:else}
			{#each alerts as alert (alert.id)}
				<AlertRow
					{alert}
					ruleName={ruleNames.get(alert.rule_id) ?? `rule #${alert.rule_id}`}
					selected={selectedId === alert.id}
				/>
			{/each}
		{/if}
	</div>
</div>

<style>
	.inbox {
		width: 376px;
		flex-shrink: 0;
		display: flex;
		flex-direction: column;
		gap: var(--space-4);
	}
	.header {
		display: flex;
		align-items: center;
		justify-content: space-between;
	}
	.title {
		font-size: var(--text-page-title);
		font-weight: 500;
	}
	.tabs {
		display: flex;
		gap: var(--space-3);
		font-size: var(--text-table);
	}
	.tabs a {
		color: var(--color-muted);
		text-decoration: none;
		padding: var(--space-1) var(--space-3);
		border-radius: var(--radius-sm);
	}
	.tabs a.active {
		background: var(--color-accent-tint);
		color: var(--color-accent-lighter);
	}
	.rows {
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
	}
</style>
```

- [ ] **Step 4: Write `AlertDetail.svelte`**

`src/lib/components/AlertDetail.svelte`:
```svelte
<script lang="ts">
	import { invalidateAll } from '$app/navigation';
	import type { AlertResponse, AlertSample, RuleResponse } from '$lib/server/siemApiClient';
	import type { AlertStats } from '$lib/alerts';

	let {
		alert,
		samples,
		stats,
		rule
	}: {
		alert: AlertResponse;
		samples: AlertSample[];
		stats: AlertStats;
		rule: RuleResponse | undefined;
	} = $props();

	let acking = $state(false);
	let muting = $state(false);

	async function acknowledge() {
		acking = true;
		try {
			await fetch(`/api/alerts/${alert.id}/ack`, { method: 'POST' });
			await invalidateAll();
		} finally {
			acking = false;
		}
	}

	async function mute() {
		muting = true;
		try {
			await fetch(`/api/alerts/${alert.id}/mute`, { method: 'POST' });
			await invalidateAll();
		} finally {
			muting = false;
		}
	}
</script>

<div class="detail">
	<div class="header">
		<div class="title-block">
			<div class="eyebrow-row">
				<span class="eyebrow severity-{alert.severity}">{alert.severity}</span>
				{#if alert.state === 'open'}
					<span class="tag">unacknowledged</span>
				{/if}
			</div>
			<h1>{alert.title}</h1>
			<p class="body">{alert.body}</p>
		</div>
		<div class="actions">
			<button class="primary" onclick={acknowledge} disabled={acking || alert.state !== 'open'}>
				Acknowledge
			</button>
			<button
				class="ghost"
				disabled
				title="Not implemented — SOAR-style automated response is out of scope for v1"
			>
				Block at gateway
			</button>
			<button class="ghost" onclick={mute} disabled={muting}>Mute rule 1h</button>
		</div>
	</div>

	<div class="stats">
		<div class="stat">
			<span class="label">Matched events</span>
			<span class="value">{stats.matchedEvents}</span>
		</div>
		<div class="stat">
			<span class="label">Distinct ports</span>
			<span class="value">{stats.distinctPorts.length}</span>
		</div>
		<div class="stat">
			<span class="label">Source IP</span>
			<span class="value">{stats.sourceIps[0] ?? '—'}</span>
		</div>
		<div class="stat">
			<span class="label">Reputation</span>
			<span class="value">{stats.reputation}</span>
		</div>
	</div>

	{#if stats.distinctPorts.length > 0}
		<div class="ports">
			<span class="label">Ports touched, in order</span>
			<div class="chips">
				{#each stats.distinctPorts as port (port)}
					<span class="chip">{port}</span>
				{/each}
			</div>
		</div>
	{/if}

	<div class="matched-events">
		<span class="label">Matched events</span>
		<div class="log-block">
			{#each samples as sample (sample.id)}
				<div class="log-line">{sample.line}</div>
			{:else}
				<div class="log-line empty">No samples recorded yet.</div>
			{/each}
		</div>
	</div>

	<div class="rule-panel">
		<span class="label">Rule that fired</span>
		{#if rule}
			<div class="rule-name">{rule.name}</div>
			<div class="rule-meta">
				<span class="enabled" class:off={!rule.enabled}>{rule.enabled ? 'enabled' : 'disabled'}</span>
				<span class="destinations">{rule.destinations.join(', ')}</span>
			</div>
			<div class="logql-block">{rule.logql}</div>
		{:else}
			<div class="rule-name">rule #{alert.rule_id}</div>
		{/if}
	</div>
</div>

<style>
	.detail {
		flex: 1;
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: var(--space-5);
	}
	.header {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-4);
	}
	.title-block {
		flex: 1 1 380px;
	}
	.eyebrow-row {
		display: flex;
		align-items: center;
		gap: var(--space-3);
	}
	.eyebrow {
		font-size: var(--text-eyebrow);
		text-transform: uppercase;
		color: var(--color-severity-critical);
	}
	.eyebrow.severity-warning {
		color: var(--color-severity-warning);
	}
	.eyebrow.severity-low,
	.eyebrow.severity-medium {
		color: var(--color-severity-info);
	}
	.tag {
		font-size: var(--text-eyebrow);
		text-transform: uppercase;
		background: var(--color-accent-tint);
		color: var(--color-accent-lighter);
		border-radius: var(--radius-sm);
		padding: 0 var(--space-2);
	}
	h1 {
		font-size: 26px;
		font-weight: 500;
		margin: var(--space-2) 0 0;
	}
	.body {
		max-width: 68ch;
		color: var(--color-muted);
		margin-top: var(--space-2);
	}
	.actions {
		display: flex;
		gap: var(--space-3);
		align-items: flex-start;
	}
	.primary {
		background: transparent;
		border: 1px solid var(--color-accent);
		color: var(--color-text);
		border-radius: var(--radius-sm);
		padding: var(--space-2) var(--space-4);
		font-size: var(--text-table);
	}
	.ghost {
		background: none;
		border: 1px solid var(--color-line-2);
		color: var(--color-accent-light);
		border-radius: var(--radius-sm);
		padding: var(--space-2) var(--space-4);
		font-size: var(--text-table);
	}
	.ghost:disabled,
	.primary:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}
	.stats {
		display: grid;
		grid-template-columns: repeat(4, 1fr);
		gap: var(--space-4);
	}
	.stat {
		background: var(--color-surface-2);
		border-radius: var(--radius-default);
		padding: var(--space-4);
		display: flex;
		flex-direction: column;
		gap: var(--space-1);
	}
	.label {
		font-size: var(--text-eyebrow);
		text-transform: uppercase;
		color: var(--color-muted-2);
	}
	.value {
		font-size: 20px;
		font-weight: 500;
	}
	.chips {
		display: flex;
		gap: var(--space-2);
		margin-top: var(--space-2);
		flex-wrap: wrap;
	}
	.chip {
		font-family: var(--font-mono);
		font-size: var(--text-label);
		background: var(--color-accent-tint);
		color: var(--color-accent-lighter);
		border-radius: var(--radius-sm);
		padding: var(--space-1) var(--space-2);
	}
	.log-block {
		background: var(--color-surface-2);
		border-radius: var(--radius-default);
		padding: var(--space-3);
		margin-top: var(--space-2);
		font-family: var(--font-mono);
		font-size: var(--text-log-row);
		line-height: var(--line-height-log);
	}
	.log-line {
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}
	.log-line.empty {
		color: var(--color-muted-2);
	}
	.rule-panel {
		background: var(--color-surface-2);
		border-radius: var(--radius-default);
		padding: var(--space-4);
	}
	.rule-name {
		font-size: 13.5px;
		font-weight: 500;
		margin-top: var(--space-2);
	}
	.rule-meta {
		display: flex;
		gap: var(--space-3);
		font-size: var(--text-table);
		color: var(--color-muted);
		margin-top: var(--space-1);
	}
	.enabled {
		text-transform: uppercase;
		font-size: var(--text-eyebrow);
		color: var(--color-severity-healthy);
	}
	.enabled.off {
		color: var(--color-muted-2);
	}
	.logql-block {
		font-family: var(--font-mono);
		font-size: var(--text-label);
		color: var(--color-muted);
		background: var(--color-surface-3);
		border-radius: var(--radius-sm);
		padding: var(--space-2) var(--space-3);
		margin-top: var(--space-2);
		overflow-x: auto;
		white-space: nowrap;
	}
</style>
```

- [ ] **Step 5: Write `RuleDetail.svelte`**

`src/lib/components/RuleDetail.svelte`:
```svelte
<script lang="ts">
	import type { RuleResponse } from '$lib/server/siemApiClient';

	let { rule }: { rule: RuleResponse } = $props();
</script>

<div class="detail">
	<div class="header">
		<span class="eyebrow">{rule.shape}</span>
		<span class="enabled" class:off={!rule.enabled}>{rule.enabled ? 'enabled' : 'disabled'}</span>
	</div>
	<h1>{rule.name}</h1>
	<div class="meta">
		<span>severity: {rule.severity}</span>
		<span>window: {rule.window_sec}s</span>
		<span>cooldown: {rule.cooldown_sec}s</span>
	</div>
	<div class="destinations">destinations: {rule.destinations.join(', ')}</div>
	<div class="logql-block">{rule.logql}</div>
</div>

<style>
	.detail {
		flex: 1;
		min-width: 0;
		background: var(--color-surface-2);
		border-radius: var(--radius-default);
		padding: var(--space-5);
	}
	.header {
		display: flex;
		justify-content: space-between;
	}
	.eyebrow {
		font-size: var(--text-eyebrow);
		text-transform: uppercase;
		color: var(--color-muted-2);
	}
	.enabled {
		font-size: var(--text-eyebrow);
		text-transform: uppercase;
		color: var(--color-severity-healthy);
	}
	.enabled.off {
		color: var(--color-muted-2);
	}
	h1 {
		font-size: 26px;
		font-weight: 500;
		margin: var(--space-2) 0 0;
	}
	.meta {
		display: flex;
		gap: var(--space-4);
		font-size: var(--text-table);
		color: var(--color-muted);
		margin-top: var(--space-3);
	}
	.destinations {
		font-size: var(--text-table);
		color: var(--color-muted);
		margin-top: var(--space-2);
	}
	.logql-block {
		font-family: var(--font-mono);
		font-size: var(--text-label);
		color: var(--color-muted);
		background: var(--color-surface-3);
		border-radius: var(--radius-sm);
		padding: var(--space-2) var(--space-3);
		margin-top: var(--space-3);
		overflow-x: auto;
		white-space: nowrap;
	}
</style>
```

- [ ] **Step 6: Write `+page.svelte`**

`src/routes/alerts/+page.svelte`:
```svelte
<script lang="ts">
	import { invalidateAll } from '$app/navigation';
	import AlertInbox from '$lib/components/AlertInbox.svelte';
	import AlertDetail from '$lib/components/AlertDetail.svelte';
	import RuleDetail from '$lib/components/RuleDetail.svelte';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	$effect(() => {
		const source = new EventSource('/api/alerts-proxy');
		source.onmessage = () => {
			invalidateAll();
		};
		return () => source.close();
	});
</script>

<div class="alerts">
	<AlertInbox
		tab={data.tab}
		alerts={data.alerts}
		rules={data.rules}
		selectedId={data.selectedAlert?.id ?? data.selectedRule?.id ?? null}
	/>
	{#if data.selectedAlert && data.stats}
		<AlertDetail
			alert={data.selectedAlert}
			samples={data.selectedSamples}
			stats={data.stats}
			rule={data.rules.find((r) => r.id === data.selectedAlert?.rule_id)}
		/>
	{:else if data.selectedRule}
		<RuleDetail rule={data.selectedRule} />
	{:else}
		<div class="empty">Select an alert to see details.</div>
	{/if}
</div>

<style>
	.alerts {
		display: flex;
		gap: var(--space-6);
		padding: var(--space-5) var(--space-6);
	}
	.empty {
		flex: 1;
		display: flex;
		align-items: center;
		justify-content: center;
		color: var(--color-muted-2);
	}
</style>
```

- [ ] **Step 7: Verify it builds**

Run: `pnpm check && pnpm build`
Expected: both succeed.

- [ ] **Step 8: Commit**

```bash
git add src/lib/components/AlertRow.svelte src/lib/components/RuleRow.svelte src/lib/components/AlertInbox.svelte src/lib/components/AlertDetail.svelte src/lib/components/RuleDetail.svelte src/routes/alerts/+page.svelte
git commit -m "Add siem-web Alerts screen components and page"
```

---

### Task 8: siem-web — wire Wall's `TriageCard` + drop Nav's `/alerts` cast

**Worktree:** `siem-web-console/siem-web`

**Files:**
- Modify: `src/lib/components/TriageCard.svelte`
- Modify: `src/lib/components/Nav.svelte`

**Interfaces:**
- Consumes: `/api/alerts/{id}/mute` passthrough route (Task 5), the real `/alerts` route
  (Task 7).

`TriageCard`'s "Investigate" becomes a real link to the Alerts screen; "Mute 1h" calls the
mute passthrough directly (no navigation), with an optimistic-refresh pattern matching
`AlertDetail.svelte`'s own mute button. `Nav.svelte`'s `/alerts` entry drops its `as
Pathname` escape hatch now that the route is real, per the comment already in that file.

Presentational + client-side interactivity, no unit test — same exemption as
`Ticker.svelte`'s `EventSource` logic in the Wall sub-project. Verify via `pnpm check` and
`pnpm build`.

- [ ] **Step 1: Update `TriageCard.svelte`**

Replace the whole file:
```svelte
<script lang="ts">
	import { invalidateAll } from '$app/navigation';
	import type { AlertResponse } from '$lib/server/siemApiClient';

	let { alert }: { alert: AlertResponse } = $props();

	let muting = $state(false);

	function ageLabel(iso: string): string {
		const ms = Date.now() - new Date(iso).getTime();
		const minutes = Math.floor(ms / 60000);
		if (minutes < 60) return `${minutes}m`;
		return `${Math.floor(minutes / 60)}h`;
	}

	async function mute() {
		muting = true;
		try {
			await fetch(`/api/alerts/${alert.id}/mute`, { method: 'POST' });
			await invalidateAll();
		} finally {
			muting = false;
		}
	}
</script>

<div class="card severity-{alert.severity}">
	<div class="header">
		<span class="eyebrow">{alert.severity}</span>
		<span class="age">{ageLabel(alert.first_seen_at)}</span>
	</div>
	<div class="title">{alert.title}</div>
	<div class="body">{alert.body}</div>
	<div class="actions">
		<a class="primary" href="/alerts?id={alert.id}">Investigate</a>
		<button class="ghost" onclick={mute} disabled={muting}>Mute 1h</button>
	</div>
</div>

<style>
	.card {
		background: var(--color-surface-2);
		border-radius: var(--radius-default);
		padding: var(--space-4);
		box-shadow: inset 0 2px 0 var(--color-severity-critical);
	}
	.card.severity-warning {
		box-shadow: inset 0 2px 0 var(--color-severity-warning);
	}
	.card.severity-low,
	.card.severity-medium {
		box-shadow: inset 0 2px 0 var(--color-severity-info);
	}
	.header {
		display: flex;
		justify-content: space-between;
		font-size: var(--text-eyebrow);
		text-transform: uppercase;
		color: var(--color-severity-critical);
	}
	.age {
		color: var(--color-muted);
		text-transform: none;
	}
	.title {
		font-size: 14px;
		font-weight: 500;
		margin-top: var(--space-2);
	}
	.body {
		font-size: 11.5px;
		color: var(--color-muted);
		margin-top: var(--space-1);
	}
	.actions {
		margin-top: var(--space-3);
		display: flex;
		gap: var(--space-3);
	}
	.primary {
		display: inline-block;
		background: transparent;
		border: 1px solid var(--color-accent);
		color: var(--color-text);
		border-radius: var(--radius-sm);
		padding: var(--space-1) var(--space-3);
		font-size: 11px;
		text-decoration: none;
	}
	.ghost {
		background: none;
		border: none;
		color: var(--color-accent-light);
		font-size: 11px;
	}
	.ghost:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}
</style>
```

- [ ] **Step 2: Update `Nav.svelte`**

Change the `navItems` array's Alerts entry from:
```ts
		{ label: 'Alerts', href: '/alerts' as Pathname },
```
to:
```ts
		{ label: 'Alerts', href: '/alerts' },
```

- [ ] **Step 3: Verify it builds**

Run: `pnpm check && pnpm build`
Expected: both succeed — `pnpm check` confirms `/alerts` now resolves as a real `Pathname`
without the cast.

- [ ] **Step 4: Run the full test suite**

Run: `pnpm vitest run`
Expected: all existing tests still pass (no test exercises `TriageCard`/`Nav` directly,
per the presentational-component exemption, but this confirms nothing else broke).

- [ ] **Step 5: Commit**

```bash
git add src/lib/components/TriageCard.svelte src/lib/components/Nav.svelte
git commit -m "Wire Wall's TriageCard to the real Alerts screen and mute endpoint"
```

---

### Task 9: Final verification and README update

**Worktree:** `siem-web-console/siem-web` (Step 1 also touches the sibling
`siem-api-implementation/siem-api` worktree — see Step 2)

**Files:**
- Modify: `siem-web/README.md`

No new application code — run everything together, refine Task 7's visual fidelity against
the design mockup, and document what shipped.

- [ ] **Step 1: Run the full siem-web suite**

From `siem-web-console/siem-web`:
```bash
pnpm check && pnpm test:unit run && pnpm build && pnpm run lint
```
Expected: all clean — every test from Tasks 3–8 passing together, 0 typecheck errors,
clean build, clean lint.

- [ ] **Step 2: Run the full siem-api suite**

From `siem-api-implementation/siem-api`:
```bash
go build ./... && go vet ./... && go test ./...
```
Expected: clean — confirms Tasks 1–2 didn't regress anything else in siem-api (this should
already be true from Task 2's own verification, but re-confirm at the end of this plan).

- [ ] **Step 3: Visual smoke test (best-effort, same constraint as the Wall sub-project)**

If a real Pocket ID OIDC client and a browser are available (per `siem-web/README.md`'s
Known Gaps — as of the Wall sub-project, neither was), run `pnpm dev` with `siem-api`
running locally, log in, navigate to `/alerts`, and compare the rendered inbox/detail
against `design_handoff_homesiem/designs/homeSIEM Console - Build.dc.html`'s Alerts screen.
Fix small, obvious visual discrepancies (spacing, color) directly; note anything larger in
the report rather than gold-plating beyond the handoff's spec. If no real login is possible
in this environment (the same blocker documented in the Wall sub-project's README), skip
this step and say so plainly — do not fabricate a visual check that didn't happen.

- [ ] **Step 4: Update `siem-web/README.md`**

In the "What's built so far" section, change:
```markdown
OIDC login, session cookie, global nav chrome, Screen 1 (Wall). The other
five screens (Search, Live tail, Alerts, Sources, Settings) are separate
future sub-projects.
```
to:
```markdown
OIDC login, session cookie, global nav chrome, Screen 1 (Wall), Screen 4
(Alerts — inbox, detail, read-only Rules tab). Search, Live tail, Sources,
and Settings are separate future sub-projects.
```

In the "Known gaps in this pass" section, add three bullets after the existing four:
```markdown
- The Alerts screen's Rules tab is read-only — no create/edit/delete/enable-toggle UI yet;
  that's a future sub-project.
- "Block at gateway" on the Alerts detail panel is a disabled button — SOAR-style automated
  response is out of scope for v1.
- The "reputation" stat on the Alerts detail panel is a static placeholder — nothing in the
  pipeline populates real threat-intel data yet.
```

- [ ] **Step 5: Commit**

```bash
git add siem-web/README.md
git commit -m "Update siem-web README for the Alerts screen sub-project"
```
