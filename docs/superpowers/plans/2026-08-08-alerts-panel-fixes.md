# Alerts Panel Fixes and Gaps Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix the severity-vocabulary color bug on `/alerts`, correct the Rules-tab
empty-detail text, add empty-state messaging to the alert/rule lists, and add a
"+ New rule" creation flow with prepopulated templates covering all three backend
rule shapes.

**Architecture:** Four frontend-only changes to `siem-web`. No backend changes —
`siem-api`'s `POST /rules` and its `threshold`/`absence`/`first_seen` evaluators
already support everything this plan exposes in the UI.

**Tech Stack:** SvelteKit, Svelte 5 runes, TypeScript, Vitest, Playwright.

## Global Constraints

- The only real alert severities are `info` / `warning` / `critical` — never
  introduce or preserve `low`/`medium`/`high` selectors; they can never match a
  real `alert.severity` value and are dead code.
- No backend (`siem-api`) changes in this plan — every field this plan adds to the
  rule-creation form already round-trips through `POST /rules` today.
- This project has no Svelte component test framework (an established convention,
  not a gap). Pure logic (data tables, string parsing) belongs in a plain `.ts`
  module under `siem-web/src/lib/` with Vitest coverage, matching existing files
  like `wall.ts`, `eventsOverTime.ts`, `alerts.ts`. UI-only changes are verified via
  `pnpm lint` / `pnpm exec svelte-check` plus manual or Playwright interaction, not
  new unit tests.
- Use Svelte 5 runes (`$state`, `$props`, `$derived`) and `resolve()` from
  `$app/paths` for internal links, matching every existing component in this
  directory.
- After a successful mutation that other parts of the page depend on, call
  `invalidateAll()` from `$app/navigation` to refresh loader data — the pattern
  already used by `routes/alerts/+page.svelte`'s SSE handler.
- Any new Playwright e2e test must read `SESSION_SECRET` from `siem-web/.env` at
  runtime to mint a session cookie — never hardcode a secret in a test file. Follow
  `siem-web/e2e/nav-account-menu.e2e.ts` exactly for this pattern.
- Before every commit: `pnpm exec vitest run`, `pnpm lint`, and
  `pnpm exec svelte-check` must all be clean (project baseline as of PR #31:
  139/139 tests, 0 lint errors, 0 type errors — svelte-check warning count may
  fluctuate slightly but must not regress above the pre-existing baseline you
  observe when you start).

---

### Task 1: Fix severity-vocabulary bug in `AlertRow.svelte` and `AlertDetail.svelte`

**Files:**
- Modify: `siem-web/src/lib/components/AlertRow.svelte`
- Modify: `siem-web/src/lib/components/AlertDetail.svelte`

**Interfaces:**
- Consumes: nothing new — both components already receive `alert: AlertResponse`
  with `alert.severity: string` (real values `info`/`warning`/`critical`).
- Produces: nothing consumed by later tasks.

**Background:** both files style severity two ways: (1) a base rule that defaults
to `--color-severity-critical`, with an explicit `.severity-warning` override but
a `.severity-low`/`.severity-medium` pair that can never match any real
`alert.severity` value (dead code — the real value would be `info`). In
`AlertRow.svelte` there's a second instance of the same bug: `.header`'s text
color is hardcoded to `--color-severity-critical` with **no per-severity override
at all**, so every alert's eyebrow text renders critical-red regardless of actual
severity.

- [ ] **Step 1: Fix `AlertRow.svelte`**

Replace the `<style>` block's severity-related rules. Current (buggy):

```css
.row {
	display: block;
	background: var(--color-surface-2);
	border-radius: var(--radius-default);
	padding: var(--space-4);
	border-left: 3px solid var(--color-severity-critical);
	text-decoration: none;
	color: inherit;
}
.row.severity-warning {
	border-left-color: var(--color-severity-warning);
}
.row.severity-low,
.row.severity-medium {
	border-left-color: var(--color-severity-info);
}
.row.selected {
	background: var(--color-accent-tint);
	box-shadow: 0 0 0 1px var(--color-accent-deep);
}
.header {
	display: flex;
	justify-content: space-between;
	font-size: var(--text-eyebrow);
	text-transform: uppercase;
	color: var(--color-severity-critical);
}
```

Replace with:

```css
.row {
	display: block;
	background: var(--color-surface-2);
	border-radius: var(--radius-default);
	padding: var(--space-4);
	border-left: 3px solid var(--color-severity-info);
	text-decoration: none;
	color: inherit;
}
.row.severity-warning {
	border-left-color: var(--color-severity-warning);
}
.row.severity-critical {
	border-left-color: var(--color-severity-critical);
}
.row.selected {
	background: var(--color-accent-tint);
	box-shadow: 0 0 0 1px var(--color-accent-deep);
}
.header {
	display: flex;
	justify-content: space-between;
	font-size: var(--text-eyebrow);
	text-transform: uppercase;
	color: var(--color-severity-info);
}
.row.severity-warning .header {
	color: var(--color-severity-warning);
}
.row.severity-critical .header {
	color: var(--color-severity-critical);
}
```

Everything else in the file (script block, markup, remaining styles) is unchanged.

- [ ] **Step 2: Fix `AlertDetail.svelte`**

Replace this block in the `<style>` section. Current (buggy):

```css
.eyebrow {
	font-size: var(--text-eyebrow);
	text-transform: uppercase;
	color: var(--color-severity-critical);
}
.eyebrow.severity-warning {
	color: var(--color-severity-warning);
}
.eyebrow.severity-low,
.eyebrow.severity-medium {
	color: var(--color-severity-info);
}
```

Replace with:

```css
.eyebrow {
	font-size: var(--text-eyebrow);
	text-transform: uppercase;
	color: var(--color-severity-info);
}
.eyebrow.severity-warning {
	color: var(--color-severity-warning);
}
.eyebrow.severity-critical {
	color: var(--color-severity-critical);
}
```

Everything else in the file is unchanged.

- [ ] **Step 3: Verify**

Run:
```bash
cd siem-web && pnpm lint && pnpm exec svelte-check
```
Expected: clean, same or fewer warnings than the pre-existing baseline.

If the running environment has real alerts of more than one severity (check via
`GET /alerts?state=open` through the app, or by looking at what's already visible
on `/alerts` in a browser/Playwright session with a minted cookie per
`siem-web/e2e/nav-account-menu.e2e.ts`'s pattern), visually confirm: a `warning`
alert's left border and eyebrow text are both amber/yellow, a `critical` alert's
are both red, and (if any `info` alert exists) its are both the info color — no
alert should show a red eyebrow with a non-red border, which was the bug. If no
real alerts of mixed severity are available in this environment, note that in your
report and rely on the CSS review above — this is not a blocking condition for the
task, since the fix is a direct, mechanical correchtion of a selector/color
mismatch.

- [ ] **Step 4: Commit**

```bash
git add siem-web/src/lib/components/AlertRow.svelte siem-web/src/lib/components/AlertDetail.svelte
git commit -m "Fix Alerts severity colors: dead low/medium selectors and always-critical eyebrow text"
```

---

### Task 2: Fix Rules-tab empty-detail text and add empty-state messages

**Files:**
- Modify: `siem-web/src/routes/alerts/+page.svelte`
- Modify: `siem-web/src/lib/components/AlertInbox.svelte`

**Interfaces:**
- Consumes: nothing new.
- Produces: nothing consumed by later tasks (Task 4 modifies `AlertInbox.svelte`
  again afterward, on top of this task's changes — read the file's current state
  before editing).

- [ ] **Step 1: Fix the empty-detail text in `+page.svelte`**

Current:

```svelte
	{#if data.selectedAlert && data.stats}
		<AlertDetail
			alert={data.selectedAlert}
			samples={data.selectedSamples}
			stats={data.stats}
			rule={data.rules.find((r) => r.id === data.selectedAlert?.rule_id)}
		/>
	{:else if data.selectedRule}
		<RuleDetail rule={data.selectedRule} />
	{:else}
		<div class="empty">Select an alert to see details.</div>
	{/if}
```

Replace the final branch:

```svelte
	{#if data.selectedAlert && data.stats}
		<AlertDetail
			alert={data.selectedAlert}
			samples={data.selectedSamples}
			stats={data.stats}
			rule={data.rules.find((r) => r.id === data.selectedAlert?.rule_id)}
		/>
	{:else if data.selectedRule}
		<RuleDetail rule={data.selectedRule} />
	{:else}
		<div class="empty">
			{data.tab === 'rules' ? 'Select a rule to see details.' : 'Select an alert to see details.'}
		</div>
	{/if}
```

- [ ] **Step 2: Add empty-state messages in `AlertInbox.svelte`**

Current `.rows` block:

```svelte
	<div class="rows">
		{#if tab === 'rules'}
			{#each rules as rule (rule.id)}
				<RuleRow {rule} selected={selectedId === rule.id} />
			{/each}
		{:else}
			{#each alerts as alert (alert.id)}
				<AlertRow
					{alert}
					ruleName={ruleNames.get(alert.rule_id) ?? `rule #${alert.rule_id}`}
					selected={selectedId === alert.id}
				/>
			{/each}
		{/if}
	</div>
```

Replace with:

```svelte
	<div class="rows">
		{#if tab === 'rules'}
			{#each rules as rule (rule.id)}
				<RuleRow {rule} selected={selectedId === rule.id} />
			{:else}
				<div class="empty-list">No rules configured yet.</div>
			{/each}
		{:else}
			{#each alerts as alert (alert.id)}
				<AlertRow
					{alert}
					ruleName={ruleNames.get(alert.rule_id) ?? `rule #${alert.rule_id}`}
					selected={selectedId === alert.id}
				/>
			{:else}
				<div class="empty-list">
					{tab === 'acked' ? 'No acknowledged alerts.' : 'No open alerts.'}
				</div>
			{/each}
		{/if}
	</div>
```

Add this rule to the `<style>` block (alongside the existing `.rows` rule):

```css
.empty-list {
	color: var(--color-muted-2);
	font-size: var(--text-table);
	padding: var(--space-4);
	text-align: center;
}
```

- [ ] **Step 3: Verify**

Run:
```bash
cd siem-web && pnpm lint && pnpm exec svelte-check
```
Expected: clean.

Visually confirm (dev server or Playwright with a minted session cookie): the
Acked tab (commonly empty in a fresh environment) shows "No acknowledged alerts."
instead of a blank list; switching to the Rules tab with nothing selected shows
"Select a rule to see details." in the right-hand panel, not "Select an alert...".

- [ ] **Step 4: Commit**

```bash
git add siem-web/src/routes/alerts/+page.svelte siem-web/src/lib/components/AlertInbox.svelte
git commit -m "Alerts: fix Rules-tab empty-detail text, add empty-state messages to lists"
```

---

### Task 3: Generalize `RuleFromEventForm.svelte` with a rule-type selector and templates

**Files:**
- Create: `siem-web/src/lib/ruleTemplates.ts`
- Create: `siem-web/src/lib/ruleTemplates.test.ts`
- Modify: `siem-web/src/lib/components/RuleFromEventForm.svelte`

**Interfaces:**
- Consumes: nothing new.
- Produces: `RULE_TEMPLATES: RuleTemplate[]` and `parseGroupBy(input: string): string[]`
  from `$lib/ruleTemplates`, and `RuleShape = 'threshold' | 'absence' | 'first_seen'`
  — used only inside `RuleFromEventForm.svelte` in this plan, but exported in case a
  later task needs them. `RuleFromEventForm.svelte`'s public props are unchanged:
  `defaultName: string`, `defaultLogql: string`, `onClose: () => void`. Task 4
  imports and renders this component with no new props.

**Background — verified backend behavior (do not re-derive; these are facts about
`siem-api`, not something to guess):**

| Shape | Uses `logql`? | Uses `window_sec`? | Uses `threshold`? | Uses `group_by`? |
|---|---|---|---|---|
| `threshold` | yes | yes | yes | yes (optional) |
| `first_seen` | yes | yes (lookback window) | no | yes (groups events; a not-seen-before value in this grouping fires) |
| `absence` | no (evaluator never reads `rule.LogQL`) | no (staleness comes from `store.Source.HeartbeatSec`, not `rule.WindowSec`) | no | no |

`POST /rules` (`siem-api/internal/api/rules.go`, `handleCreateRule`) does no field
validation, so sending unused fields for a given shape is harmless.

- [ ] **Step 1: Write the failing test for `ruleTemplates.ts`**

Create `siem-web/src/lib/ruleTemplates.test.ts`:

```typescript
import { describe, it, expect } from 'vitest';
import { RULE_TEMPLATES, parseGroupBy } from './ruleTemplates';

describe('RULE_TEMPLATES', () => {
	it('has five templates covering all three rule shapes', () => {
		expect(RULE_TEMPLATES).toHaveLength(5);
		const shapes = new Set(RULE_TEMPLATES.map((t) => t.shape));
		expect(shapes).toEqual(new Set(['threshold', 'absence', 'first_seen']));
	});

	it('every template has a non-empty label and name', () => {
		for (const template of RULE_TEMPLATES) {
			expect(template.label.length).toBeGreaterThan(0);
			expect(template.name.length).toBeGreaterThan(0);
		}
	});

	it('the absence template does not rely on logql', () => {
		const source_quiet = RULE_TEMPLATES.find((t) => t.shape === 'absence');
		expect(source_quiet?.logql).toBe('');
	});
});

describe('parseGroupBy', () => {
	it('splits a comma-separated list and trims whitespace', () => {
		expect(parseGroupBy('a, b,c')).toEqual(['a', 'b', 'c']);
	});

	it('returns an empty array for a blank string', () => {
		expect(parseGroupBy('')).toEqual([]);
		expect(parseGroupBy('   ')).toEqual([]);
	});

	it('filters out empty entries from trailing or doubled commas', () => {
		expect(parseGroupBy('a,,b,')).toEqual(['a', 'b']);
	});

	it('returns a single-element array for one field name', () => {
		expect(parseGroupBy('source')).toEqual(['source']);
	});
});
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd siem-web && pnpm exec vitest run ruleTemplates`
Expected: FAIL — `Cannot find module './ruleTemplates'` (the module doesn't exist yet).

- [ ] **Step 3: Implement `ruleTemplates.ts`**

Create `siem-web/src/lib/ruleTemplates.ts`:

```typescript
export type RuleShape = 'threshold' | 'absence' | 'first_seen';
export type RuleSeverity = 'info' | 'warning' | 'critical';

export type RuleTemplate = {
	label: string;
	name: string;
	shape: RuleShape;
	logql: string;
	windowSec: number;
	threshold: number;
	groupBy: string;
	severity: RuleSeverity;
};

export const RULE_TEMPLATES: RuleTemplate[] = [
	{
		label: 'Repeated critical events from one source',
		name: 'critical-burst',
		shape: 'threshold',
		logql: '{job="siem", severity="critical"}',
		windowSec: 300,
		threshold: 5,
		groupBy: 'source',
		severity: 'critical'
	},
	{
		label: 'VPN connection',
		name: 'vpn-connect',
		shape: 'threshold',
		logql: '{job="siem"} |= "Connected to VPN"',
		windowSec: 60,
		threshold: 1,
		groupBy: '',
		severity: 'info'
	},
	{
		label: 'Admin accessed UniFi OS',
		name: 'admin-access',
		shape: 'threshold',
		logql: '{job="siem"} |= "Admin Accessed UniFi OS"',
		windowSec: 60,
		threshold: 1,
		groupBy: '',
		severity: 'warning'
	},
	{
		label: 'Source went quiet',
		name: 'source-quiet',
		shape: 'absence',
		logql: '',
		windowSec: 60,
		threshold: 1,
		groupBy: '',
		severity: 'warning'
	},
	{
		label: 'New source seen',
		name: 'new-source',
		shape: 'first_seen',
		logql: '{job="siem"}',
		windowSec: 86400,
		threshold: 1,
		groupBy: 'source',
		severity: 'info'
	}
];

export function parseGroupBy(input: string): string[] {
	return input
		.split(',')
		.map((s) => s.trim())
		.filter((s) => s.length > 0);
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd siem-web && pnpm exec vitest run ruleTemplates`
Expected: PASS, 7 tests.

- [ ] **Step 5: Rewrite `RuleFromEventForm.svelte`**

Replace the entire file content:

```svelte
<script lang="ts">
	import { RULE_TEMPLATES, parseGroupBy, type RuleShape, type RuleSeverity } from '$lib/ruleTemplates';

	let {
		defaultName,
		defaultLogql,
		onClose
	}: {
		defaultName: string;
		defaultLogql: string;
		onClose: () => void;
	} = $props();

	const BLANK_SHAPE: RuleShape = 'threshold';
	const BLANK_WINDOW_SEC = 60;
	const BLANK_THRESHOLD = 5;
	const BLANK_GROUP_BY = '';
	const BLANK_SEVERITY: RuleSeverity = 'warning';

	let name = $state(defaultName);
	let logql = $state(defaultLogql);
	let shape = $state<RuleShape>(BLANK_SHAPE);
	let windowSec = $state(BLANK_WINDOW_SEC);
	let threshold = $state(BLANK_THRESHOLD);
	let groupBy = $state(BLANK_GROUP_BY);
	let severity = $state<RuleSeverity>(BLANK_SEVERITY);
	let submitting = $state(false);
	let error = $state<string | null>(null);

	function applyTemplate(event: Event) {
		const index = Number((event.target as HTMLSelectElement).value);
		if (index < 0) {
			name = defaultName;
			logql = defaultLogql;
			shape = BLANK_SHAPE;
			windowSec = BLANK_WINDOW_SEC;
			threshold = BLANK_THRESHOLD;
			groupBy = BLANK_GROUP_BY;
			severity = BLANK_SEVERITY;
			return;
		}
		const template = RULE_TEMPLATES[index];
		name = template.name;
		logql = template.logql;
		shape = template.shape;
		windowSec = template.windowSec;
		threshold = template.threshold;
		groupBy = template.groupBy;
		severity = template.severity;
	}

	async function submit(event: SubmitEvent) {
		event.preventDefault();
		submitting = true;
		error = null;
		try {
			const response = await fetch('/api/search/rules', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({
					name,
					shape,
					logql,
					window_sec: windowSec,
					threshold,
					group_by: parseGroupBy(groupBy),
					severity,
					destinations: ['inapp'],
					cooldown_sec: 3600,
					interval_sec: 60,
					enabled: true
				})
			});
			if (!response.ok) {
				error = 'Failed to create rule.';
				return;
			}
			onClose();
		} finally {
			submitting = false;
		}
	}
</script>

<div class="overlay">
	<form class="rule-form" onsubmit={submit}>
		<h2>Create rule</h2>
		<label>
			Template
			<select onchange={applyTemplate}>
				<option value="-1">Blank / custom</option>
				{#each RULE_TEMPLATES as template, index (template.name)}
					<option value={index}>{template.label}</option>
				{/each}
			</select>
		</label>
		<label>
			Name
			<input bind:value={name} required />
		</label>
		<label>
			Rule type
			<select bind:value={shape}>
				<option value="threshold">threshold</option>
				<option value="absence">absence</option>
				<option value="first_seen">first_seen</option>
			</select>
		</label>
		{#if shape !== 'absence'}
			<label>
				LogQL
				<textarea bind:value={logql} required></textarea>
			</label>
		{/if}
		{#if shape !== 'absence'}
			<label>
				Window (seconds)
				<input type="number" bind:value={windowSec} min="1" />
			</label>
		{/if}
		{#if shape === 'threshold'}
			<label>
				Threshold
				<input type="number" bind:value={threshold} min="1" />
			</label>
		{/if}
		{#if shape !== 'absence'}
			<label>
				Group by (comma-separated)
				<input bind:value={groupBy} placeholder="source, host" />
			</label>
		{/if}
		<label>
			Severity
			<select bind:value={severity}>
				<option value="critical">critical</option>
				<option value="warning">warning</option>
				<option value="info">info</option>
			</select>
		</label>
		{#if error}
			<p class="error">{error}</p>
		{/if}
		<div class="actions">
			<button type="button" onclick={onClose}>Cancel</button>
			<button type="submit" disabled={submitting}>
				{submitting ? 'Creating…' : 'Create rule'}
			</button>
		</div>
	</form>
</div>

<style>
	.overlay {
		position: fixed;
		inset: 0;
		background: rgba(0, 0, 0, 0.5);
		display: flex;
		align-items: center;
		justify-content: center;
		z-index: 10;
	}
	.rule-form {
		background: var(--color-surface);
		border-radius: var(--radius-lg);
		box-shadow: var(--shadow-raised);
		padding: var(--space-6);
		width: 360px;
		max-height: 90vh;
		overflow-y: auto;
		display: flex;
		flex-direction: column;
		gap: var(--space-3);
	}
	h2 {
		margin: 0;
		font-size: var(--text-section-head);
	}
	label {
		display: flex;
		flex-direction: column;
		gap: var(--space-1);
		font-size: var(--text-label);
		color: var(--color-muted);
	}
	input,
	select,
	textarea {
		background: var(--color-surface-2);
		border: none;
		border-radius: var(--radius-sm);
		color: var(--color-text);
		padding: var(--space-2);
		font-size: var(--text-table);
		font-family: inherit;
	}
	textarea {
		font-family: var(--font-mono);
		font-size: var(--text-label);
		min-height: 60px;
		resize: vertical;
	}
	.error {
		color: var(--color-severity-critical);
		font-size: var(--text-label);
		margin: 0;
	}
	.actions {
		display: flex;
		justify-content: flex-end;
		gap: var(--space-2);
		margin-top: var(--space-2);
	}
	.actions button {
		border: none;
		border-radius: var(--radius-sm);
		padding: var(--space-1) var(--space-3);
		font-size: var(--text-label);
		cursor: pointer;
	}
	.actions button[type='submit'] {
		background: var(--color-accent-tint-2);
		color: var(--color-accent-lighter);
	}
	.actions button[type='button'] {
		background: var(--color-surface-2);
		color: var(--color-text);
	}
	.actions button:disabled {
		opacity: 0.6;
		cursor: default;
	}
</style>
```

Note the one deliberate addition beyond the original file: `max-height: 90vh` and
`overflow-y: auto` on `.rule-form`, since the form now has more fields and could
overflow a short viewport.

- [ ] **Step 6: Verify**

Run:
```bash
cd siem-web && pnpm exec vitest run && pnpm lint && pnpm exec svelte-check
```
Expected: all clean, full suite passes (baseline count + 7 new tests).

Manually verify (dev server or Playwright): open the Search page's "Alert on this"
flow (unchanged call site — confirms the default/blank path still works exactly as
before), then separately confirm each of the 5 templates in the dropdown correctly
populates all fields, and that selecting `absence` hides LogQL/Window/Group-by,
while `first_seen` hides only Threshold.

- [ ] **Step 7: Commit**

```bash
git add siem-web/src/lib/ruleTemplates.ts siem-web/src/lib/ruleTemplates.test.ts siem-web/src/lib/components/RuleFromEventForm.svelte
git commit -m "Generalize RuleFromEventForm: rule-type selector, group-by field, and 5 starter templates"
```

---

### Task 4: Wire "+ New rule" into the Alerts screen, with e2e coverage

**Files:**
- Modify: `siem-web/src/lib/components/AlertInbox.svelte`
- Modify: `siem-web/src/routes/alerts/+page.svelte`
- Create: `siem-web/e2e/alerts-new-rule.e2e.ts`

**Interfaces:**
- Consumes: `RuleFromEventForm` from Task 3, unchanged props
  (`defaultName: string`, `defaultLogql: string`, `onClose: () => void`).
- Produces: nothing consumed elsewhere.

- [ ] **Step 1: Add the button to `AlertInbox.svelte`**

Add `onNewRule` to the component's props (current props block, after Task 2's
changes):

```svelte
	let {
		tab,
		alerts,
		rules,
		selectedId,
		onNewRule
	}: {
		tab: 'open' | 'acked' | 'rules';
		alerts: AlertResponse[];
		rules: RuleResponse[];
		selectedId: number | null;
		onNewRule: () => void;
	} = $props();
```

In the `.header` block, add the button after the `.tabs` div, visible only on the
Rules tab:

```svelte
	<div class="header">
		<span class="title">Alerts</span>
		<div class="tabs">
			{#each tabs as t (t.value)}
				<a href={resolve(`/alerts?state=${t.value}`)} class:active={tab === t.value}>{t.label}</a>
			{/each}
		</div>
		{#if tab === 'rules'}
			<button class="new-rule" onclick={onNewRule}>+ New rule</button>
		{/if}
	</div>
```

Add to the `<style>` block:

```css
.new-rule {
	background: none;
	border: 1px solid var(--color-line-2);
	color: var(--color-accent-light);
	border-radius: var(--radius-sm);
	padding: var(--space-1) var(--space-3);
	font-size: var(--text-table);
	cursor: pointer;
}
```

- [ ] **Step 2: Wire it up in `+page.svelte`**

Add the import and state, and render the form conditionally. Current script block:

```svelte
<script lang="ts">
	import { invalidateAll } from '$app/navigation';
	import { resolve } from '$app/paths';
	import AlertInbox from '$lib/components/AlertInbox.svelte';
	import AlertDetail from '$lib/components/AlertDetail.svelte';
	import RuleDetail from '$lib/components/RuleDetail.svelte';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	$effect(() => {
		const source = new EventSource(resolve('/api/alerts-proxy'));
		source.onmessage = () => {
			invalidateAll();
		};
		return () => source.close();
	});
</script>
```

Replace with:

```svelte
<script lang="ts">
	import { invalidateAll } from '$app/navigation';
	import { resolve } from '$app/paths';
	import AlertInbox from '$lib/components/AlertInbox.svelte';
	import AlertDetail from '$lib/components/AlertDetail.svelte';
	import RuleDetail from '$lib/components/RuleDetail.svelte';
	import RuleFromEventForm from '$lib/components/RuleFromEventForm.svelte';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();
	let showRuleForm = $state(false);

	$effect(() => {
		const source = new EventSource(resolve('/api/alerts-proxy'));
		source.onmessage = () => {
			invalidateAll();
		};
		return () => source.close();
	});
</script>
```

Update the markup: pass `onNewRule` to `AlertInbox`, and render the form at the end
of the `.alerts` div's sibling content:

```svelte
<div class="alerts">
	<AlertInbox
		tab={data.tab}
		alerts={data.alerts}
		rules={data.rules}
		selectedId={data.selectedAlert?.id ?? data.selectedRule?.id ?? null}
		onNewRule={() => (showRuleForm = true)}
	/>
	{#if data.selectedAlert && data.stats}
		<AlertDetail
			alert={data.selectedAlert}
			samples={data.selectedSamples}
			stats={data.stats}
			rule={data.rules.find((r) => r.id === data.selectedAlert?.rule_id)}
		/>
	{:else if data.selectedRule}
		<RuleDetail rule={data.selectedRule} />
	{:else}
		<div class="empty">
			{data.tab === 'rules' ? 'Select a rule to see details.' : 'Select an alert to see details.'}
		</div>
	{/if}
</div>

{#if showRuleForm}
	<RuleFromEventForm
		defaultName=""
		defaultLogql=""
		onClose={() => {
			showRuleForm = false;
			invalidateAll();
		}}
	/>
{/if}
```

(The `<style>` block is unchanged.)

- [ ] **Step 3: Verify manually before writing the e2e test**

Run:
```bash
cd siem-web && pnpm exec vitest run && pnpm lint && pnpm exec svelte-check
```
Expected: clean.

Start the app (or use the existing dev/preview flow) and manually confirm: on
`/alerts?state=rules`, a "+ New rule" button appears next to the tabs (and only on
that tab); clicking it opens the form; selecting a template populates the fields;
submitting creates the rule and the new rule appears in the list without a page
reload.

- [ ] **Step 4: Write the e2e test**

Create `siem-web/e2e/alerts-new-rule.e2e.ts`, following
`siem-web/e2e/nav-account-menu.e2e.ts`'s exact pattern for reading `SESSION_SECRET`
from `.env` and minting a session cookie — read that file first for the full
boilerplate (imports, `loadSessionSecret()`, `BASE_URL`, cookie-setting via
`context.addCookies`) and reuse it verbatim, changing only the test body below:

```typescript
import { test, expect } from '@playwright/test';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import { mintSessionToken, SESSION_COOKIE_NAME } from '../src/lib/server/session';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const BASE_URL = 'http://localhost:4173';

function loadSessionSecret(): Uint8Array {
	const envPath = path.resolve(__dirname, '../.env');
	const envContents = readFileSync(envPath, 'utf-8');
	const match = envContents.match(/^SESSION_SECRET=(.*)$/m);
	if (!match) {
		throw new Error(`SESSION_SECRET not found in ${envPath}`);
	}
	return new Uint8Array(Buffer.from(match[1].trim(), 'base64'));
}

test('creating a rule from a template shows it in the Rules list', async ({ page, context }) => {
	const secret = loadSessionSecret();
	const token = await mintSessionToken(
		{
			sub: 'oidc-sub-e2e',
			userId: 1,
			email: 'alice@townsville.cc',
			displayName: 'Alice Analyst',
			role: 'analyst'
		},
		secret
	);
	await context.addCookies([
		{
			name: SESSION_COOKIE_NAME,
			value: token,
			url: BASE_URL,
			httpOnly: true,
			sameSite: 'Lax'
		}
	]);

	await page.goto(`${BASE_URL}/alerts?state=rules`);

	const newRuleButton = page.getByRole('button', { name: '+ New rule' });
	await expect(newRuleButton).toBeVisible();
	await newRuleButton.click();

	const uniqueName = `e2e-vpn-connect-${Date.now()}`;
	await page.getByLabel('Template').selectOption({ label: 'VPN connection' });
	await page.getByLabel('Name').fill(uniqueName);
	await page.getByRole('button', { name: 'Create rule' }).click();

	await expect(page.getByRole('dialog').or(page.locator('.rule-form'))).toHaveCount(0);
	await expect(page.getByText(uniqueName)).toBeVisible();
});
```

Check the exact `mintSessionToken` argument shape and `RuleFromEventForm`'s actual
rendered DOM (whether `Template`/`Name` `<label>` wrapping makes `getByLabel` resolve
correctly, and whether closing the form removes `.rule-form` from the DOM as
assumed above) against the real files before finalizing — adjust selectors to match
if the assumptions above don't hold exactly; the test must actually pass against
the real implementation, not just against this plan's prediction of it.

- [ ] **Step 5: Run the e2e test**

Run: `cd siem-web && pnpm test:e2e alerts-new-rule`
Expected: PASS.

- [ ] **Step 6: Run the full verification suite one more time**

```bash
cd siem-web && pnpm exec vitest run && pnpm lint && pnpm exec svelte-check
```
Expected: clean.

- [ ] **Step 7: Commit**

```bash
git add siem-web/src/lib/components/AlertInbox.svelte siem-web/src/routes/alerts/+page.svelte siem-web/e2e/alerts-new-rule.e2e.ts
git commit -m "Alerts: add + New rule button wired to the templated rule-creation form, with e2e coverage"
```

---

## Addendum: final-review follow-ups (Tasks 5-9)

Tasks 1-4 above were reviewed clean (task-level and final whole-branch review,
commit range `54bb4bf..4d75178`, plus one Important layout fix at `22889fb`). The
final review also surfaced five Minor findings and one already-fixed Important
one. The human partner asked to fix all of them rather than defer. These five
tasks close them out. They build on top of commit `22889fb`.

**Additional Global Constraint for this addendum:** `siem-web/src/lib/ruleTemplates.ts`
is imported by a client-side Svelte component (`RuleFromEventForm.svelte`).
SvelteKit forbids client code from importing anything under `$lib/server/`. Any
shared type used by both `siemApiClient.ts` (server-only) and `ruleTemplates.ts`
(client) must live in a new non-server module, not in `siemApiClient.ts` itself.

### Task 5: Fix info-severity text contrast (WCAG AA)

**Files:**
- Modify: `siem-web/src/lib/components/AlertRow.svelte`
- Modify: `siem-web/src/lib/components/AlertDetail.svelte`
- Modify: `siem-web/src/lib/components/TriageCard.svelte`

**Interfaces:** none — pure CSS.

**Background:** Task 1 fixed the info-severity *border* color but reused the same
`--color-severity-info` token (`#5d5294`) for the eyebrow/header *text* too. Against
`--color-surface-2` (`#1b1d2c`) that's ~2.46:1 contrast — below WCAG AA's 4.5:1 floor
(and below the 3:1 large-text floor). A higher-contrast token already exists in
`siem-web/src/lib/styles/tokens.css:28`: `--color-severity-notice: #796cbf`
(~3.7:1), already used elsewhere in this codebase for a "notice"-tier syslog
severity in `siem-web/src/lib/tail.ts`. Reuse it here for text only — borders stay
`--color-severity-info`.

- [ ] **Step 1: `AlertRow.svelte`**

Change only these two lines in the `<style>` block (leave `.row`'s
`border-left: 3px solid var(--color-severity-info);` and the `.severity-warning`/
`.severity-critical` override rules untouched):

```css
.header {
	display: flex;
	justify-content: space-between;
	font-size: var(--text-eyebrow);
	text-transform: uppercase;
	color: var(--color-severity-notice);
}
```
(only the `color` value on `.header` changes, from `--color-severity-info` to
`--color-severity-notice`; `.row.severity-warning .header` and
`.row.severity-critical .header` stay as-is)

- [ ] **Step 2: `AlertDetail.svelte`**

Change only the `.eyebrow` base rule's `color`:

```css
.eyebrow {
	font-size: var(--text-eyebrow);
	text-transform: uppercase;
	color: var(--color-severity-notice);
}
```
(`.eyebrow.severity-warning` and `.eyebrow.severity-critical` stay as-is)

- [ ] **Step 3: `TriageCard.svelte`**

Change only the `.header` base rule's `color` (the card's `box-shadow` border strip
stays `--color-severity-info`):

```css
.header {
	display: flex;
	justify-content: space-between;
	font-size: var(--text-eyebrow);
	text-transform: uppercase;
	color: var(--color-severity-notice);
}
```
(`.card.severity-critical .header` and `.card.severity-warning .header` stay as-is)

- [ ] **Step 4: Verify**

```bash
cd siem-web && pnpm lint && pnpm exec svelte-check
```
Expected: clean, no new warnings.

- [ ] **Step 5: Commit**

```bash
git add siem-web/src/lib/components/AlertRow.svelte siem-web/src/lib/components/AlertDetail.svelte siem-web/src/lib/components/TriageCard.svelte
git commit -m "Alerts/Wall: fix info-severity text contrast (was ~2.46:1, below WCAG AA)"
```

### Task 6: Add backend-semantics invariant tests to `ruleTemplates.test.ts`

**Files:**
- Modify: `siem-web/src/lib/ruleTemplates.test.ts`

**Interfaces:** none — test-only change, no source changes.

**Background:** the existing tests assert template count, non-empty labels/names,
and that the one `absence` template has an empty `logql`. They don't assert the
converse invariants that actually matter operationally (per the backend semantics
table established in Task 3's brief): a `first_seen` or `threshold` template with
an empty `logql` would create a rule that silently never fires, and nothing today
would catch that regression.

- [ ] **Step 1: Add the test**

Add this new `describe` block to `siem-web/src/lib/ruleTemplates.test.ts`, after
the existing `describe('RULE_TEMPLATES', ...)` block and before
`describe('parseGroupBy', ...)`:

```typescript
describe('RULE_TEMPLATES backend-semantics invariants', () => {
	it.each(RULE_TEMPLATES)(
		'$name ($shape): logql/threshold/windowSec match what its evaluator reads',
		(template) => {
			if (template.shape !== 'absence') {
				expect(template.logql.length).toBeGreaterThan(0);
			}
			if (template.shape === 'threshold') {
				expect(template.threshold).toBeGreaterThanOrEqual(1);
			}
			expect(template.windowSec).toBeGreaterThanOrEqual(1);
		}
	);
});
```

- [ ] **Step 2: Run the tests**

Run: `cd siem-web && pnpm exec vitest run ruleTemplates`
Expected: PASS, 12 tests (7 existing + 5 new, one per template via `it.each`).

- [ ] **Step 3: Commit**

```bash
git add siem-web/src/lib/ruleTemplates.test.ts
git commit -m "ruleTemplates: add backend-semantics invariant tests (logql/threshold/window per shape)"
```

### Task 7: Narrow alert/rule severity from `string` to a shared union type

**Files:**
- Create: `siem-web/src/lib/severity.ts`
- Modify: `siem-web/src/lib/server/siemApiClient.ts`
- Modify: `siem-web/src/lib/ruleTemplates.ts`
- Modify: `siem-web/src/lib/wall.ts`
- Modify: `siem-web/src/lib/components/RuleFromEventForm.svelte`

**Interfaces:**
- Produces: `AlertSeverity` type (`'info' | 'warning' | 'critical'`) from
  `$lib/severity`, consumed by all four modified files.
- Consumes: nothing new.

**Background:** `AlertResponse.severity`, `RuleResponse.severity`, and
`CreateRuleRequest.severity` in `siemApiClient.ts` are all typed as bare `string`.
`ruleTemplates.ts` independently defined its own `RuleSeverity` union
(`'info' | 'warning' | 'critical'`) with no connection to the API types. This is
the structural reason the Task 1 low/medium bug could happen in the first place —
nothing tied the CSS severity classes to a real type. This task creates one shared
type and wires the existing call sites to it — it does not change any runtime
behavior, only type annotations (plus one `Record` key change in `wall.ts` — see
Step 4).

- [ ] **Step 1: Create `siem-web/src/lib/severity.ts`**

```typescript
export type AlertSeverity = 'info' | 'warning' | 'critical';
```

That's the whole file — deliberately minimal so both server and client code can
import it without crossing SvelteKit's `$lib/server` boundary.

- [ ] **Step 2: Update `siemApiClient.ts`**

Add the import at the top of the file:

```typescript
import type { AlertSeverity } from './severity';
```

Change three `severity: string;` field declarations to `severity: AlertSeverity;`:
- `AlertResponse.severity` (currently line 17)
- `RuleResponse.severity` (currently line 42)
- `CreateRuleRequest.severity` (currently line 57)

Leave every other field (including `NotificationSettingsResponse.min_severity`,
which is a different, unrelated concept — a threshold setting, not an alert's own
severity — do not touch it) exactly as-is.

- [ ] **Step 3: Update `ruleTemplates.ts`**

Replace the file's own `RuleSeverity` type definition with an import, and update
its one usage:

```typescript
import type { AlertSeverity } from './severity';

export type RuleShape = 'threshold' | 'absence' | 'first_seen';
```

(delete the line `export type RuleSeverity = 'info' | 'warning' | 'critical';`
entirely) and change the `RuleTemplate` type's `severity` field:

```typescript
export type RuleTemplate = {
	label: string;
	name: string;
	shape: RuleShape;
	logql: string;
	windowSec: number;
	threshold: number;
	groupBy: string;
	severity: AlertSeverity;
};
```

The `RULE_TEMPLATES` array's literal values don't change — only the type they're
checked against.

- [ ] **Step 4: Update `wall.ts`**

Add the import and change `SEVERITY_RANK`'s declared type from
`Record<string, number>` to `Record<AlertSeverity, number>` (the literal object
already has exactly the three required keys, so this is purely a stricter type —
TypeScript will now error at compile time if a key is ever misspelled or a new
severity is added to `AlertSeverity` without updating this map):

```typescript
import type { AlertSeverity } from './severity';
import type { AlertResponse, LogEntry } from './server/siemApiClient';
```

```typescript
const SEVERITY_RANK: Record<AlertSeverity, number> = { critical: 3, warning: 2, info: 1 };
```

Leave the `?? 0` fallback in `topTriageAlerts`'s sort comparator exactly as-is —
even though the type now guarantees three keys, the fallback is a legitimate
runtime defense against a value that doesn't match its declared type (e.g. stale
data, a backend bug) and removing it would be an unrelated behavior change, not
part of this task.

- [ ] **Step 5: Update `RuleFromEventForm.svelte`**

Change the import line from:

```typescript
import {
	RULE_TEMPLATES,
	parseGroupBy,
	type RuleShape,
	type RuleSeverity
} from '$lib/ruleTemplates';
```

to:

```typescript
import { RULE_TEMPLATES, parseGroupBy, type RuleShape } from '$lib/ruleTemplates';
import type { AlertSeverity } from '$lib/severity';
```

Then replace every use of `RuleSeverity` in this file with `AlertSeverity` (two
occurrences: `const BLANK_SEVERITY: RuleSeverity = 'warning';` and
`let severity = $state<RuleSeverity>(BLANK_SEVERITY);`). No other changes in this
file.

- [ ] **Step 6: Verify**

```bash
cd siem-web && pnpm exec vitest run && pnpm lint && pnpm exec svelte-check
```
Expected: all clean — this is a type-only change plus one `Record` key
annotation, so no test's runtime behavior should differ (same 146+12 tests from
Tasks 1-6 all still passing). Pay attention to `svelte-check`: confirm it reports
zero *new* errors (a bad import path across the server/client boundary would
surface here as a build-time error, not a silent runtime issue).

- [ ] **Step 7: Commit**

```bash
git add siem-web/src/lib/severity.ts siem-web/src/lib/server/siemApiClient.ts siem-web/src/lib/ruleTemplates.ts siem-web/src/lib/wall.ts siem-web/src/lib/components/RuleFromEventForm.svelte
git commit -m "Narrow alert/rule severity from string to a shared AlertSeverity union"
```

### Task 8: Harden the new-rule form (network-error handling) and gate it by role

**Files:**
- Modify: `siem-web/src/lib/components/RuleFromEventForm.svelte`
- Modify: `siem-web/src/routes/alerts/+page.server.ts`
- Modify: `siem-web/src/routes/alerts/+page.svelte`
- Modify: `siem-web/src/lib/components/AlertInbox.svelte`

**Interfaces:**
- Consumes: `locals.user?.role` (already populated by `hooks.server.ts` on every
  authenticated request — see the identical existing pattern at
  `siem-web/src/routes/sources/+page.server.ts:62`, `userRole: locals.user?.role`).
- Produces: `AlertInbox` gains a new required prop `canCreateRule: boolean`
  (replacing the current unconditional-on-tab visibility) — no other file in this
  branch renders `AlertInbox`, so this is not a breaking change to any other call
  site.

**Background:** two separate Minor findings from the final review, both about the
same feature (the new rule-creation form), fixed together:
1. `RuleFromEventForm.svelte`'s `submit()` has no `catch` — a network-level fetch
   rejection leaves `submitting` reset (via `finally`) but shows no error message,
   so the form silently snaps back to idle. A non-ok response only ever shows the
   generic "Failed to create rule.", even for a 403 (a `viewer`-role user who
   reached the form, which Task 8's second half prevents from happening in the
   normal UI flow, but the fetch could still 403 from a stale session or a
   role downgrade mid-session).
2. `POST /rules` is `RoleAnalyst`-gated server-side
   (`siem-api/internal/api/server.go:68`), but the "+ New rule" button Task 4 added
   is visible to every role, including `viewer` (rank 1, below `analyst`'s rank 2).
   A `viewer` who clicks it gets a 403 from the fix in point 1 above, but the
   button shouldn't be there at all — this codebase's established pattern
   (`Nav.svelte:66` hides Settings for non-admins; `sources/+page.svelte:38`'s
   `canClaim={data.userRole === 'admin'}`) is to hide role-gated actions rather
   than show-then-reject them.

- [ ] **Step 1: Fix `RuleFromEventForm.svelte`'s error handling**

Replace the `submit` function's body:

```typescript
async function submit(event: SubmitEvent) {
	event.preventDefault();
	submitting = true;
	error = null;
	try {
		const response = await fetch('/api/search/rules', {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({
				name,
				shape,
				logql,
				window_sec: windowSec,
				threshold,
				group_by: parseGroupBy(groupBy),
				severity,
				destinations: ['inapp'],
				cooldown_sec: 3600,
				interval_sec: 60,
				enabled: true
			})
		});
		if (!response.ok) {
			error =
				response.status === 403
					? "You don't have permission to create rules."
					: 'Failed to create rule.';
			return;
		}
		onClose();
	} catch {
		error = 'Network error — check your connection and try again.';
	} finally {
		submitting = false;
	}
}
```

- [ ] **Step 2: Add `userRole` to the alerts page load**

In `siem-web/src/routes/alerts/+page.server.ts`, add `userRole: locals.user?.role`
to the object returned at the end of the `load` function (alongside the existing
`tab`, `alerts`, `rules`, `selectedAlert`, `selectedSamples`, `stats`,
`selectedRule` fields — do not change any of those).

- [ ] **Step 3: Gate the button in `AlertInbox.svelte`**

Change the props type and destructure to replace `onNewRule`'s always-rendered
condition with a role check. Current:

```svelte
	let {
		tab,
		alerts,
		rules,
		selectedId,
		onNewRule
	}: {
		tab: 'open' | 'acked' | 'rules';
		alerts: AlertResponse[];
		rules: RuleResponse[];
		selectedId: number | null;
		onNewRule: () => void;
	} = $props();
```

Replace with:

```svelte
	let {
		tab,
		alerts,
		rules,
		selectedId,
		onNewRule,
		canCreateRule
	}: {
		tab: 'open' | 'acked' | 'rules';
		alerts: AlertResponse[];
		rules: RuleResponse[];
		selectedId: number | null;
		onNewRule: () => void;
		canCreateRule: boolean;
	} = $props();
```

And change the button's condition from `{#if tab === 'rules'}` to
`{#if tab === 'rules' && canCreateRule}` (this is the same button block Step 1 of
Task 4 already positioned before `.tabs` in the "final review" fix — do not move
it again, only change the `{#if}` condition).

- [ ] **Step 4: Pass `canCreateRule` from `+page.svelte`**

In `siem-web/src/routes/alerts/+page.svelte`, add the prop to the existing
`<AlertInbox>` call:

```svelte
	<AlertInbox
		tab={data.tab}
		alerts={data.alerts}
		rules={data.rules}
		selectedId={data.selectedAlert?.id ?? data.selectedRule?.id ?? null}
		onNewRule={() => (showRuleForm = true)}
		canCreateRule={data.userRole === 'admin' || data.userRole === 'analyst'}
	/>
```

- [ ] **Step 5: Verify**

```bash
cd siem-web && pnpm exec vitest run && pnpm lint && pnpm exec svelte-check
```
Expected: clean. `svelte-check` in particular will catch it if `canCreateRule` is
missing from any `AlertInbox` call site — there is exactly one, in `+page.svelte`,
already updated by Step 4.

Manually verify (dev server or Playwright with a minted session, varying the
`role` claim between `admin`/`analyst`/`viewer` in the minted token): the button
is visible on the Rules tab for `admin`/`analyst` and absent for `viewer`.

- [ ] **Step 6: Commit**

```bash
git add siem-web/src/lib/components/RuleFromEventForm.svelte siem-web/src/routes/alerts/+page.server.ts siem-web/src/routes/alerts/+page.svelte siem-web/src/lib/components/AlertInbox.svelte
git commit -m "Alerts: handle network errors in rule creation, hide + New rule from viewer role"
```

### Task 9: e2e test cleanup and a documenting comment

**Files:**
- Modify: `siem-web/e2e/alerts-new-rule.e2e.ts`

**Interfaces:** none.

**Background:** two Minor findings, both about the e2e test added in Task 4: (1)
it creates a real rule via `POST /rules` with no cleanup, so rows accumulate in
whatever database `API_URL` points at across repeated runs (bounded and harmless
since names are `Date.now()`-uniqued, but untidy); (2) nothing in the file
documents that it requires a live `siem-api` reachable at `API_URL` — if one isn't
running, the test fails with a 500 from `+page.server.ts`'s `load` function, not
an obviously-relevant selector error, which the final review confirmed is already
true of the *existing* `nav-account-menu.e2e.ts`/`login.e2e.ts` suite (they load
pages whose `load` functions also call `siem-api`) — this task's cleanup step is
the one genuinely *new* dependency surface (a *write*, not just a read).

The cleanup must not add any new production-facing route — do this by having the
test call `siem-api` directly (bypassing `siem-web`'s server entirely), using the
same JWT (`token`) the test already mints for the session cookie as a bearer
token, and the same `.env`-reading pattern already used for `SESSION_SECRET` to
read `API_URL`. `siem-api` already exposes `GET /rules` and
`DELETE /rules/{id}` (`RoleAnalyst`-gated, matching this test's `groups`), so no
backend or `siem-web` proxy change is needed.

- [ ] **Step 1: Add an `API_URL` reader and a cleanup helper**

Add this near the top of `siem-web/e2e/alerts-new-rule.e2e.ts`, after
`loadSessionSecret`:

```typescript
function loadApiUrl(): string {
	const envPath = path.resolve(__dirname, '../.env');
	const envContents = readFileSync(envPath, 'utf-8');
	const match = envContents.match(/^API_URL=(.*)$/m);
	if (!match) {
		throw new Error(`API_URL not found in ${envPath}`);
	}
	return match[1].trim();
}

async function deleteRuleByName(apiUrl: string, token: string, name: string): Promise<void> {
	const listResponse = await fetch(`${apiUrl}/rules`, {
		headers: { Authorization: `Bearer ${token}` }
	});
	if (!listResponse.ok) return;
	const rules = (await listResponse.json()) as { id: number; name: string }[];
	const created = rules.find((r) => r.name === name);
	if (!created) return;
	await fetch(`${apiUrl}/rules/${created.id}`, {
		method: 'DELETE',
		headers: { Authorization: `Bearer ${token}` }
	});
}
```

- [ ] **Step 2: Call it in the test, and add the documenting comment**

Add a comment above the `test(...)` call documenting the live-backend
requirement, and wrap the test body so cleanup runs even if an assertion fails.
Replace:

```typescript
test('creating a rule from a template shows it in the Rules list', async ({ page, context }) => {
	const secret = loadSessionSecret();
	const token = await mintSessionToken(
```

with:

```typescript
// This test exercises the real create-rule flow end to end (no mocking), so it
// requires a live siem-api reachable at the API_URL configured in siem-web/.env.
// Without one, the /alerts?state=rules page load itself fails with a 502 before
// any Playwright selector runs — same requirement the rest of this e2e suite
// already has for reads; this test is the first one in the suite to also write
// (create a rule), so it cleans up after itself via a direct siem-api call below.
test('creating a rule from a template shows it in the Rules list', async ({ page, context }) => {
	const apiUrl = loadApiUrl();
	const secret = loadSessionSecret();
	const token = await mintSessionToken(
```

Then, at the end of the test body, replace:

```typescript
	await expect(page.locator('.rule-form')).toHaveCount(0);
	await expect(page.getByText(uniqueName)).toBeVisible();
});
```

with:

```typescript
	try {
		await expect(page.locator('.rule-form')).toHaveCount(0);
		await expect(page.getByText(uniqueName)).toBeVisible();
	} finally {
		await deleteRuleByName(apiUrl, token, uniqueName);
	}
});
```

- [ ] **Step 3: Run the e2e test against a real backend**

This requires a live `siem-api` reachable at the `API_URL` in `siem-web/.env`,
seeded the same way Task 4's implementer already did (built `siem-api` from
source, seeded a `role_mappings` row and a `users` row so the minted session's
`groups` claim resolves to a real role and `rules.created_by`'s foreign key has a
row to point at). Run: `cd siem-web && pnpm test:e2e alerts-new-rule`

Expected: PASS. Additionally confirm cleanup actually worked — after the run,
`GET {API_URL}/rules` (with a valid bearer token) should NOT contain a rule named
with this run's `e2e-vpn-connect-<timestamp>` value. Stop any backend process you
started for this verification afterward, the same way Task 4's implementer did —
this is throwaway local infrastructure, never committed.

- [ ] **Step 4: Run the full non-e2e verification suite one more time**

```bash
cd siem-web && pnpm exec vitest run && pnpm lint && pnpm exec svelte-check
```
Expected: clean.

- [ ] **Step 5: Commit**

```bash
git add siem-web/e2e/alerts-new-rule.e2e.ts
git commit -m "alerts-new-rule e2e: clean up the created rule after each run, document the live-siem-api requirement"
```
