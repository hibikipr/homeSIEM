# siem-web: Sources screen — design

Status: approved
Scope: third sub-project of the `siem-web` service (`design_handoff_homesiem/README.md`,
Screen 5 — Sources). Builds on the auth/BFF + shell + Wall sub-project (PR #2) and the
Alerts sub-project. Covers the Sources table, parser preview, ingest-health panel, and
unclaimed-senders/claim flow. Search, Live tail, and Settings remain separate future
sub-projects.

## Context

siem-api already has almost everything the store/API layer needs for this screen:
`store.Source` (`id, name, address, transport, parser, claimed, heartbeat_sec,
last_seen_at, created_at`), `ListSources`, `ClaimSource`, `StaleSources` (the "is this
source silent" query, currently used internally only), and the routes `GET /sources`
(`viewer`+) and `POST /sources/{id}/claim` (`admin`+). `siem-ingest`'s heartbeat sink
(`sinks.siem_heartbeat`) already populates this table for every source that has sent
traffic, throttled to at most one heartbeat per source per 15 minutes
(`heartbeat_throttle`, `window_secs = 900`) — unknown senders are never dropped, they show
up here unclaimed, per the design handoff's explicit requirement.

`siem-ingest`'s Vector process already runs its GraphQL API (`[api] enabled = true`,
`0.0.0.0:8686`, not published to the host — reachable only from other containers on the
stack's shared network). Its schema exposes `SourceMetrics` (received bytes/events),
`SinkMetrics` (received/sent events, sent bytes), `TransformMetrics` (received/sent
events), and a generic `ComponentErrorsTotal` — confirmed by introspecting the real schema
during brainstorming. It does **not** expose anything queue-depth or latency-percentile
shaped — no buffer-depth type exists anywhere in the schema.

## Goals

- Sources table per the handoff's Screen 5 layout: Source · Address · Transport · Parser
  (tag) · Events/min (mono, right-aligned) · Last seen · Health (colored dot + word).
- Parser preview: side-by-side raw line / extracted fields for a selected source, reusing
  `GET /events/search?source=X`.
- Ingest health panel, backed by Vector's GraphQL API (not a new Prometheus pipeline) —
  see "Known gaps" for how this pass adapts the mockup's four metrics to what Vector
  actually exposes.
- Unclaimed senders list with a Claim button (`admin`+), wired to the existing
  `POST /sources/{id}/claim`.
- "Point a device here" static instructional block (UniFi path + host/ports), copy only —
  no new data needed.

Out of scope for this pass: per-source `heartbeat_sec` editing (no UI exists to set it;
every heartbeat call still hardcodes the schema default — a pre-existing gap this pass
doesn't resolve), a CEF-aware parser for the UniFi SIEM-Server integration (documented gap
in `siem-ingest/README.md`, unrelated to this screen), Search/Live tail/Settings screens.

## Backend additions to siem-api

### `GET /sources` — response gains two fields

`sourceResponse` gains:
- `status: "healthy" | "silent"` — computed in the handler from the same rule
  `StaleSources` already uses (`last_seen_at IS NULL OR now - last_seen_at > heartbeat_sec`),
  reimplemented as a plain Go comparison against the already-fetched `LastSeenAt` and
  `HeartbeatSec` rather than a second SQL query.
- `events_per_min: float64` — one additional Loki query in the handler, reusing
  `queryHourlyBySource`'s shape:
  `sum by (source) (count_over_time({job="siem"}[5m])) / 5`, joined onto each source row
  by name. Sources with no matching series (never logged, or the query window is empty)
  get `0`.

No store-layer change — `StaleSources` stays as-is for its existing internal caller.

### `GET /sources/ingest-health` (new)

- `viewer`+ role, matching `GET /sources`.
- New `internal/vector` package: a small GraphQL client (`POST {VectorGraphQLURL}/graphql`)
  mirroring `internal/loki/client.go`'s shape (a `*http.Client`-backed struct with one
  `Query(ctx, query string) (json.RawMessage, error)` method), so the handler can be tested
  against a fake HTTP server the same way `stats_test.go` fakes Loki.
- One query fetching `sources { nodes { componentId metrics { receivedEventsTotal {
  receivedEventsTotal } } } }` and `sinks { nodes { componentId metrics { sentEventsTotal {
  sentEventsTotal } } } }` — schema verified against a real `timberio/vector:0.49.0-alpine`
  instance during planning (`sources`/`sinks` are Relay-style connections off the root
  `Query` type; `nodes` skips the `edges`/`cursor` wrapper since this pass doesn't paginate).
  **Component error counts are not included**: introspection found `componentErrorsTotals`
  only exists on the `Subscription` root type (websocket push), not `Query` — there is no
  one-shot HTTP way to read it. Adding a GraphQL-over-websocket client for one metric isn't
  justified this pass; see "Known gaps."
- Handler shapes the response as:
  ```json
  {
    "received_events_per_source": {"unifi": 1234, "hosts_tcp": 56, "hosts_tls": 0},
    "loki_sent_events_total": 1290
  }
  ```
- New config: `VECTOR_GRAPHQL_URL` (default `http://siem-ingest:8686`), added to
  `internal/config/config.go` alongside `LokiURL` (not in the `required` map — if unset or
  unreachable, the handler returns the last-known-good zero value with a `"degraded": true`
  flag rather than 502ing the whole screen, since ingest health is supplementary, not the
  page's primary data).

## Frontend structure

```text
siem-web/src/
  routes/
    sources/
      +page.server.ts       # load(): sources (status + events_per_min already computed
                             # server-side by siem-api), ingest-health summary, and — for
                             # the ?preview= source (default: first row) — one
                             # /events/search?source=X&limit=1 sample for the preview panel
      +page.svelte           # table + parser preview + right rail
    api/
      sources/
        [id]/claim/+server.ts   # thin POST passthrough to siem-api's existing
                                 # POST /sources/{id}/claim (mirrors the alerts mute/ack
                                 # proxy pattern exactly — browser never holds the bearer
                                 # token)
  lib/
    components/
      SourcesTable.svelte    # the main table; each row links to ?preview={name}
      ParserPreview.svelte   # raw line (left) / extracted fields (right), both mono
      IngestHealthPanel.svelte  # right rail: the four adapted ingest-health values
      UnclaimedSenders.svelte   # right rail: claimed === false rows + Claim button
    sources.ts                # splitClaimedUnclaimed(sources), formatEventsPerMin() —
                               # mirrors wall.ts/alerts.ts's data-shaping-helpers pattern
```

## Routing

`/sources?preview=<name>` — `preview` picks which source's parser-preview sample is
loaded (default: the first row in table order, i.e. alphabetical by name, matching
`ListSources`'s own `ORDER BY name`). Selecting a different row in `SourcesTable.svelte`
navigates via `goto` to `?preview=<name>`, triggering a fresh SSR `load()` — the same
query-param-driven-selection convention the Alerts screen already established
(`/alerts?state=...&id=...`), not a client-side fetch against a new proxy route.

## Claim flow

`UnclaimedSenders.svelte` renders a Claim button only when `userRole === 'admin'` (passed
down the same way `+layout.server.ts` already threads `locals.user.role` to the nav). On
click, `POST`s to `siem-web`'s own `/api/sources/{id}/claim` passthrough, then
optimistically removes the row from the unclaimed list on success (same optimistic-update
shape as Wall's "Mute 1h"). A non-admin viewer still sees the unclaimed list (matches
`GET /sources`'s own `viewer`+ gate) but no button — consistent with how the Alerts screen
already hides admin-only actions from lower roles rather than disabling them.

## Refresh strategy

SSR-load-once per page visit — no polling, no SSE. Justification: `heartbeat_throttle`
caps heartbeats at one per source per 15 minutes, so a source's `last_seen_at`/`status`
cannot usefully change more often than that; `events_per_min` is already a 5-minute
rolling window, not an instantaneous value that benefits from sub-second updates. This
matches Wall's own precedent — `Ticker.svelte`'s `EventSource`/SSE pattern remains
reserved for the live-tail use case it was built for, not extended here. A manual reload
(or, later, revisiting the page) is sufficient.

## Testing

Same split as the Wall and Alerts sub-projects:
- Go (`sources_test.go`, `stats_test.go`-style): the `status`/`events_per_min` computation
  in `handleListSources`, the new `internal/vector` GraphQL client against a fake HTTP
  server, and `handleIngestHealth`'s response shaping including the "Vector unreachable →
  degraded, not 502" path.
- TDD (Vitest): `sources.ts`'s data-shaping helpers (`splitClaimedUnclaimed`,
  `formatEventsPerMin`), the claim passthrough route's auth-forwarding logic (mirrors the
  existing alerts mute/ack proxy tests).
- No unit tests for presentational Svelte components.
- No new Playwright e2e — the existing login-flow test already covers the auth boundary.

## Known gaps for this pass

- **Ingest health panel shows four metrics adapted from, not identical to, the mockup's
  four** ("queue depth, Loki write p95, parse failures, dropped UDP"). Vector's GraphQL
  API has no buffer-depth or latency-percentile type at all (confirmed by full schema
  introspection during brainstorming), so this pass substitutes the closest real signals
  it does expose: per-source received-events throughput, Loki sink sent-events throughput
  (a proxy for "is Loki keeping up"), and a summed component-errors-total across the parse
  and enrichment transforms (a proxy for "parse failures"). There is no real substitute for
  "dropped UDP" specifically — nothing in the pipeline counts OS-level UDP packet loss —
  so that value isn't shown at all this pass, same honest-omission precedent as Alerts'
  "reputation" stat card.
- **`heartbeat_sec` still isn't editable** — every heartbeat call hardcodes the schema
  default (900s); this screen only displays it, doesn't let an admin change it per source.
  Pre-existing gap, documented in `siem-ingest/README.md`.
- Parser preview shows one sample (`limit=1`) per source, not a scrollable history.
- Carries forward Wall/Alerts' already-documented gaps (never-exercised real login, no
  production adapter beyond what's already shipped, etc.) — see `siem-web/README.md`.

## Decisions carried from brainstorming

- Ingest health panel: built against Vector's existing GraphQL API this pass, not a new
  Prometheus scrape pipeline.
- `status` (healthy/silent) computed server-side in siem-api's `GET /sources` response,
  reusing `StaleSources`'s threshold logic, rather than recomputed client-side in
  siem-web.
- Parser preview and row selection: query-param routing (`?preview=<name>`), matching
  Alerts' `?state=&id=` precedent, not a client-side fetch against a new proxy route.
- Refresh strategy: SSR-load-once, not polling or SSE — justified by the heartbeat
  throttle window making anything faster than manual-reload cadence not meaningfully
  fresher.
- Claim button: hidden entirely for non-admin roles, not shown-disabled.
