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

- `pnpm test:unit` — Vitest, TDD coverage for session/cookie handling, the
  siem-api client, claims extraction, the auth gate, and Wall's data-shaping
  helpers.
- `pnpm exec playwright test` — the login-flow e2e test (see its own file
  for what is/isn't automated, depending on Pocket ID's WebAuthn testability).

## What's built

OIDC login, session cookie, global nav chrome (active-route highlighting,
account menu with avatar, live ingest-rate/open-alert-count summary), and
all six screens:

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
- **Alerts** — inbox, detail panel (acknowledge/mute, matched-event stats),
  and a Rules tab that supports creating rules and toggling
  enabled/disabled (no edit or delete yet — see known gaps).
- **Sources** — claimed/unclaimed sources table, parser preview, ingest-health
  panel, claim flow.
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

- Wall's country breakdown is a best-effort client-side derivation from a
  bounded `/events/search` call, not a real aggregation endpoint.
- Break-glass local admin login has no UI — the session/auth layer supports
  it (`SIEM_LOCAL_ADMIN_USERNAME`/`SIEM_LOCAL_ADMIN_PASSWORD_HASH` on
  `siem-api`), but there's no login form in `siem-web` that uses it; OIDC is
  the only path in through the UI today.
- The Alerts screen's Rules tab supports create and enable/disable, but not
  edit or delete — those would need their own follow-up.
- "Block at gateway" on the Alerts detail panel is a disabled button — SOAR-style automated
  response is out of scope for v1.
- The "reputation" stat on the Alerts detail panel is a static placeholder — nothing in the
  pipeline populates real threat-intel data into it yet (threat-intel matching happens at
  the `siem-ingest` fast-path layer, not surfaced back into this stat).
- The Alerts screen's "distinct ports"/"source IP" stat cards depend on log lines carrying
  structured `src_ip`/`dst_port` JSON fields (`dst_port` as a JSON number) — only some
  parsed formats (netfilter-style, CEF) populate these; free-text log lines don't.
- Acknowledge/Mute buttons are shown to every role, not just `analyst`+/`admin` — siem-api
  correctly rejects the request either way, but a `viewer` clicking either button sees
  an inline "failed" message rather than the button being hidden/disabled up front.
- Muting an alert removes it from every list (Wall's triage lane, the Alerts inbox) for the
  full mute window with no "Muted" tab or countdown — matches the design's intent for
  Wall's triage lane, but is an easy-to-miss side effect from the Alerts detail pane.
- Ack/mute changes made by one analyst aren't pushed live to other open browser sessions —
  only new alerts raised by the rule engine publish over SSE; a second person's ack/mute
  only becomes visible to you on your next own action or reload.
- The Sources screen's parser preview only shows the single most recent sample
  (`limit=1`), not a scrollable history.
- There's no "dropped UDP" metric on the Sources screen — Vector doesn't expose one,
  and component-error counts aren't queryable over one-shot HTTP (only Subscription is),
  so this isn't currently obtainable at all.
- The ingest-health panel will show as degraded unless `siem-ingest` is actually
  running — it's an optional/profiled service in the deployment compose file, not
  started by default.
- No Svelte component test framework — UI-only changes are verified via
  `svelte-check`/lint/manual or Playwright interaction, not automated
  component-behavior tests. `pnpm test:unit`'s Vitest coverage is for
  non-component logic (session/cookie handling, the siem-api client, claims
  extraction, data-shaping helpers) — see Testing above.
