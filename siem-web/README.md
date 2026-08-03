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

OIDC login, session cookie, global nav chrome, Screen 1 (Wall), Screen 4
(Alerts — inbox, detail, read-only Rules tab). Search, Live tail, Sources,
and Settings are separate future sub-projects.

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
- The Alerts screen's Rules tab is read-only — no create/edit/delete/enable-toggle UI yet;
  that's a future sub-project.
- "Block at gateway" on the Alerts detail panel is a disabled button — SOAR-style automated
  response is out of scope for v1.
- The "reputation" stat on the Alerts detail panel is a static placeholder — nothing in the
  pipeline populates real threat-intel data yet.
- The Alerts screen's "distinct ports"/"source IP" stat cards depend on log lines carrying
  structured `src_ip`/`dst_port` JSON fields (`dst_port` as a JSON number) — nothing in the
  pipeline populates these yet, same class of gap as Wall's country breakdown.
- Acknowledge/Mute buttons are shown to every role, not just `analyst`+/`admin` — siem-api
  correctly rejects the request either way, but a `viewer` clicking either button now sees
  an inline "failed" message rather than the button being hidden/disabled up front. Proper
  role-gating needs `user.role` plumbed through both the Wall and Alerts screens' loads —
  deferred as its own small follow-up, not done in this pass.
- Muting an alert removes it from every list (Wall's triage lane, the Alerts inbox) for the
  full mute window with no "Muted" tab or countdown — this matches the design's intent for
  Wall's triage lane, but is an easy-to-miss side effect from the Alerts detail pane.
- Ack/mute changes made by one analyst aren't pushed live to other open browser sessions —
  only new alerts raised by the rule engine publish over SSE; a second person's ack/mute
  only becomes visible to you on your next own action or reload.
