# siem-web: Settings → Authentication — design

Status: approved
Scope: sixth sub-project of the `siem-web` service (`design_handoff_homesiem/README.md`,
Screen 6 — Settings → Authentication), and the last remaining screen in the app. Replaces
the existing `/settings` page, which was built as a static visual mockup (hardcoded
example values transcribed from the design handoff's illustrative text, no data loading,
no wired actions) — see the review that preceded this spec for the full list of gaps.
Covers wiring the Group → role mapping panel to real data with real add/edit. The
OIDC provider panel and the Session & break-glass panel are explicitly dropped this
pass — see Decisions below. The other five settings tabs (Retention & storage,
Notifications, Parsers, Backups, About) remain separate future sub-projects, unchanged
from their current placeholder state.

## Context

`GET /settings/auth` and `PUT /settings/auth` (both `admin`+, already routed) already
exist from the original siem-api build. `GET` returns real OIDC issuer/client-ID/scope
config plus every role mapping (`{id, group_claim, role, priority}`) from the store,
ordered by `priority ASC` — "first match wins" is literally the store's own iteration
order (`store.ResolveRole`). `PUT` accepts a list of role mappings and upserts each one
by `group_claim` (`store.UpsertRoleMapping`'s `ON CONFLICT(group_claim) DO UPDATE`) — so
"add a new mapping" and "edit an existing one" are the same operation from the backend's
perspective, keyed entirely on `group_claim`. There is no delete endpoint, and no
member-count field anywhere in the API — group membership is only ever resolved
per-login from a user's JWT claims at that moment, never tracked in aggregate.

The existing `/settings/+page.svelte` has no `+page.server.ts`, no `SiemApiClient`
methods for this endpoint, and no wired actions — this sub-project builds all of that
for the first time, keeping the existing sidebar-nav shell and visual styling.

## Goals

- `+page.server.ts` loads real role mappings via `GET /settings/auth`.
- Group → role mapping table: **Group claim · Role · Can** (three columns, down from the
  mockup's four — see Decisions). "Can" is a fixed, client-side lookup by role name
  (`viewer`/`analyst`/`admin` are a closed enum matching `auth.RoleViewer` etc.), not
  per-row API data.
- Add/edit form: group claim (text, editable only when adding a new mapping — see
  Decisions), role (dropdown of the three valid roles). Submits to a new proxy route,
  which forwards to the existing `PUT /settings/auth`.
- New priority auto-assigned as `max(existing priorities) + 1` (evaluated last by
  default) — not an editable field this pass.
- New `SiemApiClient.getAuthSettings`/`updateRoleMappings` methods and a new
  `/api/settings/auth` proxy route (`admin`+ enforced server-side by the existing
  siem-api route, same as every other write action in this app).

Out of scope for this pass: the OIDC provider panel (issuer/client-ID/secret/scopes/
redirect-URI display and "Test connection"), the Session & break-glass panel (session
lifetime, local-admin status, audit-log retention), deleting a role mapping, editing an
existing mapping's priority/reordering, and the five non-Authentication settings tabs.

## Frontend structure

```text
siem-web/src/
  routes/
    settings/
      +page.server.ts   # load(): GET /settings/auth's role_mappings
      +page.svelte       # MODIFY (existing) — Authentication section's content
                          # becomes the real role-mapping table + add/edit form; sidebar
                          # nav and the other five tabs' placeholder state are untouched
    api/
      settings/
        auth/+server.ts   # thin PUT passthrough to siem-api's PUT /settings/auth
  lib/
    components/
      RoleMappingTable.svelte    # the table + inline "Edit" trigger per row
      RoleMappingForm.svelte     # shared add/edit form (group_claim read-only in edit
                                  # mode, per the Decisions section)
    settings.ts                  # roleCapabilityLabel(role): the fixed role→"Can"
                                  # lookup — the only pure logic this sub-project adds
```

## Data flow

`+page.server.ts` calls `client.getAuthSettings(token)` → `role_mappings`, passed to
`RoleMappingTable.svelte` (each row's "Can" column derived via `settings.ts`'s
`roleCapabilityLabel`). Clicking "+ Add mapping" opens `RoleMappingForm.svelte` empty;
clicking a row's "Edit" opens it pre-filled with that mapping's `group_claim`/`role`,
with `group_claim` rendered read-only. Submitting either mode `PUT`s the full list of
role mappings (existing ones from `data.roleMappings` plus the new/edited one) to
`/api/settings/auth`, then `invalidateAll()`s — same pattern Alerts already uses for its
ack/mute actions — so the table re-reflects the real, just-saved state from the server
rather than trusting client-side optimism.

## Error handling

`GET /settings/auth` failing (non-auth `SiemApiError`) 502s the page — it's this screen's
only real data source. A 401/403 redirects to `/auth/logout`, matching every other
screen. The add/edit form surfaces a save failure inline (same `error` state pattern
`AlertDetail.svelte`'s ack/mute already use) rather than failing silently.

## Testing

- Go: none needed — `GET`/`PUT /settings/auth` are unchanged, already tested.
- TDD (Vitest): `settings.ts`'s `roleCapabilityLabel` (the one pure function this
  sub-project adds); the new `SiemApiClient` methods; the new proxy route's
  auth-forwarding and error-propagation logic; a `+page.server.ts` load-function test
  (mirroring Alerts/Sources/Search's own load-function test conventions).
- No unit tests for the Svelte components, matching every other screen's convention.
- No new Playwright e2e.

## Known gaps for this pass

- **No delete** — removing a role mapping isn't possible from this screen; the backend
  has no delete endpoint either. A future pass would need both.
- **No priority editing/reordering** — new mappings always evaluate last; changing an
  existing mapping's evaluation order isn't possible from this screen.
- **OIDC provider panel dropped entirely** — issuer/client-ID/scopes are env-var-sourced
  at process startup with no DB-backed storage, and the client secret is never exposed
  by the API (correctly, for security). Making this panel real would require moving
  OIDC config into DB-backed settings — a genuine architecture change, not a frontend
  gap, and explicitly deferred.
- **Session & break-glass panel dropped entirely** — no backend endpoint exists for
  session lifetime or audit-log retention at all. "Local admin enabled" is technically
  derivable today (`LocalAdminUsername`/`LocalAdminPasswordHash` being set), but showing
  one real card next to two fabricated ones was judged worse than omitting the whole
  panel — deferred as a set, not piecemeal.
- The other five settings tabs (Retention & storage, Notifications, Parsers, Backups,
  About) remain the existing placeholder stub — unchanged, separate future sub-projects.

## Decisions carried from brainstorming

- OIDC provider panel: dropped entirely for this pass, not built read-only — confirmed
  with the user after establishing that making it truly editable needs a real backend
  architecture change (env-var config → DB-backed settings), and a read-only-but-inert
  panel wasn't worth keeping either.
- Session & break-glass panel: dropped entirely as a set, not shown with one real card
  (local admin enabled) next to two fabricated ones — confirmed with the user, same
  "don't show fabricated data" reasoning applied consistently.
- Role mapping: add + edit, explicitly no delete this pass — the backend already
  supports add/edit for free (both are the same upsert-by-`group_claim` operation);
  delete would need new backend work, deferred.
- "Members" column: dropped from the table — no backing data exists anywhere in the API,
  and none is planned (group membership is resolved per-login, never aggregated).
- "Can" column: kept, but reimplemented as a fixed client-side lookup by role name
  rather than per-row API data, since the three roles are a closed, unchanging enum.
- Editing a mapping keeps `group_claim` read-only (only `role` is editable) — a
  correctness constraint following directly from the backend's upsert-by-`group_claim`
  keying, not a UX preference: allowing `group_claim` edits would silently create a new
  mapping instead of renaming the old one.
