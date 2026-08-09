# PR #34 Follow-ups Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the three items PR #34's final review left as explicit
follow-ups: stale `"low"` severity fixtures at the siem-api store layer, a
Live-tail empty state that doesn't distinguish "no events yet" from "a filter
is hiding real events," and two same-labeled "New rule" buttons on Search with
no distinguishing accessible name.

**Architecture:** Three independent, small tasks. Task 1 is `siem-api` (Go);
Tasks 2-3 are `siem-web` (SvelteKit).

**Tech Stack:** Go (siem-api), SvelteKit/Svelte 5 (siem-web).

## Global Constraints

- The only real severity values are `info`/`warning`/`critical`.
- `siem-web` has no Svelte component test framework — Tasks 2-3 verify via
  `pnpm lint` / `pnpm exec svelte-check` plus manual interaction.
- `siem-api` changes are unit-testable — Task 1's `go test ./...` must stay
  green after the fixture change (none of the touched tests assert on
  severity itself, so behavior must not change).
- Before every commit in a `siem-web` task: `pnpm exec vitest run`,
  `pnpm lint`, `pnpm exec svelte-check` clean. Before Task 1's commit:
  `go build ./... && go vet ./... && go test ./...` clean.

---

### Task 1: siem-api — fix remaining stale `"low"` severity fixtures

**Files:**
- Modify: `siem-api/internal/api/alerts_test.go`
- Modify: `siem-api/internal/store/rules_test.go`
- Modify: `siem-api/internal/store/seen_values_test.go`
- Modify: `siem-api/internal/store/alerts_test.go`

**Interfaces:** none.

**Background:** these tests call `store.CreateRule`/`store.InsertAlert`
directly (bypassing the HTTP handler validation added in a prior branch), so
they were out of scope for that validation task. All 20 occurrences use
`Severity: "low"` as an incidental fixture value — none of these tests assert
anything about severity itself (they test scheduler behavior, alert dedup,
alert listing/filtering, and seen-value tracking). Confirmed occurrences via
`grep -n '"low"' siem-api/internal/api/alerts_test.go siem-api/internal/store/rules_test.go siem-api/internal/store/seen_values_test.go siem-api/internal/store/alerts_test.go`:
`alerts_test.go` (api package, 13 occurrences: lines 19, 25, 29, 55, 61, 81,
87, 176, 182, 202, 208, 252, 258), `rules_test.go` (store package, 4
occurrences: lines 46, 50, 68, 116), `seen_values_test.go` (store package, 1
occurrence: line 52), `alerts_test.go` (store package, 2 occurrences: lines
197 and 201, both in the same test).

- [ ] **Step 1: Replace every `Severity: "low"` with `Severity: "warning"`**

In all four files, change every occurrence of the Go struct-literal field
`Severity: "low"` to `Severity: "warning"`. Use a search-and-replace across
exactly these four files — do not touch `siem-api/internal/api/rules_test.go`
(already fixed in a prior branch) or any other file. Re-run the grep from the
Background section afterward to confirm zero remaining `"low"` matches in
these four files.

- [ ] **Step 2: Run the full test suite**

Run: `cd siem-api && go build ./... && go vet ./... && go test ./...`
Expected: all clean, identical pass/fail results to before the change (if any
test's behavior changes, that test was relying on the specific string
`"low"` in a way this plan didn't anticipate — stop and investigate rather
than reverting).

- [ ] **Step 3: Commit**

```bash
git add siem-api/internal/api/alerts_test.go siem-api/internal/store/rules_test.go siem-api/internal/store/seen_values_test.go siem-api/internal/store/alerts_test.go
git commit -m "siem-api: fix remaining stale 'low' severity fixtures in store-layer tests"
```

---

### Task 2: Live tail — make the empty state filter-aware

**Files:**
- Modify: `siem-web/src/lib/components/TailViewport.svelte`

**Interfaces:** none.

**Background:** the empty-state message added in a prior branch always reads
"Waiting for events…", even when the real cause is that the active severity
filter excludes everything currently in `buffer` (i.e., events ARE arriving,
they're just all filtered out). `rendered` is derived from `buffer` via
`filterBySeverity` (see the `$effect` at lines 52-62 and `appendEntry` at
lines 33-44) — `buffer` is already a bindable prop in scope in the template,
so no new state is needed to distinguish the two cases.

- [ ] **Step 1: Make the empty-row message conditional on `buffer`**

Current (lines 127-130):

```svelte
				{:else}
					<tr class="empty-row">
						<td colspan="6">Waiting for events…</td>
					</tr>
				{/each}
```

Replace with:

```svelte
				{:else}
					<tr class="empty-row">
						<td colspan="6">
							{buffer.length === 0
								? 'Waiting for events…'
								: 'No events match the current filter.'}
						</td>
					</tr>
				{/each}
```

- [ ] **Step 2: Verify**

```bash
cd siem-web && pnpm lint && pnpm exec svelte-check
```
Expected: clean.

Manually verify (dev server, with live traffic arriving): with all severity
filters active and no traffic yet, see "Waiting for events…"; with traffic
arriving but every severity filter checkbox deselected (so `buffer` grows but
`rendered` stays empty), see "No events match the current filter." instead.

- [ ] **Step 3: Commit**

```bash
git add siem-web/src/lib/components/TailViewport.svelte
git commit -m "Live tail: distinguish 'no events yet' from 'filter hides everything' in the empty state"
```

---

### Task 3: Search — distinguish the two "New rule" buttons' accessible names

**Files:**
- Modify: `siem-web/src/lib/components/QueryBar.svelte`
- Modify: `siem-web/src/lib/components/EventInspector.svelte`

**Interfaces:** none — visible text stays "New rule" in both places (per the
already-confirmed product decision from the prior branch); only an
`aria-label` is added to each, giving them distinct accessible names without
changing what's visibly rendered.

**Background:** both buttons read "New rule" (a deliberate, confirmed
decision), but seed the created rule differently — `QueryBar.svelte`'s scopes
to the whole current search query, `EventInspector.svelte`'s scopes to a
single selected event. Two controls with an identical accessible name on the
same page is an accessibility/testability gap: screen readers announce both
identically, and a future `getByRole('button', { name: 'New rule' })` in a
Playwright test would hit strict-mode ambiguity if both are on screen at
once (both render together whenever an event is selected in the inspector).

- [ ] **Step 1: `QueryBar.svelte`**

Current (line 72):

```svelte
	<button type="button" class="action" onclick={onAlertOnThis}>New rule</button>
```

Replace with:

```svelte
	<button type="button" class="action" onclick={onAlertOnThis} aria-label="New rule from this query">
		New rule
	</button>
```

- [ ] **Step 2: `EventInspector.svelte`**

Current (line 49):

```svelte
			<button onclick={() => onRuleFromThis(entry)}>New rule</button>
```

Replace with:

```svelte
			<button onclick={() => onRuleFromThis(entry)} aria-label="New rule from this event">
				New rule
			</button>
```

- [ ] **Step 3: Verify**

```bash
cd siem-web && pnpm exec vitest run && pnpm lint && pnpm exec svelte-check
```
Expected: clean.

Manually verify (dev server, browser devtools accessibility tree or a screen
reader): the visible text on both buttons still reads "New rule"; the
QueryBar one's accessible name is "New rule from this query" and the
EventInspector one's is "New rule from this event".

- [ ] **Step 4: Commit**

```bash
git add siem-web/src/lib/components/QueryBar.svelte siem-web/src/lib/components/EventInspector.svelte
git commit -m "Search: give the two same-labeled New rule buttons distinct accessible names"
```
