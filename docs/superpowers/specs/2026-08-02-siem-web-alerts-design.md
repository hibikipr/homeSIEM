# siem-web: Alerts screen — design

Status: approved
Scope: second sub-project of the `siem-web` service (`design_handoff_homesiem/README.md`,
Screen 4 — Alerts). Builds on the auth/BFF + shell + Wall sub-project (PR #2). Covers the
Alerts inbox/detail screen, a read-only Rules tab, and wiring Wall's previously-inert
TriageCard actions ("Investigate", "Mute 1h") to real behavior. Search, Live tail, Sources,
and Settings remain separate future sub-projects.

## Context

siem-api already exposes `GET /alerts`, `POST /alerts/{id}/ack`, and `GET /alerts/stream`
(SSE). It also stores up to 10 recent raw log samples per alert internally
(`alert_samples` table, `AddAlertSample`/`ListAlertSamples`), but nothing in the HTTP API
exposes them yet. The `alerts` table schema already reserves a `muted` state
(`state TEXT ... -- open|acked|muted|closed`), but no code path ever sets it — there is no
mute endpoint. Rule CRUD (`POST`/`PUT`/`DELETE /rules`, `GET /rules`) is fully built.

Wall (shipped in PR #2) already renders a `TriageCard.svelte` with "Investigate" and
"Mute 1h" buttons, but they're inert — no handlers were wired, since the Alerts screen
didn't exist yet. This sub-project wires them up.

## Goals

- Alerts inbox (Open / Acked / Rules tabs) + detail pane, per the handoff's Screen 4 layout.
- Two small siem-api additions: `GET /alerts/{id}/samples` (expose existing stored samples)
  and `POST /alerts/{id}/mute` (new: real mute lifecycle, not stubbed).
- Live inbox updates via a same-origin SSE proxy, reusing the pattern from Task 12
  (`/api/tail-proxy`).
- Wire Wall's `TriageCard` "Investigate" (→ `/alerts?id=X`) and "Mute 1h" (→ inline mute
  call, optimistic removal from the triage lane) to real behavior.
- Read-only Rules tab (name, shape, enabled, LogQL) — no rule create/edit/delete UI this
  pass.

Out of scope for this pass: full rule CRUD editor, "Block at gateway" as a real action
(SOAR-style automated response is explicitly out of scope for v1 per the design handoff),
Search/Live tail/Sources/Settings screens, real threat-intel/"reputation" data (nothing in
the pipeline populates it yet — `siem-ingest` doesn't exist).

## Backend additions to siem-api

Added to the still-open siem-api PR #1 (same precedent as `/events/stats` during the Wall
sub-project).

### `GET /alerts/{id}/samples`

- `viewer`+ role, matching `/alerts`'s own gating.
- Returns the existing `ListAlertSamples` result as JSON: `[{id, ts, line}, ...]`, newest
  first, capped at the 10 already retained per alert.
- 404 if the alert id doesn't exist.

### `POST /alerts/{id}/mute`

- `analyst`+ role, matching `/alerts/{id}/ack`.
- New `muted_until` column on `alerts` (nullable `TEXT`/timestamp, migration alongside the
  existing schema). Sets `state = 'muted'`, `muted_until = now + 1h`.
- Alert lifecycle (`internal/alerts/service.go`'s `Raise`): a `muted` alert with an
  unexpired `muted_until` is treated the same way an `open` alert under cooldown already
  is — `event_count` keeps accruing on the existing row, no new notification, no reopen.
  Once `muted_until` has passed, normal reopen logic applies (the existing
  `FindLatestAlert`/`ReopenAlert` path already generalized during siem-api's earlier
  ack/reopen fix handles this without further changes — a muted-and-expired alert is just
  another case of "existing alert found, route through reopen").
- Response: `204 No Content`, matching `/ack`'s own response shape.

## Frontend structure

```text
siem-web/src/
  routes/
    alerts/
      +page.server.ts       # load(): alerts (state from ?state=, default 'open'), samples
                             # for the selected ?id, rules list for the Rules tab
      +page.svelte           # inbox + detail split layout
    api/
      alerts-proxy/+server.ts   # same-origin SSE proxy for GET /alerts/stream (mirrors
                                 # Task 12's tail-proxy pattern exactly)
      alerts/[id]/mute/+server.ts  # thin POST passthrough to siem-api's new mute endpoint,
                                    # used by both the Alerts detail page and Wall's
                                    # TriageCard
  lib/
    components/
      AlertInbox.svelte      # left pane: Open/Acked/Rules segmented control + row list
      AlertRow.svelte        # one inbox row
      AlertDetail.svelte     # right pane: header, actions, stat cards, matched-events
                              # block, "rule that fired" panel
      RuleRow.svelte         # read-only row for the Rules tab
      TriageCard.svelte      # MODIFY (existing, from Wall) — wire Investigate/Mute 1h
    alerts.ts                # data-shaping helpers: port/source-IP extraction from
                              # sample JSON (mirrors wall.ts's deriveCountryBreakdown
                              # pattern), stat aggregation for the four stat cards
```

## Routing

`/alerts?state=open|acked&id=123` — `state` picks the inbox tab (default `open`), `id`
picks the selected/detail alert. Landing on `/alerts` with no `id` selects the first row
in the current tab. The Rules tab is a third `state` value in the same query param
(`?state=rules`) rather than a separate route, matching the mockup's single-screen
inbox+detail layout.

## Wall integration

- `TriageCard.svelte`'s "Investigate" button becomes `<a href="/alerts?id={alert.id}">`.
- "Mute 1h" becomes a button that `POST`s to `siem-web`'s own
  `/api/alerts/{id}/mute` passthrough route (not siem-api directly — same "browser never
  holds the bearer token" boundary as everything else), then optimistically removes the
  card from Wall's triage lane on success.

## Live updates

`AlertInbox.svelte` subscribes to the new `/api/alerts-proxy` SSE endpoint on mount (same
`$effect`/`EventSource`/cleanup-on-unmount pattern as `Ticker.svelte`), prepending new
alerts and patching state changes (ack/mute) into the current list as events arrive — no
polling.

## Rules tab

Read-only for this pass: `GET /rules` (already built, `viewer`+) — name, shape
(threshold/first-seen/absence), enabled, LogQL. No create/edit/delete/enable-toggle UI.
Selecting a rule row shows its LogQL and destinations in the detail pane, matching the
handoff's "Rule that fired" panel styling, but as a browse-only view. Full rule management
becomes its own future sub-project.

## Testing

Same split as the Wall sub-project:
- TDD (Vitest): `alerts.ts`'s data-shaping helpers (port/source-IP extraction, stat
  aggregation), the mute/ack passthrough routes, the SSE proxy route's auth-forwarding
  logic (mirrors Task 12's tail-proxy test).
- No unit tests for presentational Svelte components.
- No new Playwright e2e — Task 13's login test already covers the auth boundary; this pass
  doesn't add a second login-dependent flow test.

## Known gaps for this pass

- **"Reputation" stat card** renders as a placeholder ("unknown") — nothing in the
  pipeline populates a `threat_intel` field yet, since `siem-ingest` doesn't exist.
- **"Block at gateway"** ships as a disabled button with a tooltip explaining it's out of
  scope for v1 (SOAR-style automated response, per the design handoff's explicit
  out-of-scope list) — not omitted, so the mockup's layout stays intact without faking
  functionality.
- **Rules tab is read-only** — full CRUD is a future sub-project.
- Carries forward all of Wall's already-documented gaps (never-exercised real login,
  nav chrome's hardcoded alert count/ingest rate, no production adapter, etc.) — this pass
  doesn't resolve those; see `siem-web/README.md`.

## Decisions carried from brainstorming

- Rules tab: read-only list, not full CRUD, for this pass.
- Mute: built for real (new siem-api endpoint + lifecycle change), not stubbed. Block at
  gateway: stubbed as a disabled button, not omitted, not built for real.
- Alert detail data (matched events, ports, source IP): new `GET /alerts/{id}/samples`
  endpoint added to siem-api, client-side derivation in `alerts.ts`, following the same
  precedent as `/events/stats` and Wall's `deriveCountryBreakdown`.
- Routing: query-param based (`/alerts?state=...&id=...`), not path-based per-alert routes.
- Wall's "Mute 1h" mutes inline from the triage card (optimistic UI), not by navigating to
  the Alerts screen first.
- Alerts inbox live-updates via SSE, reusing Task 12's proxy pattern, rather than
  load-once-per-visit.
