# Known Gaps and App Icons — Design

**Status:** Approved
**Origin:** v0.5.12's "Known gaps" release notes (siem-api severity validation) plus the four remaining unstarted items from the original live GUI audit, plus new app icon assets the user added to `design_handoff_homesiem/icons/`.

## Goal

Close v0.5.12's two documented known gaps and the remaining four items from the
original GUI audit, and wire up a real favicon/PWA icon set (the app currently
ships the default, unmodified SvelteKit scaffold favicon — a Svelte logo — with
no `<link rel="icon">` anywhere referencing it, no PWA manifest, and no
`<title>` tag).

## 1. siem-api: validate `severity` on rule create/update

**Background:** `siem-api`'s real severity vocabulary is `info`/`warning`/`critical`
(already assumed by `severityRank`/`severityToPriority` in
`siem-api/internal/alerts/service.go:148-169`, which default any unrecognized
value to the lowest tier). `handleCreateRule`/`handleUpdateRule`
(`siem-api/internal/api/rules.go`) accept `Severity string` with zero validation
— any string round-trips through to storage. The project's own test suite
(`rules_test.go:101,108,150,195`) uses `"low"`, an out-of-vocabulary value, as an
incidental fixture — this is the exact class of bug (stale/invalid severity
strings) fixed repeatedly on the frontend this project, now closed at the one
real input boundary on the backend.

**Fix:** add a package-level `validSeverities = map[string]bool{"info": true,
"warning": true, "critical": true}` in `rules.go`, and check `req.Severity`
against it in both `handleCreateRule` and `handleUpdateRule` immediately after
JSON decoding, returning `400` with a clear message
(`"severity must be one of: info, warning, critical"`) on failure — same pattern
already used for the `invalid json body`/`invalid rule id` 400s in the same file.
Update the four `"low"` fixtures in `rules_test.go` to `"warning"` (none of those
four tests are actually about severity — it was an incidental placeholder value —
so this doesn't change what they verify) and add one new test asserting the 400
on an invalid severity.

## 2. App icons: favicon + PWA manifest

**Background:** the user added a complete, pre-sized icon set to
`design_handoff_homesiem/icons/` (a directory outside `siem-web`, not tracked by
git in this worktree — it must be copied in, not referenced in place):
`homesiem-16.png`, `homesiem-32.png`, `homesiem-64.png`, `homesiem-180.png`
(apple-touch-icon size), `homesiem-192.png` and `homesiem-512.png` (PWA manifest
sizes), `homesiem-maskable-512.png` (Android adaptive icon, already correctly
padded to the safe-zone spec), plus three SVG variants
(`homesiem-icon.svg`/`-light.svg`/`-transparent.svg`, not used by this task —
the PNG set covers every size this task needs).

**Fix:**
- Copy the seven PNGs into `siem-web/static/icons/` (SvelteKit's convention for
  static assets referenced by absolute URL).
- Delete `siem-web/src/lib/assets/favicon.svg` — confirmed unused anywhere in
  the codebase (it's the unmodified default SvelteKit scaffold icon, a Svelte
  logo, never wired up).
- Create `siem-web/static/manifest.webmanifest`:
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
  (`#131523` is `--color-bg`, matching the icons' own dark background — verified
  in `siem-web/src/lib/styles/tokens.css:3`.)
- Update `siem-web/src/app.html`: add a `<title>homeSIEM</title>` (currently
  missing entirely — every tab shows a blank title today), favicon links for
  16/32/64px, an apple-touch-icon link (180px), the manifest link, and a
  `theme-color` meta tag.

## 3. Settings: hide stub tabs

**Decision (confirmed):** hide `retention`, `parsers`, `backups`, and `about`
from the sidebar entirely, leaving only `authentication` and `notifications` —
matching the precedent already set on the Wall dashboard, where the exposed
"Retention: not yet available" placeholder tile was removed outright (v0.5.11)
rather than left visible. None of the four have any backend support today
(`+page.server.ts`'s loader only fetches auth settings and notification
settings). This is a straightforward filter of the `sections` array and its
render branches — no new content is invented for the hidden tabs, and re-adding
any of them once real content exists is a one-line revert.

## 4. Live tail: empty state

**Background:** `TailViewport.svelte` has no `{#if rendered.length === 0}`
branch anywhere — a fresh tail with no events yet, or a severity filter that
matches nothing, renders a table with just the header row and zero body rows,
with no indication of why.

**Fix:** add an empty-state message below the table (or in place of the body,
implementer's judgment on exact placement — the requirement is that it's
visible whenever `rendered.length === 0`), text "Waiting for events…" — matching
the exact tone and wording already established for this same "streaming,
currently empty" situation on the Wall dashboard's `Ticker.svelte` (v0.5.11).

## 5. Search: HOST column tooltip

**Background:** `ResultTable.svelte`'s `.col-host` span
(`{entry.Labels.host ?? ''}`) has a fixed 88px width with `text-overflow:
ellipsis` and no `title` attribute or any other way to see the full value when
truncated.

**Fix:** add `title={entry.Labels.host ?? ''}` to the host `<span>` — the
browser's native tooltip is sufficient here (this table already has no other
custom tooltip infrastructure, unlike the Wall's `HeatGrid`, which needed a real
positioned tooltip for richer content; a single plain-text value doesn't
warrant that complexity). Scoped to the HOST column only, matching what the
audit called out — `.col-program` has the identical gap but is out of scope
for this task.

## 6. Search: reconcile ambiguous rule-creation labels

**Background:** two buttons on the Search screen open the identical
`RuleFromEventForm` ("Create rule") modal under different labels — QueryBar's
"Alert on this" (seeds the whole current query) and EventInspector's "Rule from
this" (seeds a single-event-scoped query) — with no shared vocabulary between
either label and the modal's own title. Additionally, QueryBar has a
permanently-`disabled` "Save" button for a not-yet-built saved-searches feature,
whose only affordance is a hover `title` tooltip explaining it doesn't work.

**Decisions (confirmed):**
- Rename both rule-creation entry points to **"New rule"** — one consistent
  label for the same action in both places (`QueryBar.svelte`'s `onclick={onAlertOnThis}`
  button and `EventInspector.svelte`'s `onclick={() => onRuleFromThis(entry)}`
  button). No change to either button's wiring/behavior — only the visible text.
- Remove the disabled "Save" button from `QueryBar.svelte` entirely — same
  "don't ship a visible non-functional stub" precedent as item 3 above and the
  Wall's Retention tile. Trivial to re-add once saved searches are real.

## Testing

- siem-api: `go test ./...` covering the new validation (a table-driven test in
  `rules_test.go` asserting 400 for an invalid severity, 201/200 for each of the
  three valid values) plus the four fixture updates.
- siem-web: this project has no Svelte component test framework (established
  convention). Items 2-6 are verified via `pnpm lint` / `pnpm exec svelte-check`
  plus manual/Playwright interaction, matching every prior UI-only task this
  project has shipped.

## Out of scope

- No new backend support for Retention/Parsers/Backups/About content — those
  tabs are hidden, not built out.
- `.col-program`'s identical tooltip gap (item 5 is HOST-only, per the audit).
- No further changes to `RoleMappingForm.svelte`'s "Save" button — it's real,
  functioning, and unambiguous on its own; the ambiguity was cross-screen
  comparison with the now-removed disabled "Save," which this task resolves by
  removing the other one.
