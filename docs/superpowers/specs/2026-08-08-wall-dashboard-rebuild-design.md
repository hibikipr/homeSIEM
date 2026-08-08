# Wall dashboard rebuild — design

Status: approved
Scope: rebuilds the Wall (landing page) dashboard, found underbuilt during a live
GUI audit, plus two real correctness bugs found in adjacent code while
investigating.

## Context

The Wall currently shows: two real numbers (24h event count, open alert count)
and an intentionally-flagged placeholder ("Retention: not yet available"); a
per-source, per-hour heat grid backed by real 24-hour Loki data but with no axis
labels, legend, or discoverable tooltip; a 3-card "top triage" alert grid; a
country breakdown bar list that renders nothing when empty; and a live-only
"Ticker" feed with no historical backfill and no empty-state message. The
audit's "empty text input labeled TICKER" observation was these last two
components stacked with no empty-state messaging, reading as one broken stub.

Two real bugs were found in the process, unrelated to visual polish:
- `siem-web/src/lib/wall.ts`'s `topTriageAlerts` ranks severity using a stale
  `critical/high/medium/low` vocabulary. Real alert severity is only ever
  `info/warning/critical` (confirmed repeatedly elsewhere this project — see
  `siem-api/internal/alerts/service.go`'s `severityRank`, fixed for the same
  reason). `warning` and `info` alerts both rank `0`, so they sort by recency
  only — `warning` isn't prioritized over `info` as intended.
- `siem-web/src/lib/components/TriageCard.svelte` has CSS rules for
  `.severity-low`/`.severity-medium` that can never match, and no
  `.severity-info` rule — an `info`-severity alert falls through to the base
  `.card` style, which defaults to a **critical-red** top border. Info alerts
  are visually mislabeled as critical.

## Goals

- Fix both severity-vocabulary bugs above.
- Add a real events-over-time chart (hour-axis labels, legend/title, hover
  crosshair + tooltip per this project's dataviz conventions) showing total
  event volume across the last 24 hours — a genuinely new visual, not a
  relabeling of the existing heat grid.
- Polish the existing per-source heat grid: hour-axis labels, a color-tier
  legend, and a real hover tooltip (not just the native `title` attribute).
- Add empty-state messaging to `CountryBar` ("no international traffic in this
  sample" or similar) and `Ticker` ("waiting for live events…") instead of
  rendering nothing.
- Remove the "Retention" stat tile from `StatRow` until there's a real backend
  data source for it.
- Keep the existing 3-card triage layout (just fixed) — not replaced with a
  denser list.

## Non-goals (this pass)

- No new "recent alerts" list view beyond the existing 3-card triage grid.
- No retention backend/data source — the tile is removed, not stubbed further.
- No changes to the Sources/Alerts/Search/Live-tail/Settings screens (separate
  items from the same audit, tracked separately).

## Design

### Backend (`siem-api`)

`internal/api/stats.go`'s `handleEventsStats` already computes an hourly,
per-source `volume` map (`bySourceHourly`) to build the heat grid — it's
just never exposed as a flat total-per-hour series. Add a new
`HourlyTotals []hourlyTotal` field to `statsResponse` (`json:"hourly_totals"`),
computed by summing `volume[source][hour]` across all sources for each of the
24 hourly buckets already being iterated for `buildHeatGrid`. No new Loki
query — this reuses data already fetched for the heat grid in the same
request.

```go
type hourlyTotal struct {
	HourStart time.Time `json:"hour_start"`
	Count     int64     `json:"count"`
}
```

### Frontend data layer (`siem-web/src/lib/server/siemApiClient.ts`)

`EventsStatsResponse` gains `hourly_totals: { hour_start: string; count: number }[]`.

### New component: `EventsOverTime.svelte`

A single-series area chart (this project has no charting library — hand-rolled
inline SVG, consistent with `HeatGrid`/`CountryBar`'s existing pattern):
- X axis: sparse hour labels (every 4 hours, e.g. `00:00`, `04:00`, ... `20:00`)
  in `--color-muted-2`, matching this app's existing eyebrow/label styling.
- Y axis: a `0` baseline and a single max-value label — recessive, not a full
  gridline system, matching the dataviz skill's "recessive grid/axes" guidance.
- Mark: a 2px line in `--color-accent-light` with a soft area fill fading from
  `--color-accent-tint-2` to transparent underneath — this app's existing
  accent ramp, already used elsewhere (`CountryBar`'s `.fill`, `HeatGrid`'s
  busy/light/quiet tiers), not a new color introduced for this chart.
- Hover: a vertical crosshair line following pointer X position, snapping to
  the nearest of the 24 hourly points, with a small tooltip showing the exact
  hour and count. Single series — no legend box needed (the chart's own title
  names it), per the dataviz skill's "a single series needs no legend box"
  rule.
- Empty state: if every bucket is `0` (or the array is empty), show "No events
  in the last 24h" instead of a flat zero-line chart.

### `HeatGrid.svelte` polish

- Add hour-axis labels below the grid (same sparse 4-hour interval as the new
  chart, for visual consistency between the two).
- Add a small legend row mapping each tier color
  (critical/warning/busy/light/quiet/none) to its label — the tier colors
  themselves are unchanged (critical/warning already correctly use the
  reserved `--color-severity-*` status tokens; busy/light/quiet/none already
  form a sound one-hue sequential ramp on the accent color — this is a
  labeling addition, not a color change).
- Replace the native `title` attribute with a real hover tooltip (small
  positioned box, not the browser's default), matching the new chart's
  tooltip styling for consistency.

### Bug fixes

**`siem-web/src/lib/wall.ts`**: `SEVERITY_RANK` updated to
`{ critical: 3, warning: 2, info: 1 }`, matching the real vocabulary.

**`siem-web/src/lib/components/TriageCard.svelte`**: CSS rules updated —
remove `.severity-low`/`.severity-medium`, add `.severity-info` (using
`--color-severity-info`, the existing-but-currently-unused token already
defined in `tokens.css` for exactly this purpose), keep `.severity-warning`
as-is. The base `.card` rule's default box-shadow color changed from
critical-red to the info token, so any future unrecognized severity value
defaults to the least alarming color rather than the most alarming.

### Empty states

**`CountryBar.svelte`**: when `countries` is empty, show a short message in
place of the (currently entirely absent) row list — e.g. "No international
traffic in this sample."

**`Ticker.svelte`**: show "Waiting for live events…" until the first SSE
message arrives, replacing the current silent-empty-list behavior.

### `StatRow.svelte`

Remove the third "Retention" stat tile and its explanatory comment entirely.
`.stat-row` still has two real tiles (Events 24h, Open alerts) — the
`.placeholder`/`margin-left: auto` layout rule that pushed it to the right is
removed with it.

## Testing

- `siem-api`: unit test for the new `hourly_totals` computation in
  `stats.go` (sums across sources correctly, handles a source with partial
  hourly data, handles zero sources).
- `siem-web`: unit tests for the two bug fixes (`topTriageAlerts` ranking
  order with the real vocabulary; `wall.test.ts` already exists per this
  project's pattern) and any new pure logic extracted for the chart (e.g. a
  function computing sparse axis label positions/nearest-point snapping,
  testable independently of the SVG rendering).
- Manual Playwright verification for the new chart's hover interaction and
  both new empty states, per this codebase's established no-component-test-
  infrastructure constraint.

## Known gaps after this pass

- No retention data source — tile removed, not deferred-with-a-stub.
- No new "recent alerts" list format — existing 3-card grid, fixed not
  replaced.
- The other items from the same audit (Settings stub tabs, Alerts/Live-tail
  empty states, minor polish) are tracked separately, not in this pass.
