# siem-web: auth/BFF + shell + Wall screen — design

Status: approved
Scope: first sub-project of the homeSIEM handoff's `siem-web` service
(`design_handoff_homesiem/README.md`, Part 1). Covers OIDC login through
Pocket ID, the session/BFF layer, the global nav chrome, and Screen 1
(Wall — the default landing screen). The remaining five screens (Search,
Live tail, Alerts, Sources, Settings) are separate future sub-projects.

## Context

`siem-api` (the backend) is fully implemented — 31 tasks, TDD throughout,
full test suite green — and is on an open PR (`hibikipr/homeSIEM#1`), not
yet merged. `siem-web` is the second of the three planned homeSIEM
services; `siem-ingest`/deployment is the third, still to come.

The handoff's design fidelity is "high — colors, type, spacing and layout
are final, recreate faithfully." This is not a creative/visual-exploration
brainstorm; the design tokens and screen layouts are already fully
specified in `design_handoff_homesiem/README.md`. This spec is about
implementation strategy: framework, architecture, and how the BFF's
contract with the now-real `siem-api` actually works.

## Goals

- OIDC login against Pocket ID (authorization code + PKCE), matching the
  pattern `cronmaster` already uses against the same issuer.
- A `siem-web`-minted internal JWT session cookie that `siem-api` accepts
  directly — no separate session store.
- The global nav chrome (header, nav links, ingest indicator, avatar) that
  every future screen mounts into.
- Screen 1 (Wall): stat row, source×hour heat grid, triage lane (top 3 open
  alerts), country breakdown, live ticker.
- One new `siem-api` endpoint (`GET /events/stats`, already built and
  pushed to the open PR during this brainstorm) providing the aggregated
  numbers Wall needs that `/events/search` can't efficiently provide.

Out of scope for this pass: Search, Live tail, Alerts, Sources, and
Settings screens; the break-glass local-admin login path; any
`siem-ingest`/deployment work.

## Stack

- **SvelteKit** — one app, one deploy. Svelte components for the UI;
  SvelteKit server routes/hooks for the BFF (OIDC callback, session
  cookie, proxying to siem-api). Chosen over Next.js/React for a smaller
  client bundle on a dense, table-heavy console, and because SvelteKit's
  server routes are a direct match for "thin BFF."
- **pnpm** as the package manager.
- **`openid-client`** for the OIDC/PKCE flow (discovery, code exchange,
  ID token verification against Pocket ID's JWKS).
- **`jose`** for minting/verifying the internal session JWT (HS256,
  `SIEM_SESSION_SECRET` shared with siem-api) — chosen over
  `jsonwebtoken` for native ESM support matching SvelteKit's tooling.
- **Vitest + `@testing-library/svelte`** for TDD on logic-bearing code.
- **Playwright** for one end-to-end test covering the full login flow.
- Styling: plain CSS custom properties (`tokens.css`) + Svelte scoped
  `<style>` blocks per component — no CSS framework. The Nocturne design
  system's 0.7× spacing scale and OKLCH severity colors are custom enough
  that a framework's default theme would need near-total replacement
  anyway; native CSS custom properties plus Svelte's own idioms is more
  direct.

## Dev/test environment

Real Pocket ID (`https://pocketid.townsville.cc`) for the OIDC flow. Real
`siem-api` binary run locally (from the `siem-api-implementation` worktree)
as the BFF's proxy target — it's fully built and tested even though not
yet merged/deployed.

## Project structure

```text
siem-web/
  src/
    routes/
      +layout.svelte           # global chrome: header, nav, ingest indicator, avatar
      +layout.server.ts        # loads session user for the layout
      +page.svelte             # Wall screen (Screen 1)
      +page.server.ts          # Wall's data load
      auth/
        login/+server.ts       # redirects to Pocket ID's /authorize
        callback/+server.ts    # exchanges code, verifies ID token, calls siem-api, mints session cookie
        logout/+server.ts      # clears cookie, redirects to Pocket ID's end-session endpoint
    lib/
      server/
        oidc.ts                # openid-client wrapper: discovery, PKCE, code exchange
        session.ts              # cookie read/write, internal JWT mint/verify
        siemApiClient.ts        # fetch wrapper: attaches Authorization: Bearer <session JWT>
      components/
        Nav.svelte, StatRow.svelte, HeatGrid.svelte, TriageCard.svelte, Ticker.svelte, CountryBar.svelte
      styles/
        tokens.css               # Nocturne design tokens as CSS custom properties
    hooks.server.ts              # reads session cookie, attaches user to event.locals, redirects unauthenticated requests
  static/
  package.json
```

## Auth flow

1. `GET /auth/login` — builds the Pocket ID `/authorize` URL via
   `openid-client` (PKCE verifier/challenge in a short-lived cookie),
   redirects the browser.
2. `GET /auth/callback` — exchanges the code (with PKCE verifier) via
   `openid-client`, verifies the ID token against Pocket ID's JWKS. This is
   the **only** place siem-web does real OIDC/JWKS work — matching
   siem-api's design assumption that siem-api itself never talks
   OIDC/JWKS. Extracts `sub`, `email`, `name`, `groups` from the verified
   ID token.
3. Calls `POST {API_URL}/auth/session` on siem-api with those claims
   (server-to-server, over the internal Docker network). siem-api resolves
   the role via `role_mappings`, upserts the user, and returns
   `{user_id, role, display_name}`, denying (403) if the group is
   unmapped.
4. siem-web mints its own internal JWT (HS256, `jose`, signed with
   `SIEM_SESSION_SECRET` — the same secret siem-api's `TokenVerifier`
   checks) carrying `{sub, user_id, email, display_name, groups}`, matching
   the exact claim shape siem-api's auth package expects. Sets it as the
   session cookie: `HttpOnly; Secure; SameSite=Lax`, 12-hour expiry per the
   handoff's "Session lifetime 12 hours" setting.
5. Redirects to `/` (Wall).
6. `hooks.server.ts` runs on every request: reads the cookie, verifies it
   locally (same `jose` verify, no network call), attaches
   `{userID, role, displayName}` to `event.locals.user`. Missing/invalid
   cookie → redirect to `/auth/login`, except for `/auth/*` routes
   themselves.
7. `siemApiClient.ts` — every server-side call to siem-api attaches
   `Authorization: Bearer <the same cookie JWT>`, since siem-api's own
   middleware re-verifies that exact token independently on every request.
   This is the "no token ever reaches JavaScript" boundary: the browser
   only ever holds the `HttpOnly` cookie.
8. `GET /auth/logout` — clears the cookie, redirects to Pocket ID's
   `/api/oidc/end-session` directly (bypassing discovery, per the
   handoff).

Break-glass local admin (`POST siem-api`'s `/auth/local`) is explicitly out
of scope here — it belongs with the Settings screen sub-project, where the
"local admin enabled" toggle lives.

## SSE proxying (ticker)

The Wall screen's ticker needs a live stream, but the JWT can't reach the
browser for the `EventSource` to authenticate directly against siem-api.
Pattern: a same-origin SvelteKit route proxies siem-api's `GET
/events/tail` — the browser's `EventSource` hits siem-web's own origin
(cookie sent automatically, standard same-origin browser behavior),
siem-web's server forwards the `Authorization` header to siem-api
server-side and re-streams the response. The same pattern will be reused
by the Alerts screen's live stream in a later sub-project.

## `GET /events/stats` (new siem-api endpoint, already built)

Added directly to the open siem-api PR during this brainstorm, since
`/events/search` only returns raw log lines (capped by `limit`) with no
aggregation — insufficient for a "1.24M events/24h" stat or a
source×hour heat grid.

- New `loki.Client.QueryMatrix` — parses Loki's Prometheus-style matrix
  response shape (`resultType: matrix`), distinct from `QueryRange`'s
  log-stream parsing. Timestamps are Unix seconds (possibly fractional),
  not the nanosecond-string format streams use.
- Four LogQL metric queries: 24h grand total
  (`sum(count_over_time({job="siem"}[24h]))`), and three per-source hourly
  breakdowns over the 24h window at `step=1h` — critical count
  (`severity="critical"`), warning count (`severity="warning"`), and total
  volume (no severity filter).
- Per-`(source, hour)` cell classification: critical count>0 → `critical`;
  else warning count>0 → `warning`; else total==0 → `none`; else
  total≥50 → `busy`; total≥5 → `light`; else → `quiet`. Matches the
  handoff's six-tier heat-grid legend. Thresholds (50/5) are a first pass.
- Response: `{event_count_24h: int64, heat_grid: [{source: string, hours:
  [24 tier strings]}]}`.
- `viewer`+ role, same auth pattern as `/events/search`.

## Wall screen data flow

`+page.server.ts`'s `load` makes parallel calls via `siemApiClient`:

- **Stat row + heat grid**: one call to `GET /events/stats`.
- **Open alerts count + triage lane**: `GET /alerts?state=open`, sorted by
  severity then recency client-side (siem-api doesn't sort by severity),
  top 3 for the triage lane.
- **Country breakdown**: no `geoip.cc` aggregation endpoint exists.
  Best-effort for this pass: query `/events/search` with a bounded window
  and derive a country count from returned entries' parsed JSON
  (`geoip.cc` field, when present). Acceptable since this is inherently a
  low-cardinality panel; if it proves inaccurate in practice it can get its
  own aggregation endpoint later, the same way `/events/stats` did.
- **Retention figures** ("31d hot · 90d cold", "18.4 GB / 60 GB"): nothing
  in siem-api or the reference config exposes storage/retention metrics —
  this is Loki/infrastructure-level data, entirely out of siem-api's
  scope. Rendered as static text sourced from environment config
  (`RETENTION_HOT_DAYS`, `RETENTION_COLD_DAYS`) for the "31d hot · 90d
  cold" line; the storage-usage line ("18.4 GB / 60 GB") is hardcoded
  placeholder text for this pass, since no config or endpoint provides it
  — flagged as a known gap for a later Loki-metrics integration, not a
  blocker.
- **Ticker**: SSE via the same-origin proxy described above.

## Design tokens & components

`src/lib/styles/tokens.css` — one file, CSS custom properties for every
value in the handoff's Design Tokens section: color (`--bg`, `--surface`,
`--accent`, severity OKLCH values, etc.), the 0.7× spacing scale as
`--space-1` through `--space-6` (2.8/5.6/8.4/11.2/16.8/22.4px), radius,
elevation (two composable box-shadow custom properties, flat/raised — "do
not stack shadows" per the handoff), and the type scale. Every component
references these — never a raw hex or px value in component styles.

Wall components (`src/lib/components/`): `Nav.svelte` (in `+layout.svelte`),
`StatRow.svelte`, `HeatGrid.svelte`, `TriageCard.svelte`, `CountryBar.svelte`,
`Ticker.svelte`. Each is presentational (props in, markup out) — no
component fetches its own data; `load`'s return value is the single
source of truth, except the ticker, which is client-side reactive via the
`EventSource` proxy.

## Testing

- TDD (Vitest): `session.ts` (JWT mint/verify round-trip, cookie
  attributes), `oidc.ts` (PKCE flow logic, mocked against `openid-client`),
  `hooks.server.ts` (redirect-when-unauthenticated logic), the heat-grid
  tier→color mapping and country-count derivation helpers.
- Playwright e2e: one test driving the full login flow against real Pocket
  ID + real local siem-api — login → land on Wall → nav chrome and stat
  row populated.
- No unit tests for presentational Svelte components — fidelity to the
  mockup is checked visually against the `.dc.html` reference, not
  asserted in test code.

## Decisions carried from brainstorming

- Build order within siem-web: auth/BFF + shell + Wall first, then the
  remaining five screens as later sub-projects.
- Dev/test against real Pocket ID + a real local siem-api binary, not
  mocks.
- Styling via plain CSS custom properties, not Tailwind.
- TDD for logic, e2e for login; no component-level unit tests.
- `GET /events/stats` was added to the still-open siem-api PR rather than
  approximating aggregation client-side or stubbing the Wall screen's
  stat/heat-grid panels — keeps the BFF thin and matches how the design
  intends this to scale.
