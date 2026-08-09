# Alerts Panel Fixes and Gaps — Design

**Status:** Approved
**Origin:** Live GUI audit ("Alerts" section) + final review of the Wall dashboard rebuild (PR #31), which found the same severity-vocabulary bug reproduced on this screen.

## Goal

Close four concrete gaps on the `/alerts` screen: a severity-color bug shared with the
now-fixed Wall page, a text mismatch on the Rules tab, missing empty-state messaging,
and a missing rule-creation affordance — while reusing the existing rule-creation
backend (`POST /rules`) and form component (`RuleFromEventForm.svelte`) rather than
building anything new from scratch.

## Architecture

No backend changes. All four items are `siem-web` frontend changes, touching:
`AlertRow.svelte`, `AlertDetail.svelte`, `AlertInbox.svelte`, `RuleFromEventForm.svelte`,
and `routes/alerts/+page.svelte`.

## Background: real severity vocabulary

Confirmed elsewhere in this codebase (`wall.ts`, `TriageCard.svelte`, both fixed in
PR #31): the only real severities are `info` / `warning` / `critical`. There is no
`low`/`medium`/`high`. Any CSS selector targeting `low`/`medium`/`high` is dead code
that can never match a real alert.

## Item 1 — Severity-vocabulary bug in `AlertRow.svelte` / `AlertDetail.svelte`

**`AlertRow.svelte`, current (buggy) CSS:**
```css
.row {
	border-left: 3px solid var(--color-severity-critical);
	...
}
.row.severity-warning {
	border-left-color: var(--color-severity-warning);
}
.row.severity-low,
.row.severity-medium {
	border-left-color: var(--color-severity-info);
}
...
.header {
	...
	color: var(--color-severity-critical);
}
```
Two bugs: (a) `.severity-low`/`.severity-medium` never match a real `alert.severity`
value, so `info`-severity alerts fall through to the `.row` default and render with
a critical-red border; (b) `.header`'s eyebrow text color is hardcoded to
`--color-severity-critical` with **no per-severity override at all** — every alert's
eyebrow text is critical-red regardless of actual severity, including `warning` ones
whose border color is already correct.

**Fix:** replace the dead `low`/`medium` selectors with `.severity-info`, and add a
matching `.header` color override per severity:
```css
.row {
	border-left: 3px solid var(--color-severity-info);
	...
}
.row.severity-warning {
	border-left-color: var(--color-severity-warning);
}
.row.severity-critical {
	border-left-color: var(--color-severity-critical);
}
...
.header {
	...
	color: var(--color-severity-info);
}
.row.severity-warning .header {
	color: var(--color-severity-warning);
}
.row.severity-critical .header {
	color: var(--color-severity-critical);
}
```
Default the base rule to `info` (not critical) so an unrecognized/future severity
degrades to the least alarming color rather than the most alarming one — same
reasoning already applied in `TriageCard.svelte`'s PR #31 fix.

**`AlertDetail.svelte`, current (buggy) CSS:**
```css
.eyebrow {
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
**Fix:** same shape as `AlertRow` — default `.eyebrow` to info, replace the dead
selectors:
```css
.eyebrow {
	color: var(--color-severity-info);
}
.eyebrow.severity-warning {
	color: var(--color-severity-warning);
}
.eyebrow.severity-critical {
	color: var(--color-severity-critical);
}
```

## Item 2 — Rules-tab empty-detail text (`routes/alerts/+page.svelte`)

**Current:**
```svelte
{#if data.selectedAlert && data.stats}
	<AlertDetail ... />
{:else if data.selectedRule}
	<RuleDetail rule={data.selectedRule} />
{:else}
	<div class="empty">Select an alert to see details.</div>
{/if}
```
The final `{:else}` fires whenever nothing is selected, on any tab, but always says
"alert" even when `data.tab === 'rules'`.

**Fix:**
```svelte
{:else}
	<div class="empty">
		{data.tab === 'rules' ? 'Select a rule to see details.' : 'Select an alert to see details.'}
	</div>
{/if}
```

## Item 3 — Empty-state messages (`AlertInbox.svelte`)

**Current:** both `{#each}` blocks (rules, alerts) render nothing when their array is
empty — no `{:else}` branch. Matches the tone already established on the Wall page
(`CountryBar.svelte`'s "No international traffic in this sample.",
`Ticker.svelte`'s "Waiting for live events…").

**Fix:**
```svelte
{#if tab === 'rules'}
	{#each rules as rule (rule.id)}
		<RuleRow {rule} selected={selectedId === rule.id} />
	{:else}
		<div class="empty-list">No rules configured yet.</div>
	{/each}
{:else}
	{#each alerts as alert (alert.id)}
		<AlertRow ... />
	{:else}
		<div class="empty-list">{tab === 'acked' ? 'No acknowledged alerts.' : 'No open alerts.'}</div>
	{/each}
{/if}
```
Add an `.empty-list` style, matching `CountryBar`/`Ticker`'s existing empty-state
treatment (`color: var(--color-muted-2)`, centered or left-aligned padding
consistent with the `.rows` flex column — implementer's judgment on exact padding,
no pixel-precise requirement here).

## Item 4 — "+ New rule" button, with prepopulated templates

### Backend facts that constrain the form (already verified against `siem-api`)

Three rule "shapes" exist server-side, each with a different evaluator and different
relevant fields:

| Shape | Uses `logql`? | Uses `window_sec`? | Uses `threshold`? | Uses `group_by`? |
|---|---|---|---|---|
| `threshold` | yes | yes | yes | yes (optional) |
| `first_seen` | yes | yes (lookback window) | **no** | yes (groups by these fields; a value not seen before in this grouping fires) |
| `absence` | **no** (ignored entirely — evaluator checks `Sources.StaleSources`, a global per-source heartbeat, never `rule.LogQL`) | **no** (unused; staleness comes from `store.Source.HeartbeatSec`, not `rule.WindowSec`) | **no** | **no** |

Source: `siem-api/internal/rules/threshold.go`, `first_seen.go`, `absence.go`.
`handleCreateRule` (`siem-api/internal/api/rules.go`) does no field validation — any
combination of populated/empty fields is accepted, so the frontend is free to omit
irrelevant fields for a given shape without a backend error.

### `RuleFromEventForm.svelte` changes

**New local state:**
- `shape: 'threshold' | 'absence' | 'first_seen'` (was implicitly always `'threshold'`
  in the submit body — now a real, user-editable field)
- `groupBy: string` — a plain comma-separated text input; split, trimmed, and
  empty-entries-filtered into `string[]` on submit (`[]` if the input is blank)

**New "Rule type" `<select>`**, bound to `shape`, options `threshold` / `absence` /
`first_seen`, placed after the LogQL field.

**Conditional field visibility**, driven by `shape`:
- `logql` field: hidden when `shape === 'absence'` (replace with no element — the
  field is genuinely inert for this shape per the table above; showing it would
  mislead the user into thinking it matters)
- `windowSec` field: hidden when `shape === 'absence'`
- `threshold` field: shown only when `shape === 'threshold'`
- `groupBy` field: hidden when `shape === 'absence'`

**New "Template" `<select>`**, placed as the first field in the form, bound to no
persisted state of its own — its `onchange` handler applies a preset by directly
reassigning `name`, `logql`, `shape`, `windowSec`, `threshold`, `groupBy`, `severity`.
Six options, in this order:

```ts
type Template = {
	label: string;
	name: string;
	shape: 'threshold' | 'absence' | 'first_seen';
	logql: string;
	windowSec: number;
	threshold: number;
	groupBy: string; // comma-separated, matches the field's own text-input shape
	severity: 'info' | 'warning' | 'critical';
};

const TEMPLATES: Template[] = [
	// index 0 = "Blank / custom" is handled specially (see below), not in this array
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
```
(`threshold`/`windowSec` values for the `absence` template are placeholders that
will never be read by the evaluator per the table above — included only so the
form's own state stays well-typed; not user-visible since those fields are hidden
for this shape.)

"Blank / custom" (the default, always-first `<option>`) resets to the form's
original defaults: `name = defaultName`, `logql = defaultLogql`, `shape = 'threshold'`,
`windowSec = 60`, `threshold = 5`, `groupBy = ''`, `severity = 'warning'` — i.e.
exactly today's pre-this-feature behavior, so the Search page's existing "Alert on
this" / "Rule from this event" call sites are unaffected by default.

**Submit body change:** `shape` and `group_by` (parsed from `groupBy`) replace the
hardcoded `shape: 'threshold'` and `group_by: []`.

### Wiring into the Alerts screen

- `AlertInbox.svelte` gains a new prop `onNewRule: () => void`. In the header, next
  to the tabs, render a "+ New rule" button — visible only when `tab === 'rules'`
  (creating a rule is a Rules-tab action; the button has no meaning on Open/Acked).
- `routes/alerts/+page.svelte` gains `let showRuleForm = $state(false);` and:
  ```svelte
  <AlertInbox ... onNewRule={() => (showRuleForm = true)} />
  ...
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
  `invalidateAll()` on every close (not just success) mirrors the page's existing
  SSE-driven refresh pattern (`+page.svelte`'s `EventSource.onmessage` handler,
  already unconditional) — re-running the load function on a cancel is harmless and
  cheap, and avoids threading a separate success/cancel distinction through
  `RuleFromEventForm`'s existing `onClose`-only callback contract.

## Testing

- `AlertRow`/`AlertDetail`/`AlertInbox`/`RuleFromEventForm`: this codebase has no
  Svelte component test infrastructure (established convention, not a gap to fix
  here) — these are verified via manual Playwright interaction during
  implementation, per this project's established pattern (Wall dashboard rebuild,
  Nav account menu).
- `routes/alerts/+page.server.test.ts`: no changes needed — item 2's fix is
  presentation-only (`+page.svelte`), not a `load` function change.
- New Playwright e2e test, following the pattern in
  `siem-web/e2e/nav-account-menu.e2e.ts` (mint a session token from `.env`'s
  `SESSION_SECRET` at runtime — never hardcode a secret in the test file):
  covers opening the "+ New rule" form from the Rules tab, selecting a template,
  submitting, and confirming the new rule appears in the list.
- `pnpm exec vitest run`, `pnpm lint`, `pnpm exec svelte-check` must all stay clean,
  matching this project's baseline (139/139 tests, 0 lint errors, 0 type errors as
  of PR #31).

## Out of scope

- No backend changes — `POST /rules` already supports every field this form now
  exposes.
- No rule editing or deletion UI (create-only, matching the existing `RuleDetail.svelte`
  which is still read-only).
- No validation beyond what `RuleFromEventForm` already has (`required` on name/logql
  — note `logql` is hidden, not removed, for `absence`, so its `required` attribute
  only applies when the field is rendered; Svelte does not validate hidden/removed
  form fields).
- The rest of the original GUI audit (Settings stub tabs, Live-tail empty state,
  Search HOST truncation, ambiguous button labels, low-contrast muted text) —
  tracked separately, not part of this pass.
