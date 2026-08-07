# siem-web: Search screen — design

Status: approved
Scope: fifth sub-project of the `siem-web` service (`design_handoff_homesiem/README.md`,
Screen 2 — Search). Builds on the auth/BFF + shell + Wall sub-project (PR #2), Alerts,
Sources (PR #11), and Live tail (PR #12). Covers the query bar, volume ribbon, facet rail,
virtualized result table, and event inspector with rule creation. Settings remains the
last future sub-project.

## Context

`GET /events/search` (`viewer`+, already routed) already accepts `source`, `host`,
`program`, `severity`, `facility`, `q` (free text), `start`/`end` (RFC3339), and `limit`
query params, builds a LogQL query via `loki.BuildQuery`, and returns
`{logql, count, entries}` — the compiled LogQL string is already part of the response,
satisfying the mockup's "always show the compiled query is going to Loki" requirement with
no new work. `POST /rules` (`analyst`+, already routed) already creates a threshold/
first-seen/absence rule and starts it on the scheduler if enabled — this is what backs
both "Alert on this" and "Rule from this."

Nothing existing returns time-bucketed event *counts* across an arbitrary range (as
opposed to per-source-per-hour, which `stats.go`'s `queryHourlyBySource` already does for
Wall's heat grid) — that's the one real backend gap, needed for the volume ribbon.

No virtualization exists anywhere in this codebase yet; this is the first screen that
needs it, since it's the first screen that can plausibly hold thousands of rows.

## Goals

- `/search` route: query bar (token display, 15m/24h/7d segmented control, Save disabled+
  tooltip, "Alert on this"), volume ribbon (72 bars, percentile-colored), a 184px facet
  rail (Severity/Program/Source country, each with a count, click to filter), a virtualized
  result table (time/severity/host/program/message columns), and a 284px inspector (raw
  line, parsed fields, "Filter to SRC", "Rule from this", a Context callout).
- Real virtualized rendering — only ~50 DOM rows exist at once regardless of result count,
  via fixed-row-height windowed rendering in plain Svelte, no new dependency.
- `GET /events/search` gains a `volume` field: bucketed event counts (72 buckets) across
  the requested time range, via a new `count_over_time` Loki matrix query alongside the
  existing log-entry query.
- The SSR load fetches up to 10,000 entries in one call; the table, facet rail, and volume
  ribbon (its per-bucket coloring, not its counts — see below) all operate on that same
  in-memory result.
- "Alert on this" and "Rule from this" both build real rules via the existing `POST
  /rules`, through a small pre-filled form.
- "Save" (saved searches) ships disabled with a tooltip — no saved-searches storage exists
  anywhere in this codebase.

Out of scope for this pass: the Settings screen, any saved-search backend/storage, a
server-side "distinct facet values across the full match, not just the fetched sample"
aggregation (facets are derived from the same bounded 10,000-entry fetch the table uses,
not a separate unbounded aggregation).

## Backend addition to siem-api

### `GET /events/search` — response gains `volume`

`searchResponse` gains `Volume []VolumeBucket` where `VolumeBucket{BucketStart time.Time,
Count int64}`, JSON `volume: [{bucket_start, count}, ...]`. The handler computes bucket
width as `(end - start) / 72` and issues one additional `s.deps.Loki.QueryMatrix(ctx,
countOverTimeLogQL, start, end, bucketWidth)` call using the same filter-derived LogQL
selector as the entries query (reusing `loki.BuildQuery`'s output, wrapped in
`count_over_time(...)`), the same query shape `stats.go`'s `queryTotal24h`/
`queryHourlyBySource` already use. If the volume query fails, the handler still returns
the entries successfully with `volume: []` and logs the error — volume is supplementary to
the primary search result, not allowed to fail the whole request (same "degrade, don't
502" precedent as Sources' ingest-health panel).

## Frontend structure

```text
siem-web/src/
  routes/
    search/
      +page.server.ts   # load(): reads filters + time range from the URL's query
                         # string, calls GET /events/search with limit=10000
      +page.svelte       # assembles QueryBar + VolumeRibbon + FacetRail + ResultTable
                          # + EventInspector
    api/
      search/
        rules/+server.ts   # thin POST passthrough to POST /rules — used by both
                            # "Alert on this" (query bar) and "Rule from this" (inspector)
  lib/
    components/
      QueryBar.svelte        # token display of active filters, time-range segmented
                              # control, Save (disabled+tooltip), "Alert on this"
      VolumeRibbon.svelte    # 72 bars from the search response's `volume` field,
                              # colored by percentile within that same array
      FacetRail.svelte       # Severity/Program/Source-country + counts, derived
                              # client-side from the fetched entries array
      ResultTable.svelte     # the virtualized table — fixed-row-height windowed
                              # rendering, ~50 real DOM rows regardless of result count
      EventInspector.svelte  # raw line, parsed fields, "Filter to SRC", "Rule from
                              # this", Context callout
      RuleFromEventForm.svelte  # the small pre-filled rule-creation form shared by
                                 # "Alert on this" and "Rule from this"
    search.ts                # pure helpers: URL query-string <-> filter-object
                              # (de)serialization, facet-count derivation, volume-bucket
                              # percentile-to-color mapping, virtualization window math
                              # (computeVisibleRange)
```

## Routing

`/search?source=&host=&program=&severity=&facility=&q=&range=15m|24h|7d&preview=<row-id>`
— every active filter and the time range live in the URL (so a search is shareable/
bookmarkable, and back/forward works), matching the query-param-driven-state precedent
Alerts and Sources already established. `preview` selects which row's detail shows in the
inspector, defaulting to none (inspector empty until a row is clicked).

## Virtualization

Every row renders at a fixed height. `ResultTable.svelte`'s scroll container has a real
height of `totalRows × rowHeight` (an empty spacer div, not real rows) so native scrolling
and the scrollbar thumb behave correctly against the full row count. On scroll,
`search.ts`'s `computeVisibleRange(scrollTop, containerHeight, rowHeight, totalRows)`
(a pure function, independently tested) returns the index range to render; a small buffer
of rows above/below the visible range absorbs fast-scroll flicker. Only that window's rows
are ever mounted, each absolutely positioned at `top: index * rowHeight`. No new
dependency — this technique needs nothing beyond fixed row heights and one scroll
listener.

## Facet rail & volume ribbon

Facet counts (Severity/Program/Source-country) are derived client-side from the same
up-to-10,000-entry array the load function already fetched — the same "bounded-sample
client-side derivation" pattern `wall.ts`'s `deriveCountryBreakdown` and `alerts.ts`'s
port/IP extraction already use, not a new aggregation endpoint. Clicking a facet value
adds it as a filter (updates the URL, triggers a fresh SSR load).

The volume ribbon's bar *heights/counts* come from the backend's new `volume` field
(a real Loki-side aggregation across the full time range, not just the fetched sample —
necessary because search results are capped at 10,000 but real event volume in a busy
window could exceed that). Bar *coloring* (amber above the 70th percentile, red above the
88th) is computed client-side from that same `volume` array.

## Inspector & rule creation

Selecting a row (via `?preview=<row-id>`) shows the raw line and its parsed JSON fields
side by side (same approach as Sources' `ParserPreview`). "Filter to SRC" adds the
selected event's `src_ip` as an `Extra` filter to the current search and re-runs it.
"Rule from this" and the query bar's "Alert on this" both open `RuleFromEventForm.svelte`
— name, LogQL (pre-filled: the current compiled query for "Alert on this", or a
`src_ip`-scoped query for "Rule from this"), threshold count/window — and POST to
`/api/search/rules` (the new passthrough route) on submit, which forwards to the existing
`POST /rules`. The Context callout runs one more bounded `GET /events/search` scoped to
the selected event's `src_ip` (last 24h) and shows a count summary — no new endpoint.

## Error handling

`GET /events/search` failing (non-auth `SiemApiError`) 502s the whole page — it's the
primary data source for this entire screen, same precedent as every other screen's
primary-fetch handling. A 401/403 redirects to `/auth/logout`. The volume sub-query
degrading to `[]` on failure (server-side, per the backend section above) means the ribbon
can render empty without taking down the rest of the page.

## Testing

- Go: one new test for `handleEventsSearch`'s `volume` field (fake Loki matrix response),
  and a test confirming the primary search response still succeeds when the volume
  sub-query fails.
- TDD (Vitest): `search.ts`'s pure helpers — query-string/filter-object (de)serialization,
  facet-count derivation, percentile-to-color mapping, `computeVisibleRange`'s windowing
  math (the highest-value, most bug-prone logic on this screen, and the only part with no
  real precedent elsewhere in the codebase to lean on).
- No unit tests for the Svelte components, matching every prior screen's convention.
- No new Playwright e2e.

## Known gaps for this pass

- **"Save" ships disabled with a tooltip** — no saved-searches storage exists anywhere;
  becomes real if/when that storage is built.
- **Facets are derived from the fetched 10,000-entry sample, not the true full match** —
  if a query matches more than 10,000 events in the selected range, facet counts undercount
  the true distribution (though the volume ribbon's counts remain accurate, since those
  come from a real Loki-side aggregation, not the sample).
- Carries forward Wall/Alerts/Sources/Live-tail's already-documented gaps (never-exercised
  real login, no production adapter, etc.).

## Decisions carried from brainstorming

- Virtualization: built for real (windowed rendering, no new dependency), not deferred —
  confirmed with the user after clarifying what virtualization means, since a capped/
  paginated table would miss the mockup's whole point for this screen.
- "Alert on this" and "Rule from this": built for real against the existing `POST /rules`.
  "Save": stubbed disabled with a tooltip, not built — no backend exists for it.
- Facet rail: client-side derivation from the bounded fetch, not a new aggregation
  endpoint. Volume ribbon: one new backend field (`volume`), since its counts need to
  reflect the true Loki-side match, not just the capped sample.
