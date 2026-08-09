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
