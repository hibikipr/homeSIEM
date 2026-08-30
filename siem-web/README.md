# siem-web

The homeSIEM console: OIDC login through Pocket ID, a session/BFF layer, and
six screens — Wall, Search, Live tail, Alerts, Sources, Settings. See
`docs/superpowers/specs/` for the full design history, one spec per
screen/feature pass (auth shell + Wall, Alerts, Sources, Live tail, Search,
Settings, nav account menu, Wall dashboard rebuild, known-gaps/app-icons
closeout, and others).

## Running locally

1. Copy `.env.example` to `.env` and fill in the values — `SESSION_SECRET`
   must match whatever `siem-api` is using for `SIEM_SESSION_SECRET`.
2. Run `siem-api` locally (see the `siem-api-implementation` worktree's
   own README/smoke-test instructions) so `API_URL` has something to talk to.
3. `pnpm install`
4. `pnpm dev`

## Testing

- `pnpm test:unit` — Vitest, split into two projects (`vite.config.ts`):
  - `server` (Node environment) — session/cookie handling, the siem-api
    client, claims extraction, the auth gate, Wall's data-shaping helpers.
  - `client` (jsdom + `@testing-library/svelte`) — component-behavior tests,
    e.g. `RuleDetail.svelte.test.ts`. Name test files `*.svelte.test.ts` so
    they land in this project instead of `server`'s. Needs
    `resolve: { conditions: ['browser'] }` on this project specifically —
    without it Vite resolves Svelte's server-only build even under jsdom.
    This project's own `afterEach` calls `@testing-library/svelte`'s
    `cleanup()` explicitly; its usual auto-cleanup only self-registers when
    `afterEach` is a vitest global, which this repo doesn't enable.
  - Run one project at a time with `pnpm test:unit -- --project client` (or
    `server`).
- `pnpm exec playwright test` — the login-flow e2e test (see its own file
  for what is/isn't automated, depending on Pocket ID's WebAuthn testability).

## What's built

OIDC login, a break-glass local-admin login (`/auth/local-login`, not linked
from the normal sign-in page — see the root [.env.example](../.env.example)
for `SIEM_LOCAL_ADMIN_USERNAME`/`SIEM_LOCAL_ADMIN_PASSWORD_HASH`), session
cookie, global nav chrome (active-route highlighting, account menu with
avatar, live ingest-rate/open-alert-count summary), and all six screens:

- **Wall** — the landing dashboard: a volume ribbon, a severity/source heat
  grid, a country breakdown bar, a live ticker, and a triage lane for open
  alerts. Rebuilt from its original v1 layout in the
  2026-08-08-wall-dashboard-rebuild pass.
- **Search** — a query bar (LogQL-ish filters: severity, source, program,
  free text) with clearable fields, a facet rail (severity/program/**source**),
  a result table, an event inspector, and rule creation from either the
  current query ("New rule" on the query bar) or a single event ("New rule"
  on the event inspector) — both open the same `RuleFromEventForm` and
  navigate straight to the new rule in Alerts → Rules on success.
- **Live tail** — a streaming SSE viewport with severity filtering, pause/resume,
  and an auto-follow "N new" pill when you've scrolled away from the bottom.
  The empty-state message distinguishes "nothing has arrived yet," "paused
  with events waiting," and "the severity filter hides everything" as three
  separate cases.
- **Alerts** — inbox with Open/Acked/Muted/Rules tabs (a muted alert shows a
  countdown to when it reopens instead of dropping out of sight), detail panel
  (acknowledge/mute, matched-event stats), and a Rules tab that supports
  creating, editing, deleting (with a confirm prompt), and toggling
  enabled/disabled.
- **Sources** — claimed/unclaimed sources table, a parser preview with a
  scrollable recent-samples history (not just the single latest line),
  ingest-health panel, claim flow.
- **Settings** — Authentication (OIDC identity, group→role mapping table,
  break-glass local-admin note) and Notifications. Retention/Parsers/Backups/About
  are intentionally hidden from the sidebar — no backend support exists for
  them, so they aren't shipped as visible stubs.
- **Insights** — a compact panel on the Wall plus a full `/insights` history
  screen for siem-api's optional LLM-powered log review (severity-coded
  suggestions, dismiss, per-suggestion evidence, a "Generate now" button).
  Empty and entirely hidden-feeling when `OLLAMA_URL` isn't configured —
  the panel just shows "No insights yet," same as any other quiet-by-default
  screen in this app.

Also: a real favicon/PWA icon set and manifest (`static/icons/`,
`static/manifest.webmanifest`), a `<title>`, and a `theme-color` meta tag —
the app no longer ships SvelteKit's default scaffold favicon.

## Known gaps

- "Block at gateway" on the Alerts detail panel is a disabled button — SOAR-style automated
  response is out of scope for v1.
- The Alerts screen's "distinct ports"/"source IP" stat cards depend on log lines carrying
  structured `src_ip`/`dst_port` JSON fields (`dst_port` as a JSON number) — only some
  parsed formats (netfilter-style, CEF) populate these; free-text log lines don't.
- There's no "dropped UDP" metric on the Sources screen — Vector doesn't expose one,
  and component-error counts aren't queryable over one-shot HTTP (only Subscription is),
  so this isn't currently obtainable at all.
- The ingest-health panel will show as degraded unless `siem-ingest` is actually
  running — it's an optional/profiled service in the deployment compose file, not
  started by default.
