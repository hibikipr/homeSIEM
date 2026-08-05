# siem-web: Live tail screen — design

Status: approved
Scope: fourth sub-project of the `siem-web` service (`design_handoff_homesiem/README.md`,
Screen 3 — Live tail). Builds on the auth/BFF + shell + Wall sub-project (PR #2), the
Alerts sub-project, and the Sources sub-project (PR #11). Covers the real-time log stream
screen: ring buffer, pause, auto-follow, severity filters, and export. Search and
Settings remain separate future sub-projects.

## Context

The data feed for this screen already exists and needs no changes: `siem-api`'s
`RunTailPoller` (`internal/api/tail_poller.go`) polls Loki every second for new entries
across all sources (no severity/source filtering — every event, every label) and publishes
each one to an `sse.Hub` topic (`"tail"`) as JSON-marshaled `loki.LogEntry{Timestamp,
Labels map[string]string, Line string}`. `Labels` carries the six allowed labels per the
design handoff's label discipline (`job, source, host, program, severity, facility`).
`GET /events/tail` (`viewer`+, already routed) serves that topic over SSE.
`siem-web/src/routes/api/tail-proxy/+server.ts` already proxies that same-origin (so the
browser never holds the bearer token), and `Ticker.svelte` (Wall screen) already consumes
it as a small always-on preview (50-row cap, no pause/filter/export). This sub-project
builds a fuller consumer of the same stream, not a replacement for Ticker.

`GET /sources` (Sources sub-project, PR #11) already exists and is reused here for the
footer's source count — no new siem-api work at all is needed for this screen.

## Goals

- `/tail` route: header (title, live/reconnecting indicator, severity filter chips,
  Pause/Export/"Search this" actions), a scrolling viewport (timestamp · severity ·
  host · program · facility · message), and a footer (buffer/wrap/sources status line).
- Ring buffer of up to 5,000 entries, always accumulating regardless of pause state.
- Auto-follow: pins to the bottom while the user hasn't scrolled up; scrolling up detaches
  and shows a "N new" affordance to re-pin.
- Pause: freezes the *rendered* view (the buffer keeps growing underneath); resuming snaps
  the view to the buffer's current state.
- Severity filter chips (all 8 standard syslog severities, active by default): toggling a
  chip filters the rendered view only, never re-fetches — everything's already buffered.
- Export: downloads the current buffer, filtered by active severity chips, as
  newline-delimited JSON matching the wire shape exactly (`{Timestamp, Labels, Line}` per
  line) — client-side only, no new backend endpoint.
- "Search this": stubbed disabled with a tooltip (Search screen doesn't exist yet).

Out of scope for this pass: the Search screen itself (and therefore any real "Search
this" pivot), server-side stream filtering (all filtering is client-side against the
already-buffered data), any change to `RunTailPoller`'s poll cadence or query scope.

## Frontend structure

```text
siem-web/src/
  routes/
    tail/
      +page.server.ts   # load(): GET /sources once, for the footer's source count
      +page.svelte       # header + TailViewport + footer
  lib/
    components/
      TailViewport.svelte   # the scrolling table: ring buffer, pause/auto-follow/scroll
    tail.ts                  # pure helpers: severity-chip filtering, NDJSON export
                              # serialization
```

`Ticker.svelte` is untouched. `TailViewport.svelte` is new and separate — different
buffer size, different feature set (pause, per-row filtering, scroll-detach), no shared
code worth extracting given how small `Ticker.svelte`'s own logic is.

## Buffer / pause / auto-follow semantics

Three independent pieces of state:

- **Buffer**: an array capped at 5,000 entries (oldest dropped first), appended to on
  every SSE message, unconditionally — pause never stops this.
- **Paused** (explicit, via the Pause button): while `true`, the rendered list does not
  update from the buffer at all. Un-pausing re-syncs the rendered list to the buffer's
  current contents in one step.
- **Auto-follow** (implicit, driven by scroll position, only meaningful while not paused):
  `true` by default; becomes `false` the moment the user scrolls away from the bottom of
  the viewport. While `true`, new rendered rows scroll into view automatically. While
  `false`, new rows still render (append to the DOM) but the scroll position doesn't move,
  and a "N new" pill (count of rows added since detaching) appears; clicking it scrolls to
  bottom and sets auto-follow back to `true`.

## Severity filter chips

Fixed set of the 8 standard syslog severities (`emerg, alert, crit, err, warning, notice,
info, debug`) — matches what Vector's syslog decoder actually derives from a real PRI
header, not a narrowed critical/warning/info-only set used elsewhere in this codebase.
All active by default. Toggling a chip off hides matching rows from the *rendered* view
only (`tail.ts`'s filter helper runs against the in-memory buffer); the buffer itself is
never filtered, so re-enabling a chip instantly reveals already-buffered matching rows.

## Export

Client-side only. Serializes the buffer's entries that pass the currently-active severity
filters as newline-delimited JSON — one raw `{Timestamp, Labels, Line}` object per line,
the same shape the wire already uses, no reshaping — via a `Blob` and an object URL
triggering a browser download. No new siem-api endpoint.

## Footer

"Buffer 5,000 lines · Wrap off · Sources: all (N)" where N is `(await GET /sources).length`
fetched once in `+page.server.ts`'s `load()`. Right-aligned: the three fixed ingest ports
(`udp/514 · tcp/601 · tls/6514`), static text — same "point a device here"-style hardcoding
precedent as the Sources screen's own static block, since these ports are fixed by
`siem-ingest`'s deployment, not derived from live data.

## Error handling

If the `/api/tail-proxy` `EventSource` errors (siem-api down, network drop), the header's
live indicator switches to a "disconnected · retrying" state (recolored, same dot).
`EventSource` reconnects natively on its own — no custom reconnect logic is needed, this
is purely a visual state change, matching the existing error-tolerant parse-and-skip
pattern already in `Ticker.svelte`'s `onmessage` handler.

## Testing

Same split as every prior siem-web sub-project:
- TDD (Vitest): `tail.ts`'s pure helpers — severity-chip filtering and NDJSON
  serialization.
- No unit tests for `TailViewport.svelte`/`+page.svelte` — presentational and
  DOM-scroll-dependent, matching the established convention (Wall/Alerts/Sources all skip
  component tests).
- No new Playwright e2e.

## Known gaps for this pass

- **"Search this" ships disabled with a tooltip** — the Search screen doesn't exist yet;
  this becomes a real pivot once that sub-project lands.
- **All filtering is client-side against the buffer**, not a server-side filtered stream —
  fine at this data volume (5,000-line cap, ~1 poll/sec), but if `siem-ingest` volume grows
  significantly, a server-side filtered SSE subscription would need to replace this.
- Carries forward Wall/Alerts/Sources' already-documented gaps (never-exercised real
  login, no production adapter, etc.) — see `siem-web/README.md`.

## Decisions carried from brainstorming

- "Search this": omitted-vs-stubbed was asked explicitly — stubbed disabled with a
  tooltip, not omitted, matching the Alerts screen's "Block at gateway" precedent.
- Export: built for real (client-side NDJSON download), not stubbed.
- Buffer/pause/auto-follow: three independent pieces of state, not one combined toggle —
  confirmed with the user before writing this spec.
- Severity chips: the full 8-value RFC 5424 syslog severity set, not a narrowed
  critical/warning/info set.
