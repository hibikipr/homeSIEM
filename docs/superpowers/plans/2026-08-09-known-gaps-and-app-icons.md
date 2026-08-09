# Known Gaps and App Icons Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close v0.5.12's siem-api severity-validation gap, wire up the new
homeSIEM icon set as a real favicon/PWA manifest, and finish the remaining four
items from the original live GUI audit (Settings stub tabs, Live-tail empty
state, Search HOST tooltip, ambiguous rule-creation button labels).

**Architecture:** Six independent, small tasks spanning `siem-api` (Go, Task 1
only) and `siem-web` (SvelteKit, Tasks 2-6). No task depends on another's
output.

**Tech Stack:** Go/`net/http` (siem-api), SvelteKit, Svelte 5 runes,
TypeScript (siem-web).

## Global Constraints

- The only real alert/rule severity values are `info`/`warning`/`critical` —
  this plan is partly about closing the last place that doesn't enforce that.
- `siem-web` has no Svelte component test framework (established convention).
  UI-only tasks (2-6) are verified via `pnpm lint` / `pnpm exec svelte-check`
  plus manual/Playwright interaction, not new unit tests.
- `siem-api` changes ARE unit-testable (`go test ./...`) and Task 1 must add
  real test coverage for the new validation.
- Before every commit in a `siem-web` task: `pnpm exec vitest run`, `pnpm lint`,
  `pnpm exec svelte-check` must all be clean. Before every commit in the
  `siem-api` task: `go build ./... && go vet ./... && go test ./...` must be
  clean.
- Don't invent new backend support for anything explicitly being hidden
  (Task 3) — hiding is the fix, not a stub replacement.

---

### Task 1: siem-api — validate `severity` on rule create/update

**Files:**
- Modify: `siem-api/internal/api/rules.go`
- Modify: `siem-api/internal/api/rules_test.go`

**Interfaces:**
- Consumes: nothing new.
- Produces: `POST /rules` and `PUT /rules/{id}` now return `400` with body
  `severity must be one of: info, warning, critical` for any `severity` value
  outside that set. No response schema change for valid requests.

**Background:** `siem-api/internal/alerts/service.go:148-169` (`severityToPriority`,
`severityRank`) already assume the real vocabulary is `info`/`warning`/`critical`
and silently default anything else to the lowest tier — but the actual input
boundary, `handleCreateRule`/`handleUpdateRule` in `rules.go`, does no validation
at all. The project's own test suite uses the out-of-vocabulary value `"low"` as
an incidental fixture in four places (none of which are testing severity
itself), which this task also fixes.

- [ ] **Step 1: Write the failing test**

Add to `siem-api/internal/api/rules_test.go`, after `TestCreateRule_RequiresAnalyst`:

```go
func TestCreateRule_RejectsInvalidSeverity(t *testing.T) {
	s := newSchedulerTestServer(t)
	token := authToken(t, s.deps.Store, "analyst", 50)

	body := `{"name":"r","shape":"absence","severity":"low","destinations":["inapp"],"cooldown_sec":60,"interval_sec":60,"enabled":true}`
	req := httptest.NewRequest(http.MethodPost, "/rules", bytes.NewReader([]byte(body)))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body=%s", rec.Code, rec.Body.String())
	}
}

func TestUpdateRule_RejectsInvalidSeverity(t *testing.T) {
	s := newSchedulerTestServer(t)
	ctx := context.Background()
	token := authToken(t, s.deps.Store, "analyst", 50)

	created, err := s.deps.Store.CreateRule(ctx, store.Rule{
		Name: "r", Shape: "absence", Severity: "warning", Destinations: []string{"inapp"},
		CooldownSec: 60, IntervalSec: 60, Enabled: true,
	}, nil)
	if err != nil {
		t.Fatalf("CreateRule() error = %v", err)
	}

	body := `{"name":"r","shape":"absence","severity":"urgent","destinations":["inapp"],"cooldown_sec":60,"interval_sec":60,"enabled":true}`
	req := httptest.NewRequest(http.MethodPut, "/rules/"+itoa(created.ID), bytes.NewReader([]byte(body)))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body=%s", rec.Code, rec.Body.String())
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd siem-api && go test ./internal/api/... -run 'TestCreateRule_RejectsInvalidSeverity|TestUpdateRule_RejectsInvalidSeverity' -v`
Expected: both FAIL — `status = 201, want 400` and `status = 200, want 400`
(neither handler validates severity yet).

- [ ] **Step 3: Add the validation**

In `siem-api/internal/api/rules.go`, add this after the `ruleRequest` type
definition (before `func (rq ruleRequest) toStoreRule()`):

```go
var validSeverities = map[string]bool{"info": true, "warning": true, "critical": true}
```

In `handleCreateRule`, immediately after the JSON-decode error check (right
before the `userID, _, ok := auth.UserFromContext(...)` line), add:

```go
	if !validSeverities[req.Severity] {
		http.Error(w, "severity must be one of: info, warning, critical", http.StatusBadRequest)
		return
	}
```

Add the identical block to `handleUpdateRule`, in the same relative position
(immediately after its own JSON-decode error check, before the
`ruleToUpdate := req.toStoreRule()` line — validate `req.Severity` before it's
copied onto the update struct).

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd siem-api && go test ./internal/api/... -run 'TestCreateRule_RejectsInvalidSeverity|TestUpdateRule_RejectsInvalidSeverity' -v`
Expected: both PASS.

- [ ] **Step 5: Fix the four stale `"low"` fixtures**

None of these four tests are about severity — `"low"` was an incidental
placeholder value. Change each occurrence of `Severity: "low"` /
`"severity":"low"` to `Severity: "warning"` / `"severity":"warning"` in
`siem-api/internal/api/rules_test.go`:
- `TestUpdateRule_DisablesStopsScheduler` (currently line 101, direct
  `store.CreateRule` call — Go struct literal `Severity: "low"`)
- `TestUpdateRule_DisablesStopsScheduler`'s HTTP body (currently line 108,
  JSON string `"severity":"low"`)
- `TestDeleteRule` (currently line 150, direct `store.CreateRule` call —
  Go struct literal `Severity: "low"`)
- `TestListRules_OmittedArrayFieldsSerializeAsEmptyArrays` (currently line 195,
  JSON string `"severity":"low"`)

- [ ] **Step 6: Run the full test suite**

Run: `cd siem-api && go build ./... && go vet ./... && go test ./...`
Expected: all clean, all packages pass (the four fixture changes must not have
broken anything they weren't testing — if any of the four tests now fails,
that test was relying on the specific value `"low"` in a way this plan didn't
anticipate; stop and investigate rather than reverting the validation).

- [ ] **Step 7: Commit**

```bash
git add siem-api/internal/api/rules.go siem-api/internal/api/rules_test.go
git commit -m "siem-api: reject invalid severity on rule create/update"
```

---

### Task 2: App icons — favicon, apple-touch-icon, and PWA manifest

**Files:**
- Create: `siem-web/static/icons/homesiem-16.png`
- Create: `siem-web/static/icons/homesiem-32.png`
- Create: `siem-web/static/icons/homesiem-64.png`
- Create: `siem-web/static/icons/homesiem-180.png`
- Create: `siem-web/static/icons/homesiem-192.png`
- Create: `siem-web/static/icons/homesiem-512.png`
- Create: `siem-web/static/icons/homesiem-maskable-512.png`
- Create: `siem-web/static/manifest.webmanifest`
- Modify: `siem-web/src/app.html`
- Delete: `siem-web/src/lib/assets/favicon.svg`

**Interfaces:** none — pure static assets and one HTML template.

**Background:** the source images already exist, pre-sized, at
`/Users/hibikipr/Documents/GitHub/homeSIEM/design_handoff_homesiem/icons/`
(outside the `siem-web` package — this directory is not part of the git
worktree you're working in, it must be read from its absolute path on the
local filesystem and copied in). `siem-web/src/lib/assets/favicon.svg` is the
unmodified default SvelteKit scaffold icon (a Svelte logo) — confirmed unused
anywhere in the codebase (no `<link>`, no import, nothing references it) and
`siem-web/src/app.html` currently has no `<title>` tag and no favicon link at
all.

- [ ] **Step 1: Copy the icon files**

```bash
mkdir -p siem-web/static/icons
cp "/Users/hibikipr/Documents/GitHub/homeSIEM/design_handoff_homesiem/icons/homesiem-16.png" siem-web/static/icons/
cp "/Users/hibikipr/Documents/GitHub/homeSIEM/design_handoff_homesiem/icons/homesiem-32.png" siem-web/static/icons/
cp "/Users/hibikipr/Documents/GitHub/homeSIEM/design_handoff_homesiem/icons/homesiem-64.png" siem-web/static/icons/
cp "/Users/hibikipr/Documents/GitHub/homeSIEM/design_handoff_homesiem/icons/homesiem-180.png" siem-web/static/icons/
cp "/Users/hibikipr/Documents/GitHub/homeSIEM/design_handoff_homesiem/icons/homesiem-192.png" siem-web/static/icons/
cp "/Users/hibikipr/Documents/GitHub/homeSIEM/design_handoff_homesiem/icons/homesiem-512.png" siem-web/static/icons/
cp "/Users/hibikipr/Documents/GitHub/homeSIEM/design_handoff_homesiem/icons/homesiem-maskable-512.png" siem-web/static/icons/
```

Verify: `file siem-web/static/icons/*.png` should list seven PNGs at 16x16,
32x32, 64x64, 180x180, 192x192, 512x512, 512x512 respectively.

- [ ] **Step 2: Delete the unused default favicon**

```bash
rm siem-web/src/lib/assets/favicon.svg
```

(If `siem-web/src/lib/assets/` is now empty, leave the empty directory —
don't remove it as part of this task.)

- [ ] **Step 3: Create the manifest**

Create `siem-web/static/manifest.webmanifest`:

```json
{
	"name": "homeSIEM",
	"short_name": "homeSIEM",
	"description": "Self-hosted SIEM console",
	"start_url": "/",
	"display": "standalone",
	"background_color": "#131523",
	"theme_color": "#131523",
	"icons": [
		{ "src": "/icons/homesiem-192.png", "sizes": "192x192", "type": "image/png" },
		{ "src": "/icons/homesiem-512.png", "sizes": "512x512", "type": "image/png" },
		{
			"src": "/icons/homesiem-maskable-512.png",
			"sizes": "512x512",
			"type": "image/png",
			"purpose": "maskable"
		}
	]
}
```

- [ ] **Step 4: Wire up `app.html`**

Replace the full contents of `siem-web/src/app.html`:

```html
<!doctype html>
<html lang="en">
	<head>
		<meta charset="utf-8" />
		<meta name="viewport" content="width=device-width, initial-scale=1" />
		<meta name="text-scale" content="scale" />
		<title>homeSIEM</title>
		<link rel="icon" type="image/png" sizes="16x16" href="/icons/homesiem-16.png" />
		<link rel="icon" type="image/png" sizes="32x32" href="/icons/homesiem-32.png" />
		<link rel="icon" type="image/png" sizes="64x64" href="/icons/homesiem-64.png" />
		<link rel="apple-touch-icon" sizes="180x180" href="/icons/homesiem-180.png" />
		<link rel="manifest" href="/manifest.webmanifest" />
		<meta name="theme-color" content="#131523" />
		%sveltekit.head%
	</head>
	<body data-sveltekit-preload-data="hover">
		<div style="display: contents">%sveltekit.body%</div>
	</body>
</html>
```

- [ ] **Step 5: Verify**

```bash
cd siem-web && pnpm lint && pnpm exec svelte-check
```
Expected: clean.

Manually verify (dev server or `pnpm run build && pnpm run preview`): the
browser tab shows "homeSIEM" as the title and the new icon as the favicon;
`/manifest.webmanifest` and each `/icons/homesiem-*.png` URL load directly
(e.g. `curl -I http://localhost:4173/manifest.webmanifest` returns 200); no
404s for any of the linked assets in the browser's network tab.

- [ ] **Step 6: Commit**

```bash
git add siem-web/static/icons siem-web/static/manifest.webmanifest siem-web/src/app.html
git rm siem-web/src/lib/assets/favicon.svg
git commit -m "Add real favicon/PWA manifest, replacing the unused default SvelteKit icon"
```

---

### Task 3: Settings — hide stub tabs

**Files:**
- Modify: `siem-web/src/routes/settings/+page.svelte`

**Interfaces:** none.

**Background:** `retention`, `parsers`, `backups`, and `about` all currently
fall through to a generic stub branch ("This section is ready for the next set
of settings content.") with zero backend support. Confirmed decision: hide
these four from the sidebar entirely, matching the precedent already set by
removing the Wall dashboard's exposed "Retention: not yet available" tile
(v0.5.11) rather than leaving a visible placeholder.

- [ ] **Step 1: Narrow the section type and list**

Current (lines 9-21):

```svelte
	type SectionKey =
		'authentication' | 'retention' | 'notifications' | 'parsers' | 'backups' | 'about';

	let selectedSection = $state<SectionKey>('authentication');

	const sections: { key: SectionKey; label: string }[] = [
		{ key: 'authentication', label: 'Authentication' },
		{ key: 'retention', label: 'Retention & storage' },
		{ key: 'notifications', label: 'Notifications' },
		{ key: 'parsers', label: 'Parsers' },
		{ key: 'backups', label: 'Backups' },
		{ key: 'about', label: 'About' }
	];
```

Replace with:

```svelte
	type SectionKey = 'authentication' | 'notifications';

	let selectedSection = $state<SectionKey>('authentication');

	const sections: { key: SectionKey; label: string }[] = [
		{ key: 'authentication', label: 'Authentication' },
		{ key: 'notifications', label: 'Notifications' }
	];
```

- [ ] **Step 2: Remove the now-unreachable stub branch**

Current (lines 166-172, the tail end of the `{#if}` chain):

```svelte
			{/if}
		{:else}
			<div class="hero">
				<h1>{sections.find((section) => section.key === selectedSection)?.label}</h1>
				<p>This section is ready for the next set of settings content.</p>
			</div>
		{/if}
```

Replace with:

```svelte
			{/if}
		{/if}
```

(The outer `{#if selectedSection === 'authentication'} ... {:else if selectedSection === 'notifications'} ... {/if}` chain now covers every value `SectionKey` can hold, so the generic `{:else}` stub is dead code once the type is narrowed — remove it rather than leave an unreachable branch.)

- [ ] **Step 3: Verify**

```bash
cd siem-web && pnpm exec vitest run && pnpm lint && pnpm exec svelte-check
```
Expected: clean — `svelte-check` in particular should flag it if any other
part of the file still references a removed `SectionKey` value (it shouldn't,
since `openAddForm`/`openEditForm`/`selectSection` don't hardcode section keys).

Manually verify (dev server): `/settings` sidebar shows only "Authentication"
and "Notifications" — no "Retention & storage", "Parsers", "Backups", or
"About".

- [ ] **Step 4: Commit**

```bash
git add siem-web/src/routes/settings/+page.svelte
git commit -m "Settings: hide stub tabs (Retention, Parsers, Backups, About) with no backend support yet"
```

---

### Task 4: Live tail — empty state

**Files:**
- Modify: `siem-web/src/lib/components/TailViewport.svelte`

**Interfaces:** none.

**Background:** the `<tbody>`'s `{#each rendered as entry, i (i)}` has no
`{:else}` branch — a fresh tail with no events yet, or a severity filter that
matches nothing, renders a table with just the header row. Fix matches the
wording already established for this exact "streaming, currently empty"
situation on `Ticker.svelte` (Wall dashboard, v0.5.11): "Waiting for events…".

- [ ] **Step 1: Add the empty-state row**

Current (lines 114-127):

```svelte
			<tbody>
				{#each rendered as entry, i (i)}
					<tr>
						<td class="col-time mono">{entry.Timestamp}</td>
						<td class="col-severity">
							<span class="dot" style:background={severityColor(entry.Labels.severity ?? 'info')}
							></span>
						</td>
						<td class="col-host mono">{entry.Labels.host ?? ''}</td>
						<td class="col-program mono">{entry.Labels.program ?? ''}</td>
						<td class="col-facility mono">{entry.Labels.facility ?? ''}</td>
						<td class="mono message">{entry.Line}</td>
					</tr>
				{/each}
			</tbody>
```

Replace with:

```svelte
			<tbody>
				{#each rendered as entry, i (i)}
					<tr>
						<td class="col-time mono">{entry.Timestamp}</td>
						<td class="col-severity">
							<span class="dot" style:background={severityColor(entry.Labels.severity ?? 'info')}
							></span>
						</td>
						<td class="col-host mono">{entry.Labels.host ?? ''}</td>
						<td class="col-program mono">{entry.Labels.program ?? ''}</td>
						<td class="col-facility mono">{entry.Labels.facility ?? ''}</td>
						<td class="mono message">{entry.Line}</td>
					</tr>
				{:else}
					<tr class="empty-row">
						<td colspan="6">Waiting for events…</td>
					</tr>
				{/each}
			</tbody>
```

- [ ] **Step 2: Add the empty-row style**

Add to the `<style>` block, near the other `.col-*`/`.message` rules:

```css
	.empty-row td {
		text-align: center;
		white-space: normal;
		color: var(--color-muted-2);
		padding: var(--space-6);
	}
```

- [ ] **Step 3: Verify**

```bash
cd siem-web && pnpm lint && pnpm exec svelte-check
```
Expected: clean.

Manually verify (dev server, e.g. deselect every severity filter checkbox so
`rendered` is empty even with live traffic, or open the tail before any
events arrive): the table shows "Waiting for events…" centered in place of
rows, not a bare header.

- [ ] **Step 4: Commit**

```bash
git add siem-web/src/lib/components/TailViewport.svelte
git commit -m "Live tail: add empty-state message instead of a bare header row"
```

---

### Task 5: Search — HOST column tooltip

**Files:**
- Modify: `siem-web/src/lib/components/ResultTable.svelte`

**Interfaces:** none.

**Background:** `.col-host` has a fixed 88px width with ellipsis truncation
and no way to see the full value. A native `title` attribute is sufficient —
this table has no other custom tooltip infrastructure, and a single plain-text
value doesn't warrant building one.

- [ ] **Step 1: Add the tooltip**

Current (line 99):

```svelte
					<span class="col-host mono">{entry.Labels.host ?? ''}</span>
```

Replace with:

```svelte
					<span class="col-host mono" title={entry.Labels.host ?? ''}>{entry.Labels.host ?? ''}</span>
```

- [ ] **Step 2: Verify**

```bash
cd siem-web && pnpm lint && pnpm exec svelte-check
```
Expected: clean.

Manually verify (dev server): hovering a truncated HOST value in the Search
results table shows the full value in the browser's native tooltip.

- [ ] **Step 3: Commit**

```bash
git add siem-web/src/lib/components/ResultTable.svelte
git commit -m "Search: add title tooltip to the truncated HOST column"
```

---

### Task 6: Search — reconcile rule-creation button labels, remove disabled Save

**Files:**
- Modify: `siem-web/src/lib/components/QueryBar.svelte`
- Modify: `siem-web/src/lib/components/EventInspector.svelte`

**Interfaces:** none — only visible button text changes and one dead button's
removal; no prop/callback signatures change (`onAlertOnThis`/`onRuleFromThis`
keep their current names — renaming a callback prop is unrelated churn this
task doesn't need).

**Background:** two buttons on the Search screen open the identical
`RuleFromEventForm` ("Create rule") modal under different labels — "Alert on
this" (QueryBar, seeds the whole current query) and "Rule from this"
(EventInspector, seeds a single-event-scoped query) — with no shared
vocabulary between either label and the modal's own title. Confirmed decision:
rename both to "New rule". QueryBar also has a permanently-`disabled` "Save"
button for an unbuilt saved-searches feature; confirmed decision: remove it
(same "don't ship a visible non-functional stub" precedent as Task 3 and the
Wall's Retention tile).

- [ ] **Step 1: `QueryBar.svelte` — remove disabled Save, rename Alert on this**

Current (lines 72-75):

```svelte
	<button type="button" class="action" disabled title="Saved searches aren't built yet">
		Save
	</button>
	<button type="button" class="action" onclick={onAlertOnThis}>Alert on this</button>
```

Replace with:

```svelte
	<button type="button" class="action" onclick={onAlertOnThis}>New rule</button>
```

Do not rename the `onAlertOnThis` prop itself — only the visible button text
changes. The `.action:disabled` CSS rule (lines 146-149) becomes unused by
this file after the Save button is removed; leave it in place if any other
`.action` element could plausibly become disabled in the future is not a
concern here — YAGNI says remove genuinely dead CSS, so delete the
`.action:disabled { opacity: 0.5; cursor: not-allowed; }` rule too, since
`svelte-check`/the compiler will not flag unused CSS created by a markup
change in this way, but it is now provably dead (no other button in this file
uses `disabled`).

- [ ] **Step 2: `EventInspector.svelte` — rename Rule from this**

Current (line 49):

```svelte
			<button onclick={() => onRuleFromThis(entry)}>Rule from this</button>
```

Replace with:

```svelte
			<button onclick={() => onRuleFromThis(entry)}>New rule</button>
```

- [ ] **Step 3: Verify**

```bash
cd siem-web && pnpm exec vitest run && pnpm lint && pnpm exec svelte-check
```
Expected: clean.

Manually verify (dev server, Search screen): the QueryBar's action row now
shows only one button, "New rule" (no "Save"); selecting an event in the
inspector and using its own rule-creation button also now reads "New rule";
both still open the same "Create rule" modal, seeded as before (whole-query vs.
single-event-scoped).

- [ ] **Step 4: Commit**

```bash
git add siem-web/src/lib/components/QueryBar.svelte siem-web/src/lib/components/EventInspector.svelte
git commit -m "Search: rename both rule-creation buttons to 'New rule', remove disabled Save"
```
