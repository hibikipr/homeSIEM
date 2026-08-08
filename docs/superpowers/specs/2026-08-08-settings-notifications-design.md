# Settings → Notifications design

Status: approved
Scope: builds out the previously-stubbed "Notifications" section of Settings
(currently a placeholder, same as Retention/Parsers/Backups/About), wiring it
to real ntfy status/testing and a persisted minimum-severity notification
filter. Fixes a related dead-code gap in the existing ntfy priority mapping.

## Context

`siem-api`'s ntfy integration (`internal/ntfy/client.go`) is fully env-var
configured (`NTFY_URL`/`NTFY_TOPIC`/`NTFY_TOKEN`) and every new/reopened
alert triggers an unconditional push (`alerts/service.go`'s `Raise()`), with
no severity filtering and no way to verify the connection short of watching
for a real alert to fire. `severityToPriority` maps `"critical"→"urgent"`,
`"high"→"high"`, `"medium"→"default"`, default→`"low"` — but rule severity
only ever takes the values `critical`/`warning`/`info` (the real vocabulary,
per `RuleFromEventForm.svelte`'s `<select>`), so the `"high"`/`"medium"`
cases are dead, and both `warning` and `info` alerts silently collapse to the
same `"low"` priority today.

**Migration gotcha discovered during design:** `store.Migrate()` only applies
`schema.sql` when the `sources` table doesn't exist yet — on any
already-deployed database (including production), it's a no-op. Simply
adding a new table to `schema.sql` would never create it on an existing
install. This needs a small always-run migration step, independent of the
one-time bootstrap, which also serves the app's other still-stubbed Settings
sections whenever they're built.

## Goals

- Settings → Notifications shows whether ntfy is configured
  (`NTFY_URL`/`NTFY_TOPIC` non-empty) and a "Send test notification" button
  that fires a real push through the existing `ntfy.Client`.
- A minimum-severity filter (`info`/`warning`/`critical`, default `info` —
  today's "notify on everything" behavior), persisted in the database and
  editable from the UI, gates the ntfy push only. Alerts still always appear
  in-app (Wall/Alerts screen SSE stream) regardless of this filter — it only
  controls whether an admin gets paged, not what's visible in the app.
- ntfy's connection settings (URL/topic/token) stay env-var-only, matching
  the precedent already set for OIDC — no UI editing, no database storage of
  deployment secrets.
- Fix `severityToPriority`'s dead cases: `critical`→`urgent`,
  `warning`→`default`, `info`→`low`.

## Non-goals (this pass)

- Per-rule or per-user notification preferences — one global minimum
  severity, same as there's currently one global "always notify."
- Any change to which alerts appear in-app — only the ntfy push is filtered.
- Editing ntfy's URL/topic/token via the UI (see Goals — explicitly
  env-var-only, matching OIDC's precedent).
- Building out the other stubbed Settings sections (Retention, Parsers,
  Backups, About) — only Notifications, this pass.

## Design

### Migration mechanism (new, small, general-purpose)

`store.Migrate()` currently does one thing: apply the full `schema.sql` if
`sources` doesn't exist. Add a second, always-run step after that check —
a small set of individually-idempotent `CREATE TABLE IF NOT EXISTS`
statements for tables introduced after the initial release, executed on
every call regardless of whether the one-time bootstrap ran. This pass adds
exactly one: `notification_settings`. Future Settings sections needing new
tables extend this same always-run step rather than editing `schema.sql`
(which stays the fresh-install-only bootstrap).

### Database

New table, added via the always-run migration step above (not `schema.sql`):

```sql
CREATE TABLE IF NOT EXISTS notification_settings (
  id           INTEGER PRIMARY KEY CHECK (id = 1),  -- single row, enforced
  min_severity TEXT NOT NULL DEFAULT 'info'
);
INSERT OR IGNORE INTO notification_settings (id, min_severity) VALUES (1, 'info');
```

`internal/store/notifications.go` (new file, matching the codebase's
one-concept-per-file convention rather than `role_mappings`' historical home
in `users.go`):

```go
func (s *Store) GetMinNotifySeverity(ctx context.Context) (string, error) { ... }
func (s *Store) SetMinNotifySeverity(ctx context.Context, severity string) error { ... }
```

### `alerts` package

- New shared helper (same file as `severityToPriority` in
  `internal/alerts/service.go`): `severityRank(severity string) int` —
  `info`→0, `warning`→1, `critical`→2, anything unrecognized→0 (matches the
  existing "unrecognized falls to the lowest tier" convention
  `severityToPriority`'s `default` case already uses).
- `AlertStore` interface gains `GetMinNotifySeverity(ctx context.Context) (string, error)`.
- `Raise()`: immediately before the `s.notifier.Publish(...)` call, fetch
  the stored minimum severity and skip the push (but keep everything else —
  DB insert, SSE hub publish — unchanged) when
  `severityRank(c.Severity) < severityRank(minSeverity)`. A store-read
  failure here logs and falls back to notifying (fail open — matches this
  codebase's existing "never let a defensive check drop a real alert"
  posture, e.g. `queryVolumeBuckets`'s error handling).
- `severityToPriority` fixed to `critical`→`urgent`, `warning`→`default`,
  `info`→`low` (drops the dead `high`/`medium` cases).

### `siem-api` HTTP surface

New file `internal/api/settings_notifications.go`, mirroring
`settings_auth.go`'s shape:

- `GET /settings/notifications` (admin-only, same `protect(...)` wrapper as
  `/settings/auth`): returns
  `{"ntfy_configured": bool, "min_severity": string}`. `ntfy_configured` is
  `true` when both `Deps`' `NtfyURL` and `NtfyTopic` config values are
  non-empty (matching `ntfy.Client`'s own requirements — a token is
  optional, URL+topic are not).
- `PUT /settings/notifications`: body `{"min_severity": string}`; validates
  it's one of `info`/`warning`/`critical` (400 otherwise); persists via
  `SetMinNotifySeverity`; 204 on success.
- `POST /settings/notifications/test`: fires a real push via the existing
  `ntfy.Client` (`Deps` gains an `Ntfy *ntfy.Client` field, threaded from
  the same `ntfyClient` `main.go` already constructs for `alerts.NewService`
  — no new client instance). Returns 400 if not configured
  (`ntfy_configured` false), 502 if the publish call itself fails, 200
  `{"ok": true}` on success. Title/message are fixed, descriptive strings
  (e.g. title `"homeSIEM test notification"`, body naming the admin who
  triggered it and a timestamp) — not user-editable input.

### `siem-web`

**`lib/server/siemApiClient.ts`** — new interface and three new client
methods, mirroring `getAuthSettings`/`updateRoleMappings` exactly:

```ts
export interface NotificationSettingsResponse {
	ntfy_configured: boolean;
	min_severity: string;
}

async getNotificationSettings(sessionToken: string): Promise<NotificationSettingsResponse>
async updateNotificationSettings(sessionToken: string, minSeverity: string): Promise<void>
async testNotification(sessionToken: string): Promise<void>
```

**`routes/api/settings/notifications/+server.ts`** (new, same-origin proxy,
mirrors `routes/api/settings/auth/+server.ts`): `GET` and `PUT`.

**`routes/api/settings/notifications/test/+server.ts`** (new): `POST`, same
proxy shape, surfaces the siem-api error message/status on failure.

**`routes/settings/+page.server.ts`**: load function also calls
`client.getNotificationSettings(token)` alongside the existing
`getAuthSettings` call, returns `notificationSettings` in the page data
(same 401/403/502 handling already in place for the existing call).

**`routes/settings/+page.svelte`**: the Notifications section (currently the
generic placeholder branch) becomes its own real branch: shows a status line
("ntfy is configured" / "ntfy is not configured — set NTFY_URL and
NTFY_TOPIC to enable"), a minimum-severity `<select>` (info/warning/critical)
that PUTs on change and shows a save confirmation/error inline, and a "Send
test notification" button that POSTs and shows a success/failure message
inline (disabled while ntfy isn't configured).

## Testing

- `internal/store`: round-trip test for `GetMinNotifySeverity`/
  `SetMinNotifySeverity`, and a `Migrate()` test confirming
  `notification_settings` gets created even when called against a database
  that already has the `sources` table (simulating an existing deployment) —
  this is the test that would have caught the migration gotcha.
- `internal/alerts`: unit tests for `severityRank` and the fixed
  `severityToPriority`; a `Raise()` test confirming a below-threshold
  severity skips the ntfy publish but still inserts the alert and publishes
  to the SSE hub; a test confirming a store-read failure for the threshold
  falls back to notifying (fail open).
- `internal/api`: handler tests for `GET`/`PUT /settings/notifications`
  (including the 400 on an invalid `min_severity` value) and
  `POST /settings/notifications/test` (success, not-configured, publish
  failure), matching `settings_auth_test.go`'s existing patterns.
- `siem-web`: `siemApiClient.test.ts` additions for the three new client
  methods; `+page.server.ts` test additions for the new load-time call.
  Manual browser verification for the new Settings UI branch (per this
  codebase's established no-component-test-infrastructure constraint — see
  the Nav-avatar-picture plan's Global Constraints for the precedent).

## Known gaps after this pass

- One global minimum-severity filter, not per-rule/per-user — see Non-goals.
- The other stubbed Settings sections (Retention, Parsers, Backups, About)
  remain stubs.
