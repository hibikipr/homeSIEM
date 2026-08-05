# siem-web: Live tail screen — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship the Live tail screen (`/tail`) — a real-time SSE log stream with a 5,000-line
ring buffer, pause, auto-follow with scroll-detach, severity filter chips, and NDJSON
export — per `docs/superpowers/specs/2026-08-05-siem-web-live-tail-design.md`.

**Architecture:** Zero siem-api changes — the SSE feed (`GET /events/tail` via the existing
`/api/tail-proxy` same-origin proxy) and the footer's source count (`GET /sources`) already
exist. This is a pure siem-web addition: one new data-shaping module, one new interactive
component owning the buffer/pause/auto-follow/EventSource logic, and one new route
assembling it with a header (chips, pause, export, live indicator) and footer.

**Tech Stack:** SvelteKit 5 + TypeScript + Vitest, matching every prior siem-web
sub-project. No new dependencies.

## Global Constraints

- `LogEntry` (existing type in `siem-web/src/lib/server/siemApiClient.ts`:
  `{ Timestamp: string; Labels: Record<string, string>; Line: string }`) is the entry type
  for this whole feature — do not define a parallel/duplicate type.
- The ring buffer is capped at 5,000 entries, oldest dropped first, and keeps growing
  regardless of pause state — pause only freezes what's *rendered*, never what's buffered.
- Severity filtering and NDJSON export are pure functions in `siem-web/src/lib/tail.ts`,
  operating on already-buffered data — no server-side filtering, no new siem-api endpoint.
- The 8-value fixed severity set is `emerg, alert, crit, err, warning, notice, info, debug`
  (RFC 5424) — not the narrower critical/warning/info set used elsewhere in this codebase.
- A `Set<string>`/`SvelteSet<string>` used as reactive Svelte state must be a `SvelteSet`
  from `svelte/reactivity` (not a plain `Set` reassigned on every mutation) — this codebase
  already established that convention for exactly this pattern in the Sources
  sub-project's `UnclaimedSenders.svelte`.
- "Search this" renders disabled with a tooltip this pass — the Search screen doesn't
  exist yet. Do not wire it to a real route.
- No unit tests for `TailViewport.svelte`/`+page.svelte` — presentational/DOM-scroll-driven,
  matching the established convention (Wall/Alerts/Sources all skip component tests).

---

### Task 1: siem-web — `tail.ts` data-shaping helpers

**Files:**
- Create: `siem-web/src/lib/tail.ts`
- Create: `siem-web/src/lib/tail.test.ts`

**Interfaces:**
- Consumes: `LogEntry` from `siem-web/src/lib/server/siemApiClient.ts` (existing).
- Produces: `SYSLOG_SEVERITIES` (readonly string array), `filterBySeverity(entries,
  activeSeverities)`, `serializeNdjson(entries)`, `severityColor(severity)` — all consumed
  by Task 2's `TailViewport.svelte` and Task 3's `+page.svelte`.

- [ ] **Step 1: Write the failing tests**

Create `siem-web/src/lib/tail.test.ts`:

```ts
import { describe, it, expect } from 'vitest';
import { filterBySeverity, serializeNdjson, severityColor } from './tail';
import type { LogEntry } from './server/siemApiClient';

function fakeEntry(overrides: Partial<LogEntry> = {}): LogEntry {
	return {
		Timestamp: '2026-08-05T00:00:00Z',
		Labels: { severity: 'info', host: 'h', program: 'p' },
		Line: 'hello',
		...overrides
	};
}

describe('filterBySeverity', () => {
	it('keeps only entries whose severity is in the active set', () => {
		const critEntry = fakeEntry({ Labels: { severity: 'crit' } });
		const debugEntry = fakeEntry({ Labels: { severity: 'debug' } });

		const result = filterBySeverity([critEntry, debugEntry], new Set(['crit']));

		expect(result).toEqual([critEntry]);
	});

	it('treats a missing severity label as info', () => {
		const noSeverity = fakeEntry({ Labels: {} });

		expect(filterBySeverity([noSeverity], new Set(['info']))).toEqual([noSeverity]);
		expect(filterBySeverity([noSeverity], new Set(['crit']))).toEqual([]);
	});

	it('returns an empty array when nothing matches', () => {
		expect(filterBySeverity([fakeEntry({ Labels: { severity: 'debug' } })], new Set(['crit']))).toEqual(
			[]
		);
	});
});

describe('serializeNdjson', () => {
	it('serializes entries as newline-delimited JSON matching the wire shape', () => {
		const entries = [fakeEntry({ Line: 'a' }), fakeEntry({ Line: 'b' })];

		const result = serializeNdjson(entries);
		const lines = result.split('\n');

		expect(lines).toHaveLength(2);
		expect(JSON.parse(lines[0])).toEqual(entries[0]);
		expect(JSON.parse(lines[1])).toEqual(entries[1]);
	});

	it('returns an empty string for no entries', () => {
		expect(serializeNdjson([])).toBe('');
	});
});

describe('severityColor', () => {
	it('maps the three most-severe syslog levels to the critical token', () => {
		expect(severityColor('emerg')).toBe('var(--color-severity-critical)');
		expect(severityColor('alert')).toBe('var(--color-severity-critical)');
		expect(severityColor('crit')).toBe('var(--color-severity-critical)');
	});

	it('maps warning and debug to their own tokens', () => {
		expect(severityColor('warning')).toBe('var(--color-severity-warning)');
		expect(severityColor('debug')).toBe('var(--color-muted-2)');
	});

	it('falls back to the info token for an unrecognized severity', () => {
		expect(severityColor('bogus')).toBe('var(--color-severity-info)');
	});
});
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd siem-web && npm run test:unit -- --run tail.test`
Expected: FAIL — `Cannot find module './tail'`.

- [ ] **Step 3: Implement the helpers**

Create `siem-web/src/lib/tail.ts`:

```ts
import type { LogEntry } from './server/siemApiClient';

export const SYSLOG_SEVERITIES = [
	'emerg',
	'alert',
	'crit',
	'err',
	'warning',
	'notice',
	'info',
	'debug'
] as const;

export function filterBySeverity(entries: LogEntry[], activeSeverities: Set<string>): LogEntry[] {
	return entries.filter((e) => activeSeverities.has(e.Labels.severity ?? 'info'));
}

export function serializeNdjson(entries: LogEntry[]): string {
	return entries.map((e) => JSON.stringify(e)).join('\n');
}

const SEVERITY_COLOR_TOKENS: Record<string, string> = {
	emerg: 'var(--color-severity-critical)',
	alert: 'var(--color-severity-critical)',
	crit: 'var(--color-severity-critical)',
	err: 'var(--color-severity-error)',
	warning: 'var(--color-severity-warning)',
	notice: 'var(--color-severity-notice)',
	info: 'var(--color-severity-info)',
	debug: 'var(--color-muted-2)'
};

export function severityColor(severity: string): string {
	return SEVERITY_COLOR_TOKENS[severity] ?? SEVERITY_COLOR_TOKENS.info;
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd siem-web && npm run test:unit -- --run tail.test`
Expected: PASS (8 tests).

- [ ] **Step 5: Commit**

```bash
git add siem-web/src/lib/tail.ts siem-web/src/lib/tail.test.ts
git commit -m "Add tail.ts: severity filtering, NDJSON export, severity color mapping"
```

---

### Task 2: siem-web — `TailViewport.svelte`

**Files:**
- Create: `siem-web/src/lib/components/TailViewport.svelte`

**Interfaces:**
- Consumes: `LogEntry` (`siemApiClient.ts`), `filterBySeverity`/`severityColor` (Task 1).
- Produces: a component with four `$bindable` props — `activeSeverities: SvelteSet<string>`,
  `paused: boolean`, `buffer: LogEntry[]`, `connected: boolean` — all owned/initialized by
  Task 3's `+page.svelte` and read/mutated here. `buffer` is written by this component
  (every incoming SSE message appends to it); `activeSeverities` and `paused` are written
  by Task 3's header controls and read here; `connected` is written here and read by Task
  3's header live indicator.

No test file — presentational/DOM-scroll-driven component, per this project's established
convention and this plan's Global Constraints.

- [ ] **Step 1: Implement the component**

Create `siem-web/src/lib/components/TailViewport.svelte`:

```svelte
<script lang="ts">
	import { resolve } from '$app/paths';
	import type { LogEntry } from '$lib/server/siemApiClient';
	import { filterBySeverity, severityColor } from '$lib/tail';

	const MAX_BUFFER = 5000;

	let {
		activeSeverities = $bindable(),
		paused = $bindable(false),
		buffer = $bindable([]),
		connected = $bindable(true)
	}: {
		activeSeverities: Set<string>;
		paused: boolean;
		buffer: LogEntry[];
		connected: boolean;
	} = $props();

	let rendered = $state<LogEntry[]>([]);
	let autoFollow = $state(true);
	let newSinceDetach = $state(0);
	let viewportEl: HTMLDivElement;

	function queueScrollToBottom() {
		requestAnimationFrame(() => {
			if (viewportEl) viewportEl.scrollTop = viewportEl.scrollHeight;
		});
	}

	function appendEntry(entry: LogEntry) {
		buffer = [...buffer, entry].slice(-MAX_BUFFER);
		if (paused) return;
		if (!activeSeverities.has(entry.Labels.severity ?? 'info')) return;

		rendered = [...rendered, entry];
		if (autoFollow) {
			queueScrollToBottom();
		} else {
			newSinceDetach += 1;
		}
	}

	// Re-derive the full rendered list from the buffer whenever the severity
	// filter changes or the view un-pauses — new-message-driven updates are
	// handled incrementally by appendEntry above, so this effect intentionally
	// does not depend on `buffer` (that would double-process every message).
	$effect(() => {
		activeSeverities;
		paused;
		if (!paused) {
			rendered = filterBySeverity(buffer, activeSeverities);
			if (autoFollow) queueScrollToBottom();
		}
	});

	$effect(() => {
		const source = new EventSource(resolve('/api/tail-proxy'));
		source.onopen = () => {
			connected = true;
		};
		source.onerror = () => {
			connected = false;
		};
		source.onmessage = (event) => {
			try {
				const entry: LogEntry = JSON.parse(event.data);
				appendEntry(entry);
			} catch {
				// malformed SSE payload — skip this line rather than breaking the stream
			}
		};
		return () => source.close();
	});

	function handleScroll() {
		if (!viewportEl) return;
		const atBottom = viewportEl.scrollHeight - viewportEl.scrollTop - viewportEl.clientHeight < 4;
		if (atBottom) {
			autoFollow = true;
			newSinceDetach = 0;
		} else {
			autoFollow = false;
		}
	}

	function reattach() {
		autoFollow = true;
		newSinceDetach = 0;
		queueScrollToBottom();
	}
</script>

<div class="viewport-wrap">
	<div class="viewport" bind:this={viewportEl} onscroll={handleScroll}>
		<table>
			<thead>
				<tr>
					<th class="col-time">Time</th>
					<th class="col-severity"></th>
					<th class="col-host">Host</th>
					<th class="col-program">Program</th>
					<th class="col-facility">Facility</th>
					<th>Message</th>
				</tr>
			</thead>
			<tbody>
				{#each rendered as entry, i (i)}
					<tr>
						<td class="col-time mono">{entry.Timestamp}</td>
						<td class="col-severity">
							<span
								class="dot"
								style:background={severityColor(entry.Labels.severity ?? 'info')}
							></span>
						</td>
						<td class="col-host mono">{entry.Labels.host ?? ''}</td>
						<td class="col-program mono">{entry.Labels.program ?? ''}</td>
						<td class="col-facility mono">{entry.Labels.facility ?? ''}</td>
						<td class="mono message">{entry.Line}</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
	{#if !autoFollow && newSinceDetach > 0}
		<button class="new-pill" onclick={reattach}>{newSinceDetach} new</button>
	{/if}
</div>

<style>
	.viewport-wrap {
		position: relative;
	}
	.viewport {
		background: var(--color-bg-alt);
		box-shadow: inset var(--shadow-flat);
		border-radius: var(--radius-default);
		height: 60vh;
		overflow-y: auto;
	}
	table {
		width: 100%;
		border-collapse: collapse;
		font-size: 12px;
		line-height: 2.05;
	}
	thead th {
		position: sticky;
		top: 0;
		background: var(--color-surface-3);
		text-align: left;
		font-size: var(--text-eyebrow);
		text-transform: uppercase;
		color: var(--color-muted-2);
		padding: var(--space-2) var(--space-3);
	}
	tbody td {
		padding: 0 var(--space-3);
		white-space: nowrap;
	}
	.mono {
		font-family: var(--font-mono);
	}
	.col-time {
		width: 150px;
		color: var(--color-muted);
	}
	.col-severity {
		width: 16px;
	}
	.col-host {
		width: 96px;
	}
	.col-program {
		width: 86px;
		color: var(--color-accent-light);
	}
	.col-facility {
		width: 64px;
		color: var(--color-muted-2);
	}
	.message {
		white-space: normal;
		word-break: break-word;
		color: var(--color-text-2);
	}
	.dot {
		display: inline-block;
		width: 8px;
		height: 8px;
		border-radius: 50%;
	}
	.new-pill {
		position: absolute;
		bottom: var(--space-4);
		left: 50%;
		transform: translateX(-50%);
		background: var(--color-accent);
		color: var(--color-bg);
		border: none;
		border-radius: 999px;
		padding: var(--space-1) var(--space-4);
		font-size: var(--text-label);
		font-weight: 500;
		cursor: pointer;
		box-shadow: var(--shadow-raised);
	}
</style>
```

- [ ] **Step 2: Typecheck**

Run: `cd siem-web && npm run check`
Expected: no new errors from this file. (`activeSeverities`'s bindable type is declared as
`Set<string>` here since `SvelteSet` structurally satisfies `Set`'s interface — Task 3
passes a real `SvelteSet` instance in, which is what makes in-place `.add`/`.delete`
mutations from Task 3's chip toggles reactive.)

- [ ] **Step 3: Commit**

```bash
git add siem-web/src/lib/components/TailViewport.svelte
git commit -m "Add TailViewport: ring buffer, pause, auto-follow, EventSource subscription"
```

---

### Task 3: siem-web — `/tail` route assembly

**Files:**
- Create: `siem-web/src/routes/tail/+page.server.ts`
- Create: `siem-web/src/routes/tail/page.server.test.ts`
- Create: `siem-web/src/routes/tail/+page.svelte`
- Modify: `siem-web/src/lib/components/Nav.svelte`

**Interfaces:**
- Consumes: `SiemApiClient.getSources` (existing), `SYSLOG_SEVERITIES`/`filterBySeverity`/
  `serializeNdjson`/`severityColor` (Task 1), `TailViewport.svelte`'s four bindable props
  (Task 2).

- [ ] **Step 1: Write the failing load-function tests**

Create `siem-web/src/routes/tail/page.server.test.ts`:

```ts
import { describe, it, expect, vi } from 'vitest';
import { load } from './+page.server';
import * as siemApiClientModule from '$lib/server/siemApiClient';
import { SiemApiError } from '$lib/server/siemApiClient';

vi.mock('$env/dynamic/private', () => ({ env: { API_URL: 'http://siem-api:8080' } }));

vi.mock('$lib/server/siemApiClient', async (importOriginal) => {
	const actual = await importOriginal<typeof import('$lib/server/siemApiClient')>();
	return { ...actual, SiemApiClient: vi.fn() };
});

describe('Live tail load', () => {
	it('returns the source count', async () => {
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return {
				getSources: vi.fn().mockResolvedValue([{ id: 1 }, { id: 2 }, { id: 3 }])
			};
		});

		const result = (await load({
			locals: { sessionToken: 'token-123' }
		} as never)) as Exclude<Awaited<ReturnType<typeof load>>, void>;

		expect(result.sourceCount).toBe(3);
	});

	it('redirects to /auth/logout on a 401/403 from siem-api', async () => {
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return {
				getSources: vi.fn().mockRejectedValue(new SiemApiError(401, 'invalid session'))
			};
		});

		await expect(load({ locals: { sessionToken: 'stale-token' } } as never)).rejects.toMatchObject({
			status: 302,
			location: '/auth/logout'
		});
	});

	it('surfaces a 502 when siem-api fails for a reason other than auth', async () => {
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return {
				getSources: vi.fn().mockRejectedValue(new SiemApiError(500, 'boom'))
			};
		});

		await expect(load({ locals: { sessionToken: 'token-123' } } as never)).rejects.toMatchObject({
			status: 502
		});
	});
});
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd siem-web && npm run test:unit -- --run routes/tail`
Expected: FAIL — `Cannot find module './+page.server'`.

- [ ] **Step 3: Implement the load function**

Create `siem-web/src/routes/tail/+page.server.ts`:

```ts
import { error, redirect } from '@sveltejs/kit';
import { env } from '$env/dynamic/private';
import type { PageServerLoad } from './$types';
import { SiemApiClient, SiemApiError } from '$lib/server/siemApiClient';

export const load: PageServerLoad = async ({ locals }) => {
	const client = new SiemApiClient({ baseUrl: env.API_URL as string });
	const token = locals.sessionToken as string;

	let sources;
	try {
		sources = await client.getSources(token);
	} catch (err) {
		if (err instanceof SiemApiError) {
			if (err.status === 401 || err.status === 403) {
				redirect(302, '/auth/logout');
			}
			error(502, 'siem-api unavailable');
		}
		throw err;
	}

	return { sourceCount: sources.length };
};
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd siem-web && npm run test:unit -- --run routes/tail`
Expected: PASS.

- [ ] **Step 5: Implement the page**

Create `siem-web/src/routes/tail/+page.svelte`:

```svelte
<script lang="ts">
	import { SvelteSet } from 'svelte/reactivity';
	import TailViewport from '$lib/components/TailViewport.svelte';
	import type { LogEntry } from '$lib/server/siemApiClient';
	import { SYSLOG_SEVERITIES, filterBySeverity, serializeNdjson } from '$lib/tail';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	let activeSeverities = $state(new SvelteSet<string>(SYSLOG_SEVERITIES));
	let paused = $state(false);
	let buffer = $state<LogEntry[]>([]);
	let connected = $state(true);

	function toggleSeverity(sev: string) {
		if (activeSeverities.has(sev)) {
			activeSeverities.delete(sev);
		} else {
			activeSeverities.add(sev);
		}
	}

	function exportBuffer() {
		const filtered = filterBySeverity(buffer, activeSeverities);
		const ndjson = serializeNdjson(filtered);
		const blob = new Blob([ndjson], { type: 'application/x-ndjson' });
		const url = URL.createObjectURL(blob);
		const a = document.createElement('a');
		a.href = url;
		a.download = `live-tail-${new Date().toISOString()}.ndjson`;
		a.click();
		URL.revokeObjectURL(url);
	}
</script>

<div class="tail-screen">
	<header class="tail-header">
		<div class="left">
			<h1>Live tail</h1>
			<span class="status" class:disconnected={!connected}>
				<span class="dot"></span>
				{connected ? 'following' : 'disconnected · retrying'}
			</span>
		</div>
		<div class="chips">
			{#each SYSLOG_SEVERITIES as sev (sev)}
				<button
					class="chip"
					class:active={activeSeverities.has(sev)}
					onclick={() => toggleSeverity(sev)}
				>
					{sev}
				</button>
			{/each}
		</div>
		<div class="actions">
			<button class="action" onclick={() => (paused = !paused)}>
				{paused ? 'Resume' : 'Pause'}
			</button>
			<button class="action" onclick={exportBuffer}>Export</button>
			<button class="action" disabled title="Search screen isn't built yet">Search this</button>
		</div>
	</header>

	<TailViewport bind:activeSeverities bind:paused bind:buffer bind:connected />

	<footer class="tail-footer">
		<span>Buffer 5,000 lines · Wrap off · Sources: all ({data.sourceCount})</span>
		<span class="mono">udp/514 · tcp/601 · tls/6514</span>
	</footer>
</div>

<style>
	.tail-screen {
		padding: var(--space-5) var(--space-6);
	}
	.tail-header {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--space-4);
		margin-bottom: var(--space-4);
	}
	.left {
		display: flex;
		align-items: center;
		gap: var(--space-3);
	}
	h1 {
		font-size: var(--text-page-title);
		margin: 0;
	}
	.status {
		display: flex;
		align-items: center;
		gap: var(--space-1);
		font-size: var(--text-label);
		color: var(--color-severity-healthy);
	}
	.status.disconnected {
		color: var(--color-severity-warning);
	}
	.status .dot {
		width: 6px;
		height: 6px;
		border-radius: 50%;
		background: currentColor;
	}
	.chips {
		display: flex;
		gap: var(--space-2);
		flex-wrap: wrap;
	}
	.chip {
		background: transparent;
		box-shadow: inset 0 0 0 1px var(--color-line-2);
		color: var(--color-muted);
		border: none;
		border-radius: var(--radius-sm);
		padding: var(--space-1) var(--space-3);
		font-size: var(--text-label);
		cursor: pointer;
	}
	.chip.active {
		background: var(--color-accent-tint);
		box-shadow: inset 0 0 0 1px var(--color-accent-deep);
		color: var(--color-accent-lighter);
	}
	.actions {
		display: flex;
		gap: var(--space-2);
		margin-left: auto;
	}
	.action {
		background: var(--color-surface-2);
		color: var(--color-text);
		border: none;
		border-radius: var(--radius-sm);
		padding: var(--space-1) var(--space-3);
		font-size: var(--text-label);
		cursor: pointer;
	}
	.action:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}
	.tail-footer {
		display: flex;
		justify-content: space-between;
		font-size: var(--text-label);
		color: var(--color-muted-2);
		background: var(--color-surface-3);
		padding: var(--space-2) var(--space-3);
		margin-top: var(--space-2);
		border-radius: var(--radius-sm);
	}
	.mono {
		font-family: var(--font-mono);
	}
</style>
```

- [ ] **Step 6: Drop the now-unnecessary `Pathname` assertion in `Nav.svelte`**

In `siem-web/src/lib/components/Nav.svelte`, the `/tail` route now exists, so drop its
forward-declaration cast (per the file's own comment, "drop each assertion as its route
lands"). Change:

```ts
		{ label: 'Live tail', href: '/tail' as Pathname },
```

to:

```ts
		{ label: 'Live tail', href: '/tail' },
```

- [ ] **Step 7: Typecheck, lint, and run the full test suite**

Run: `cd siem-web && npm run check && npm run lint && npm run test:unit -- --run`
Expected: no new type errors, no lint errors, all tests (existing + this plan's) pass.

- [ ] **Step 8: Manual verification**

Run `cd siem-web && npm run dev`, sign in if possible, and visit `/tail`. Since this
environment likely has no real siem-api/siem-ingest producing live traffic, confirm at
minimum: the page renders without crashing, the severity chips toggle their active state
on click, Pause/Resume toggles its label, and the live indicator shows "disconnected ·
retrying" if the SSE connection can't actually reach a live backend (expected in this
environment) rather than crashing. Note in the task report whichever is actually observed.

- [ ] **Step 9: Commit**

```bash
git add siem-web/src/routes/tail siem-web/src/lib/components/Nav.svelte
git commit -m "Assemble the Live tail screen and wire the nav link"
```
