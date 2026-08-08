# Wall Dashboard Rebuild Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rebuild the Wall (landing page) dashboard: fix two real severity-vocabulary bugs, add a real events-over-time chart, polish the existing per-source heat grid, add empty-state messaging where panels currently render nothing, and remove an exposed "not yet available" placeholder.

**Architecture:** `siem-api`'s `handleEventsStats` already computes an hourly, per-source volume map to build the heat grid — Task 1 exposes a flat total-per-hour series from that same already-fetched data (no new Loki query). Everything else is `siem-web` frontend work: two small bug fixes, one new hand-rolled inline-SVG chart component (this codebase has no charting library — consistent with `HeatGrid`/`CountryBar`'s existing pattern), polish to the existing heat grid, and small empty-state additions.

**Tech Stack:** Go (siem-api), SvelteKit (Svelte 5 runes), TypeScript, Vitest, inline SVG (no charting library).

## Global Constraints

- Real alert/event severity is only ever `info`/`warning`/`critical` — never `critical`/`high`/`medium`/`low`. Every place this plan touches severity values or ranking must use the real vocabulary.
- The new chart and the polished heat grid share the same sparse hour-axis-label convention (every 4 hours) and the same tooltip visual style, for consistency between the two.
- No new Loki query for the events-over-time chart — it's built from data `handleEventsStats` already fetches for the heat grid.
- This codebase has no Svelte component test infrastructure and none should be added. Cover pure logic (severity ranking, chart point/label computation) with plain Vitest unit tests; verify SVG rendering and hover interaction manually via Playwright with a minted session cookie, per this project's established pattern.
- No retention backend work — the `StatRow` tile is removed outright, not replaced with another stub.

---

### Task 1: Backend — expose `hourly_totals` in `/events/stats`

**Files:**
- Modify: `siem-api/internal/api/stats.go`
- Modify: `siem-api/internal/api/stats_test.go`
- Modify: `siem-web/src/lib/server/siemApiClient.ts`

**Interfaces:**
- Produces: `statsResponse.HourlyTotals []hourlyTotal` (JSON: `hourly_totals: [{hour_start: string, count: number}]`), sorted ascending by `hour_start`. Task 3 consumes this via `EventsStatsResponse.hourly_totals` in `siem-web`.

- [ ] **Step 1: Write the failing Go tests**

In `siem-api/internal/api/stats_test.go`, add a new standalone test after the existing `TestEventsStats_ReturnsTotalAndHeatGrid`:

```go
func TestBuildHourlyTotals_SumsAcrossSources(t *testing.T) {
	volume := bySourceHourly{
		"udm-ultra": {1700000000: 1, 1700003600: 0},
		"host-1":    {1700000000: 60, 1700003600: 3},
	}

	totals := buildHourlyTotals(volume)

	if len(totals) != 2 {
		t.Fatalf("len(totals) = %d, want 2", len(totals))
	}
	if totals[0].HourStart.Unix() != 1700000000 || totals[0].Count != 61 {
		t.Errorf("totals[0] = %+v, want {1700000000, 61}", totals[0])
	}
	if totals[1].HourStart.Unix() != 1700003600 || totals[1].Count != 3 {
		t.Errorf("totals[1] = %+v, want {1700003600, 3}", totals[1])
	}
}

func TestBuildHourlyTotals_EmptyVolume(t *testing.T) {
	totals := buildHourlyTotals(bySourceHourly{})
	if len(totals) != 0 {
		t.Errorf("len(totals) = %d, want 0", len(totals))
	}
}
```

Also extend the existing `TestEventsStats_ReturnsTotalAndHeatGrid` test — add this block right after the existing `host1.Hours[1]` assertion (before the closing `}` of the function):

```go
	if len(resp.HourlyTotals) != 2 {
		t.Fatalf("len(HourlyTotals) = %d, want 2, got %+v", len(resp.HourlyTotals), resp.HourlyTotals)
	}
	if resp.HourlyTotals[0].Count != 61 {
		t.Errorf("HourlyTotals[0].Count = %d, want 61 (1 udm-ultra + 60 host-1)", resp.HourlyTotals[0].Count)
	}
	if resp.HourlyTotals[1].Count != 3 {
		t.Errorf("HourlyTotals[1].Count = %d, want 3 (0 udm-ultra + 3 host-1)", resp.HourlyTotals[1].Count)
	}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd siem-api && go test ./internal/api/... -run "TestBuildHourlyTotals|TestEventsStats_ReturnsTotalAndHeatGrid" -v`
Expected: FAIL to compile (`buildHourlyTotals`, `hourlyTotal`, and `resp.HourlyTotals` don't exist yet).

- [ ] **Step 3: Add `hourlyTotal`, `buildHourlyTotals`, and wire it into the handler**

In `siem-api/internal/api/stats.go`, add to the `statsResponse` struct:

```go
type statsResponse struct {
	EventCount24h int64           `json:"event_count_24h"`
	HeatGrid      []sourceHeatRow `json:"heat_grid"`
	HourlyTotals  []hourlyTotal   `json:"hourly_totals"`
}

type hourlyTotal struct {
	HourStart time.Time `json:"hour_start"`
	Count     int64     `json:"count"`
}
```

Add the new function (near `buildHeatGrid`):

```go
// buildHourlyTotals sums the same per-source hourly volume buildHeatGrid uses
// across all sources, producing a flat total-events-per-hour series - no new
// Loki query needed, this reuses data already fetched for the heat grid.
func buildHourlyTotals(volume bySourceHourly) []hourlyTotal {
	sums := map[int64]float64{}
	for _, hours := range volume {
		for ts, count := range hours {
			sums[ts] += count
		}
	}

	var timestamps []int64
	for ts := range sums {
		timestamps = append(timestamps, ts)
	}
	sort.Slice(timestamps, func(i, j int) bool { return timestamps[i] < timestamps[j] })

	totals := make([]hourlyTotal, len(timestamps))
	for i, ts := range timestamps {
		totals[i] = hourlyTotal{HourStart: time.Unix(ts, 0).UTC(), Count: int64(sums[ts])}
	}
	return totals
}
```

In `handleEventsStats`, update the response construction:

```go
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(statsResponse{
		EventCount24h: total,
		HeatGrid:      buildHeatGrid(critical, warning, volume),
		HourlyTotals:  buildHourlyTotals(volume),
	})
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd siem-api && go test ./internal/api/... -v`
Expected: All PASS, including every pre-existing test in this package.

- [ ] **Step 5: Add the field to the frontend client type**

In `siem-web/src/lib/server/siemApiClient.ts`, update `EventsStatsResponse`:

```ts
export interface EventsStatsResponse {
	event_count_24h: number;
	heat_grid: { source: string; hours: string[] }[];
	hourly_totals: { hour_start: string; count: number }[];
}
```

- [ ] **Step 6: Run the full siem-api and siem-web test suites**

Run: `cd siem-api && go build ./... && go vet ./... && go test ./...`
Run: `cd siem-web && pnpm exec vitest run`
Expected: All PASS (the `siem-web` change is a type-only addition; no existing test constructs an `EventsStatsResponse` literal that would need updating — if `pnpm exec svelte-check` or `vitest run` surfaces one, add the field there too).

- [ ] **Step 7: Commit**

```bash
git add siem-api/internal/api/stats.go siem-api/internal/api/stats_test.go \
  siem-web/src/lib/server/siemApiClient.ts
git commit -m "Expose hourly_totals in /events/stats, reusing already-fetched heat grid data"
```

---

### Task 2: Fix severity-vocabulary bugs in triage ranking and card styling

**Files:**
- Modify: `siem-web/src/lib/wall.ts`
- Modify: `siem-web/src/lib/wall.test.ts`
- Modify: `siem-web/src/lib/components/TriageCard.svelte`

**Interfaces:**
- None new — this is a pure bug fix to existing exports (`topTriageAlerts`) and CSS.

- [ ] **Step 1: Write the failing tests**

In `siem-web/src/lib/wall.test.ts`, update the `alert()` helper's default severity (currently `'low'`, which no longer exists in the real vocabulary):

```ts
function alert(overrides: Partial<AlertResponse>): AlertResponse {
	return {
		id: 1,
		rule_id: 1,
		group_key: 'a',
		severity: 'info',
		title: 't',
		body: 'b',
		event_count: 1,
		state: 'open',
		first_seen_at: '2026-08-02T00:00:00Z',
		last_seen_at: '2026-08-02T00:00:00Z',
		...overrides
	};
}
```

Update the existing `'sorts by severity rank descending, then recency descending'` test's fixture data to the real vocabulary (`'low'` → `'info'`, `'medium'` → `'warning'`; same expected order since the relative ranking is preserved):

```ts
	it('sorts by severity rank descending, then recency descending', () => {
		const alerts = [
			alert({ id: 1, severity: 'info', last_seen_at: '2026-08-02T03:00:00Z' }),
			alert({ id: 2, severity: 'critical', last_seen_at: '2026-08-02T01:00:00Z' }),
			alert({ id: 3, severity: 'critical', last_seen_at: '2026-08-02T02:00:00Z' }),
			alert({ id: 4, severity: 'warning', last_seen_at: '2026-08-02T04:00:00Z' })
		];

		const top = topTriageAlerts(alerts, 3);

		expect(top.map((a) => a.id)).toEqual([3, 2, 4]);
	});
```

Add a new test proving the exact bug this task fixes, directly after it:

```ts
	it('ranks warning above info even when info is more recent (regression: both used to rank 0)', () => {
		const alerts = [
			alert({ id: 1, severity: 'info', last_seen_at: '2026-08-02T05:00:00Z' }),
			alert({ id: 2, severity: 'warning', last_seen_at: '2026-08-02T01:00:00Z' })
		];

		const top = topTriageAlerts(alerts, 2);

		expect(top.map((a) => a.id)).toEqual([2, 1]);
	});
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd siem-web && pnpm exec vitest run src/lib/wall.test.ts`
Expected: FAIL — the new regression test fails (both alerts currently rank 0, so `info`'s more-recent timestamp puts id 1 first); the updated first test may also fail depending on current `SEVERITY_RANK` defaults for the new string values.

- [ ] **Step 3: Fix `SEVERITY_RANK`**

In `siem-web/src/lib/wall.ts`, replace:

```ts
const SEVERITY_RANK: Record<string, number> = { critical: 4, high: 3, medium: 2, low: 1 };
```

with:

```ts
const SEVERITY_RANK: Record<string, number> = { critical: 3, warning: 2, info: 1 };
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd siem-web && pnpm exec vitest run src/lib/wall.test.ts`
Expected: PASS

- [ ] **Step 5: Fix `TriageCard.svelte`'s severity CSS**

In `siem-web/src/lib/components/TriageCard.svelte`, replace:

```css
	.card {
		background: var(--color-surface-2);
		border-radius: var(--radius-default);
		padding: var(--space-4);
		box-shadow: inset 0 2px 0 var(--color-severity-critical);
	}
	.card.severity-warning {
		box-shadow: inset 0 2px 0 var(--color-severity-warning);
	}
	.card.severity-low,
	.card.severity-medium {
		box-shadow: inset 0 2px 0 var(--color-severity-info);
	}
```

with:

```css
	.card {
		background: var(--color-surface-2);
		border-radius: var(--radius-default);
		padding: var(--space-4);
		box-shadow: inset 0 2px 0 var(--color-severity-info);
	}
	.card.severity-critical {
		box-shadow: inset 0 2px 0 var(--color-severity-critical);
	}
	.card.severity-warning {
		box-shadow: inset 0 2px 0 var(--color-severity-warning);
	}
```

Also update the `.header` rule directly below it, which hardcodes the eyebrow text color to critical-red regardless of actual severity:

```css
	.header {
		display: flex;
		justify-content: space-between;
		font-size: var(--text-eyebrow);
		text-transform: uppercase;
		color: var(--color-severity-critical);
	}
```

replace with:

```css
	.header {
		display: flex;
		justify-content: space-between;
		font-size: var(--text-eyebrow);
		text-transform: uppercase;
		color: var(--color-severity-info);
	}
	.card.severity-critical .header {
		color: var(--color-severity-critical);
	}
	.card.severity-warning .header {
		color: var(--color-severity-warning);
	}
```

- [ ] **Step 6: Run the full siem-web test suite, lint, and type-check**

Run: `cd siem-web && pnpm exec vitest run && pnpm lint && pnpm exec svelte-check`
Expected: All PASS, no new type errors.

- [ ] **Step 7: Commit**

```bash
git add siem-web/src/lib/wall.ts siem-web/src/lib/wall.test.ts \
  siem-web/src/lib/components/TriageCard.svelte
git commit -m "Fix triage severity ranking and card styling to use the real info/warning/critical vocabulary"
```

---

### Task 3: New `EventsOverTime` chart component

**Files:**
- Create: `siem-web/src/lib/components/EventsOverTime.svelte`
- Create: `siem-web/src/lib/eventsOverTime.ts`
- Create: `siem-web/src/lib/eventsOverTime.test.ts`
- Modify: `siem-web/src/routes/+page.server.ts`
- Modify: `siem-web/src/routes/+page.svelte`

**Interfaces:**
- Consumes: `data.hourlyTotals` (from Task 1's `EventsStatsResponse.hourly_totals`, threaded through `+page.server.ts`'s existing `Promise.all` load).
- Produces: pure helper functions in `eventsOverTime.ts` (`computeChartPoints`, `formatHourLabel`) that `EventsOverTime.svelte` uses — extracted so the geometry/label logic is unit-testable without rendering SVG.

- [ ] **Step 1: Write the failing tests for the pure logic**

Create `siem-web/src/lib/eventsOverTime.ts` is created in Step 3 below; write its test first.

Create `siem-web/src/lib/eventsOverTime.test.ts`:

```ts
import { describe, it, expect } from 'vitest';
import { computeChartPoints, formatHourLabel, CHART_WIDTH, CHART_HEIGHT } from './eventsOverTime';

describe('computeChartPoints', () => {
	it('scales count to the chart height, with the max count at y=0', () => {
		const totals = [
			{ hour_start: '2026-08-02T00:00:00Z', count: 10 },
			{ hour_start: '2026-08-02T01:00:00Z', count: 100 },
			{ hour_start: '2026-08-02T02:00:00Z', count: 50 }
		];

		const points = computeChartPoints(totals);

		expect(points).toHaveLength(3);
		expect(points[1].y).toBe(0); // max count -> top of chart
		expect(points[0].y).toBe(CHART_HEIGHT * 0.9); // 10/100 of the way up
		expect(points[2].y).toBe(CHART_HEIGHT * 0.5); // 50/100 of the way up
	});

	it('spaces points evenly across the chart width', () => {
		const totals = [
			{ hour_start: '2026-08-02T00:00:00Z', count: 1 },
			{ hour_start: '2026-08-02T01:00:00Z', count: 1 },
			{ hour_start: '2026-08-02T02:00:00Z', count: 1 }
		];

		const points = computeChartPoints(totals);

		expect(points[0].x).toBe(0);
		expect(points[1].x).toBe(CHART_WIDTH / 2);
		expect(points[2].x).toBe(CHART_WIDTH);
	});

	it('handles an all-zero series without dividing by zero', () => {
		const totals = [
			{ hour_start: '2026-08-02T00:00:00Z', count: 0 },
			{ hour_start: '2026-08-02T01:00:00Z', count: 0 }
		];

		const points = computeChartPoints(totals);

		expect(points.every((p) => Number.isFinite(p.y))).toBe(true);
	});

	it('returns an empty array for an empty series', () => {
		expect(computeChartPoints([])).toEqual([]);
	});
});

describe('formatHourLabel', () => {
	it('formats an ISO timestamp as HH:00 in UTC', () => {
		expect(formatHourLabel('2026-08-02T05:00:00Z')).toBe('05:00');
		expect(formatHourLabel('2026-08-02T00:00:00Z')).toBe('00:00');
		expect(formatHourLabel('2026-08-02T23:00:00Z')).toBe('23:00');
	});
});
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd siem-web && pnpm exec vitest run src/lib/eventsOverTime.test.ts`
Expected: FAIL — `./eventsOverTime` doesn't exist yet.

- [ ] **Step 3: Implement the pure logic**

Create `siem-web/src/lib/eventsOverTime.ts`:

```ts
export const CHART_WIDTH = 760;
export const CHART_HEIGHT = 140;

export interface ChartPoint {
	x: number;
	y: number;
	hourStart: string;
	count: number;
}

export function computeChartPoints(
	totals: { hour_start: string; count: number }[]
): ChartPoint[] {
	if (totals.length === 0) return [];

	const maxCount = Math.max(1, ...totals.map((t) => t.count));

	return totals.map((t, i) => ({
		x: totals.length > 1 ? (i / (totals.length - 1)) * CHART_WIDTH : CHART_WIDTH / 2,
		y: CHART_HEIGHT - (t.count / maxCount) * CHART_HEIGHT,
		hourStart: t.hour_start,
		count: t.count
	}));
}

export function formatHourLabel(iso: string): string {
	const d = new Date(iso);
	return `${String(d.getUTCHours()).padStart(2, '0')}:00`;
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd siem-web && pnpm exec vitest run src/lib/eventsOverTime.test.ts`
Expected: PASS

- [ ] **Step 5: Build the chart component**

Create `siem-web/src/lib/components/EventsOverTime.svelte`:

```svelte
<script lang="ts">
	import { computeChartPoints, formatHourLabel, CHART_WIDTH, CHART_HEIGHT } from '$lib/eventsOverTime';

	let { totals }: { totals: { hour_start: string; count: number }[] } = $props();

	let points = $derived(computeChartPoints(totals));
	let hasData = $derived(totals.some((t) => t.count > 0));
	let maxCount = $derived(Math.max(1, ...totals.map((t) => t.count)));

	let linePath = $derived(points.map((p, i) => `${i === 0 ? 'M' : 'L'} ${p.x} ${p.y}`).join(' '));
	let areaPath = $derived(
		points.length > 0
			? `M ${points[0].x} ${CHART_HEIGHT} ` +
				points.map((p) => `L ${p.x} ${p.y}`).join(' ') +
				` L ${points[points.length - 1].x} ${CHART_HEIGHT} Z`
			: ''
	);

	let axisLabelPoints = $derived(points.filter((_, i) => i % 4 === 0));

	let svgEl: SVGSVGElement | undefined = $state();
	let hoveredIndex = $state<number | null>(null);

	function handlePointerMove(event: PointerEvent) {
		if (!svgEl || points.length === 0) return;
		const rect = svgEl.getBoundingClientRect();
		const scaleX = CHART_WIDTH / rect.width;
		const localX = (event.clientX - rect.left) * scaleX;
		const index = Math.max(
			0,
			Math.min(points.length - 1, Math.round((localX / CHART_WIDTH) * (points.length - 1)))
		);
		hoveredIndex = index;
	}

	function handlePointerLeave() {
		hoveredIndex = null;
	}

	let hoveredPoint = $derived(hoveredIndex !== null ? points[hoveredIndex] : null);
	let tooltipTransform = $derived.by(() => {
		if (hoveredIndex === null) return 'translateX(-50%)';
		if (hoveredIndex === 0) return 'translateX(0)';
		if (hoveredIndex === points.length - 1) return 'translateX(-100%)';
		return 'translateX(-50%)';
	});
</script>

<div class="events-over-time">
	<div class="eyebrow">Events, last 24h</div>
	{#if !hasData}
		<div class="empty">No events in the last 24h</div>
	{:else}
		<div class="chart-wrap">
			<svg
				bind:this={svgEl}
				viewBox="0 0 {CHART_WIDTH} {CHART_HEIGHT}"
				preserveAspectRatio="none"
				class="chart"
				onpointermove={handlePointerMove}
				onpointerleave={handlePointerLeave}
			>
				<path d={areaPath} class="area" />
				<path d={linePath} class="line" />
				{#if hoveredPoint}
					<line
						x1={hoveredPoint.x}
						y1="0"
						x2={hoveredPoint.x}
						y2={CHART_HEIGHT}
						class="crosshair"
					/>
					<circle cx={hoveredPoint.x} cy={hoveredPoint.y} r="4" class="marker" />
				{/if}
			</svg>
			{#if hoveredPoint}
				<div
					class="tooltip"
					style:left="{(hoveredPoint.x / CHART_WIDTH) * 100}%"
					style:transform={tooltipTransform}
				>
					<span class="tooltip-hour">{formatHourLabel(hoveredPoint.hourStart)}</span>
					<span class="tooltip-count">{hoveredPoint.count} events</span>
				</div>
			{/if}
		</div>
		<div class="axis">
			{#each axisLabelPoints as point (point.hourStart)}
				<span class="axis-label" style:left="{(point.x / CHART_WIDTH) * 100}%">
					{formatHourLabel(point.hourStart)}
				</span>
			{/each}
		</div>
		<div class="max-label">max {maxCount}/h</div>
	{/if}
</div>

<style>
	.events-over-time {
		background: var(--color-surface-2);
		border-radius: var(--radius-default);
		box-shadow: var(--shadow-flat);
		padding: var(--space-4);
		position: relative;
	}
	.eyebrow {
		font-size: var(--text-eyebrow);
		text-transform: uppercase;
		letter-spacing: 0.1em;
		color: var(--color-muted-2);
		margin-bottom: var(--space-3);
	}
	.empty {
		font-size: var(--text-table);
		color: var(--color-muted);
		padding: var(--space-5) 0;
		text-align: center;
	}
	.chart-wrap {
		position: relative;
	}
	.chart {
		width: 100%;
		height: 140px;
		display: block;
		cursor: crosshair;
	}
	.area {
		fill: var(--color-accent-tint-2);
		opacity: 0.5;
		stroke: none;
	}
	.line {
		fill: none;
		stroke: var(--color-accent-light);
		stroke-width: 2;
	}
	.crosshair {
		stroke: var(--color-line-2);
		stroke-width: 1;
	}
	.marker {
		fill: var(--color-accent-light);
	}
	.tooltip {
		position: absolute;
		top: var(--space-2);
		display: flex;
		flex-direction: column;
		gap: 2px;
		background: var(--color-surface-3);
		box-shadow: var(--shadow-flat);
		border-radius: var(--radius-sm);
		padding: var(--space-2) var(--space-3);
		pointer-events: none;
		white-space: nowrap;
	}
	.tooltip-hour {
		font-size: var(--text-label);
		color: var(--color-muted);
	}
	.tooltip-count {
		font-size: var(--text-table);
		color: var(--color-text);
	}
	.axis {
		position: relative;
		height: var(--space-5);
	}
	.axis-label {
		position: absolute;
		font-size: var(--text-label);
		color: var(--color-muted-2);
		transform: translateX(-50%);
	}
	.max-label {
		position: absolute;
		top: var(--space-4);
		right: var(--space-4);
		font-size: var(--text-label);
		color: var(--color-muted-2);
	}
</style>
```

- [ ] **Step 6: Wire `hourlyTotals` through the load function**

In `siem-web/src/routes/+page.server.ts`, add `hourlyTotals: stats.hourly_totals` to the returned object:

```ts
	return {
		eventCount24h: stats.event_count_24h,
		heatGrid: stats.heat_grid,
		hourlyTotals: stats.hourly_totals,
		openAlertCount: openAlerts.length,
		triageAlerts: topTriageAlerts(openAlerts),
		countryBreakdown: deriveCountryBreakdown(sample.entries)
	};
```

- [ ] **Step 7: Add the chart to the Wall page**

In `siem-web/src/routes/+page.svelte`, add the import and render it between `StatRow` and `HeatGrid`:

```svelte
	import EventsOverTime from '$lib/components/EventsOverTime.svelte';
```

```svelte
		<StatRow eventCount24h={data.eventCount24h} openAlertCount={data.openAlertCount} />
		<EventsOverTime totals={data.hourlyTotals} />
		<HeatGrid rows={data.heatGrid} />
```

- [ ] **Step 8: Manually verify the chart in a real browser**

Per Global Constraints, no component test infrastructure — verify by hand using the same minted-session-cookie Playwright technique established earlier this session:

1. Start the dev server: `cd siem-web && pnpm dev`.
2. Mint a valid session cookie and navigate to the Wall page.
3. Confirm the chart renders with a visible line/area, hour labels beneath it, and a max-value label.
4. Move the pointer across the chart: confirm a crosshair line, a marker dot, and a tooltip (showing the correct hour and count) follow the pointer and snap to the nearest hourly point.
5. Move the pointer off the chart: confirm the crosshair/marker/tooltip disappear.
6. Temporarily pass an all-zero or empty `totals` array (e.g. by editing `+page.server.ts` briefly) and confirm the "No events in the last 24h" empty state renders instead of a flat zero-line. Revert the temporary edit afterward.

- [ ] **Step 9: Run the full siem-web test suite, lint, and type-check**

Run: `cd siem-web && pnpm exec vitest run && pnpm lint && pnpm exec svelte-check`
Expected: All PASS, no new type errors.

- [ ] **Step 10: Commit**

```bash
git add siem-web/src/lib/components/EventsOverTime.svelte siem-web/src/lib/eventsOverTime.ts \
  siem-web/src/lib/eventsOverTime.test.ts siem-web/src/routes/+page.server.ts \
  siem-web/src/routes/+page.svelte
git commit -m "Add EventsOverTime chart with hover crosshair/tooltip to the Wall dashboard"
```

---

### Task 4: `HeatGrid.svelte` polish — axis labels, legend, real tooltip

**Files:**
- Modify: `siem-web/src/lib/components/HeatGrid.svelte`

**Interfaces:**
- None new. (`HeatGrid`'s cells carry only a tier string per hour, not a real timestamp — unlike Task 3's `hourly_totals`, so it can't reuse `formatHourLabel`; its axis uses relative "N hours ago" / "now" endpoint labels instead, see Step 1.)

- [ ] **Step 1: Add hour-axis labels and a legend**

In `siem-web/src/lib/components/HeatGrid.svelte`, update the script block:

```svelte
<script lang="ts">
	import { heatTierColor } from '$lib/wall';

	let { rows }: { rows: { source: string; hours: string[] }[] } = $props();

	const LEGEND_TIERS = ['critical', 'warning', 'busy', 'light', 'quiet', 'none'] as const;
</script>
```

`sourceHeatRow.hours` (from `siem-api`) is just an ordered tier-string array — each cell has no real timestamp to format, unlike Task 3's `hourly_totals`. The axis below uses relative endpoint labels ("N hours ago" → "now") instead of clock times. Replace the template with:

```svelte
<div class="heat-grid">
	{#each rows as row (row.source)}
		<div class="row">
			<span class="label">{row.source}</span>
			<div class="cells">
				{#each row.hours as tier, i (i)}
					<span
						class="cell"
						style="background: {heatTierColor(tier)}"
						title="{tier} ({row.hours.length - i} hour{row.hours.length - i === 1 ? '' : 's'} ago)"
					></span>
				{/each}
			</div>
		</div>
	{/each}
	{#if rows.length > 0}
		<div class="axis">
			<span class="axis-start">{rows[0].hours.length - 1} hours ago</span>
			<span class="axis-end">now</span>
		</div>
	{/if}
	<div class="legend">
		{#each LEGEND_TIERS as tier (tier)}
			<span class="legend-item">
				<span class="legend-swatch" style="background: {heatTierColor(tier)}"></span>
				{tier}
			</span>
		{/each}
	</div>
</div>
```

(The per-cell `title` attribute is upgraded from a bare tier name to a relative-age description — still the browser's native tooltip, which the design spec called out to replace with a richer hover tooltip; see Step 2 for the upgrade to a real positioned tooltip. `formatHourLabel` isn't actually usable here since individual cells have no real timestamp in the current data shape, only relative position — the import is removed from the final version; don't leave an unused import.)

- [ ] **Step 2: Replace the native tooltip with a real hover tooltip**

Since heat grid cells don't carry real timestamps (unlike the new chart), a full crosshair+tooltip isn't meaningful here the same way — but per-cell hover should still show tier + relative age in a styled tooltip rather than the browser's default. Update the script to track a hovered cell:

```svelte
<script lang="ts">
	import { heatTierColor } from '$lib/wall';

	let { rows }: { rows: { source: string; hours: string[] }[] } = $props();

	const LEGEND_TIERS = ['critical', 'warning', 'busy', 'light', 'quiet', 'none'] as const;

	let hovered = $state<{ source: string; tier: string; hoursAgo: number } | null>(null);

	function ageLabel(hoursAgo: number): string {
		return hoursAgo === 0 ? 'this hour' : `${hoursAgo}h ago`;
	}
</script>
```

And the template's per-cell span:

```svelte
					<span
						class="cell"
						style="background: {heatTierColor(tier)}"
						onpointerenter={() =>
							(hovered = { source: row.source, tier, hoursAgo: row.hours.length - 1 - i })}
						onpointerleave={() => (hovered = null)}
					></span>
```

Add the tooltip markup right after the `.heat-grid` div's closing content (still inside it, as a sibling to `.legend`):

```svelte
	{#if hovered}
		<div class="hover-tooltip">
			<span class="tooltip-source">{hovered.source}</span>
			<span class="tooltip-tier">{hovered.tier} · {ageLabel(hovered.hoursAgo)}</span>
		</div>
	{/if}
```

- [ ] **Step 3: Update the CSS**

Add to `siem-web/src/lib/components/HeatGrid.svelte`'s `<style>` block:

```css
	.axis {
		display: flex;
		justify-content: space-between;
		margin-top: var(--space-1);
		margin-left: calc(96px + var(--space-2));
		font-size: var(--text-label);
		color: var(--color-muted-2);
	}
	.legend {
		display: flex;
		gap: var(--space-4);
		margin-top: var(--space-3);
		flex-wrap: wrap;
	}
	.legend-item {
		display: flex;
		align-items: center;
		gap: var(--space-1);
		font-size: var(--text-label);
		color: var(--color-muted);
		text-transform: capitalize;
	}
	.legend-swatch {
		width: 10px;
		height: 10px;
		border-radius: 2px;
		flex-shrink: 0;
	}
	.hover-tooltip {
		position: absolute;
		background: var(--color-surface-3);
		box-shadow: var(--shadow-flat);
		border-radius: var(--radius-sm);
		padding: var(--space-2) var(--space-3);
		font-size: var(--text-label);
		display: flex;
		flex-direction: column;
		gap: 2px;
		pointer-events: none;
		z-index: 10;
	}
	.tooltip-source {
		color: var(--color-muted);
	}
	.tooltip-tier {
		color: var(--color-text);
		text-transform: capitalize;
	}
```

Also add `position: relative;` to the existing `.heat-grid` rule so the absolutely-positioned tooltip is contained within it:

```css
	.heat-grid {
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
		position: relative;
	}
```

Note: this simple version doesn't follow the tooltip's exact cursor position (unlike the chart's crosshair tooltip) — it renders at a fixed spot. This is an acceptable, deliberately simpler treatment for a per-cell grid tooltip versus a continuous line chart's crosshair; if precise positioning is wanted later, thread the hovered cell's bounding rect through the same way `EventsOverTime` does.

- [ ] **Step 4: Manually verify in a real browser**

Using the same Playwright technique as Task 3: confirm hour-axis labels and the legend render below the grid, and hovering a cell shows the styled tooltip (source + tier + relative age) instead of the browser's native tooltip.

- [ ] **Step 5: Run the full siem-web test suite, lint, and type-check**

Run: `cd siem-web && pnpm exec vitest run && pnpm lint && pnpm exec svelte-check`
Expected: All PASS, no new type errors.

- [ ] **Step 6: Commit**

```bash
git add siem-web/src/lib/components/HeatGrid.svelte
git commit -m "Add hour-axis labels, a tier legend, and a real hover tooltip to HeatGrid"
```

---

### Task 5: Empty states (CountryBar, Ticker) and remove the Retention stat tile

**Files:**
- Modify: `siem-web/src/lib/components/CountryBar.svelte`
- Modify: `siem-web/src/lib/components/Ticker.svelte`
- Modify: `siem-web/src/lib/components/StatRow.svelte`

**Interfaces:**
- None new.

- [ ] **Step 1: Add an empty state to `CountryBar.svelte`**

Replace the template:

```svelte
<div class="country-bar">
	<div class="eyebrow">Where it's coming from</div>
	{#each countries as c (c.country)}
		<div class="row">
			<span class="name">{c.country}</span>
			<div class="track">
				<div class="fill" style="width: {(c.count / max) * 100}%"></div>
			</div>
			<span class="count">{c.count}</span>
		</div>
	{/each}
</div>
```

with:

```svelte
<div class="country-bar">
	<div class="eyebrow">Where it's coming from</div>
	{#if countries.length === 0}
		<div class="empty">No international traffic in this sample.</div>
	{:else}
		{#each countries as c (c.country)}
			<div class="row">
				<span class="name">{c.country}</span>
				<div class="track">
					<div class="fill" style="width: {(c.count / max) * 100}%"></div>
				</div>
				<span class="count">{c.count}</span>
			</div>
		{/each}
	{/if}
</div>
```

Add to the `<style>` block:

```css
	.empty {
		font-size: var(--text-label);
		color: var(--color-muted-2);
	}
```

- [ ] **Step 2: Add a waiting-for-events state to `Ticker.svelte`**

Replace the template:

```svelte
<div class="ticker">
	<div class="eyebrow">Ticker</div>
	{#each entries as entry, i (i)}
		<div class="row">
			<span class="time">{entry.time}</span>
			<span class="dot severity-{entry.severity}"></span>
			<span class="line">{entry.host} {entry.program}: {entry.message}</span>
		</div>
	{/each}
</div>
```

with:

```svelte
<div class="ticker">
	<div class="eyebrow">Ticker</div>
	{#if entries.length === 0}
		<div class="empty">Waiting for live events…</div>
	{:else}
		{#each entries as entry, i (i)}
			<div class="row">
				<span class="time">{entry.time}</span>
				<span class="dot severity-{entry.severity}"></span>
				<span class="line">{entry.host} {entry.program}: {entry.message}</span>
			</div>
		{/each}
	{/if}
</div>
```

Add to the `<style>` block:

```css
	.empty {
		font-size: var(--text-label);
		color: var(--color-muted-2);
	}
```

- [ ] **Step 3: Remove the Retention tile from `StatRow.svelte`**

Replace the template:

```svelte
<div class="stat-row">
	<div class="stat">
		<div class="eyebrow">Events 24h</div>
		<div class="value">{events.value}<span class="unit">{events.unit}</span></div>
	</div>
	<div class="stat">
		<div class="eyebrow">Open alerts</div>
		<div class="value critical">{openAlertCount}</div>
	</div>
	<div class="stat placeholder">
		<!-- No data source for retention figures in this sub-project yet — see design spec. -->
		<div class="eyebrow">Retention</div>
		<div class="value-small">not yet available</div>
	</div>
</div>
```

with:

```svelte
<div class="stat-row">
	<div class="stat">
		<div class="eyebrow">Events 24h</div>
		<div class="value">{events.value}<span class="unit">{events.unit}</span></div>
	</div>
	<div class="stat">
		<div class="eyebrow">Open alerts</div>
		<div class="value critical">{openAlertCount}</div>
	</div>
</div>
```

Remove the now-unused `.placeholder` and `.value-small` rules from the `<style>` block:

```css
	.placeholder {
		margin-left: auto;
		text-align: right;
	}
	.value-small {
		font-size: var(--text-table);
		color: var(--color-muted-2);
	}
```

- [ ] **Step 4: Manually verify in a real browser**

Using the same Playwright technique as prior tasks: confirm `CountryBar` shows its empty-state message when there's no country data (e.g. temporarily pass an empty array), confirm `Ticker` shows "Waiting for live events…" on initial load before any SSE message arrives, and confirm the Retention tile is gone from `StatRow` with the remaining two tiles laid out sensibly (no leftover right-aligned gap).

- [ ] **Step 5: Run the full siem-web test suite, lint, and type-check**

Run: `cd siem-web && pnpm exec vitest run && pnpm lint && pnpm exec svelte-check`
Expected: All PASS, no new type errors.

- [ ] **Step 6: Commit**

```bash
git add siem-web/src/lib/components/CountryBar.svelte siem-web/src/lib/components/Ticker.svelte \
  siem-web/src/lib/components/StatRow.svelte
git commit -m "Add empty states to CountryBar/Ticker; remove the exposed Retention placeholder"
```
