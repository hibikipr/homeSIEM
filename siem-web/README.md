# siem-web

The homeSIEM console: OIDC login through Pocket ID, session/BFF layer, and
(so far) the Wall screen. See `docs/superpowers/specs/2026-08-02-siem-web-auth-shell-wall-design.md`
for the design.

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

## What's built so far

OIDC login, session cookie, global nav chrome, Screen 1 (Wall). The other
five screens (Search, Live tail, Alerts, Sources, Settings) are separate
future sub-projects.

## Known gaps in this pass

- Nav chrome's alert count and ingest-rate figures are hardcoded to 0 — no
  shared layout-level data source for them yet.
- Wall's country breakdown is a best-effort client-side derivation from a
  bounded `/events/search` call, not a real aggregation endpoint.
- Wall's retention figures have no data source at all yet.
- Break-glass local admin login isn't wired up (belongs with the Settings
  screen sub-project).
- Full manual smoke test (login → Wall screen render → visual comparison
  against the design mockup) has never been performed in this project — it
  requires a real registered Pocket ID OIDC client (`OIDC_CLIENT_ID`) and an
  interactive browser with WebAuthn support, neither available in the
  automated development environment used to build this sub-project. Someone
  with Pocket ID admin access needs to register a client and run this by
  hand before shipping.
