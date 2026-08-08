# Nav bar: account dropdown menu instead of instant sign-out — design

Status: approved
Scope: replaces the Nav bar avatar's direct `<a href="/auth/logout">` link with
a proper account dropdown menu, fixing a real accidental-sign-out bug found
during a live GUI audit.

## Context

`Nav.svelte`'s avatar is currently a plain link straight to `/auth/logout`.
That route (`routes/auth/logout/+server.ts`) deletes the session cookie
*unconditionally* before redirecting to the OIDC provider's logout URL — so a
single accidental click on the avatar ends the local session immediately,
with zero warning from the app itself. The identity provider's own logout
page may show its own confirmation, but by then the `siem_session` cookie is
already gone; declining there doesn't undo it.

## Goals

- Clicking the avatar opens a dropdown instead of navigating anywhere.
- The dropdown shows read-only identity info (picture, display name, email,
  role) and a single "Sign out" action that performs the actual logout
  navigation.
- Closes on click-outside or Escape, matching standard dropdown behavior.
- Two deliberate clicks (open menu, then Sign out) replaces the single-click
  accident, without adding a separate confirm dialog on top of that.

## Non-goals (this pass)

- No "Profile" or "Settings" links in the dropdown — this app has no
  self-service profile page (identity is fully delegated to PocketID), and
  Settings is already a separate, correctly-gated main-nav item for admins.
  Explicitly decided against a link out to PocketID's own account page too.
- No changes to `/auth/logout`'s own behavior (still deletes the cookie then
  redirects) — only how it gets triggered from the Nav bar.

## Design

**`siem-web/src/hooks.server.ts`**
- `locals.user` gains `email` — the field already exists on `SessionClaims`
  and is already verified from the session JWT, just not currently copied
  into `locals.user` (`displayName`/`role`/`picture` all already are).

**`siem-web/src/app.d.ts`**
- `Locals.user` type gains `email: string`, matching the pattern the
  `picture` field addition already established.

**`siem-web/src/routes/+layout.svelte`**
- Passes a new `userEmail={data.user?.email ?? ''}` prop to `<Nav>`, same
  pattern as `userDisplayName`/`userRole`/`userPicture`.

**`siem-web/src/lib/components/Nav.svelte`**
- New `userEmail: string` prop.
- New `let menuOpen = $state(false)`.
- The avatar element becomes a `<button type="button">` (not `<a>`) that
  toggles `menuOpen`, with `aria-haspopup="true"` and
  `aria-expanded={menuOpen}`.
- A dropdown panel (`{#if menuOpen}`) positioned below the avatar, containing:
  - The same picture/placeholder (larger), display name, email, and role.
  - A "Sign out" button/link that navigates to `/auth/logout` (a real link,
    not a fetch — the existing route is a plain `GET` redirect chain to the
    OIDC provider, which needs a real navigation).
- Closing behavior: a `svelte:window onclick` listener that closes the menu
  when the click target is outside both the button and the panel (checked
  via a bound element reference, not a CSS class check), plus an `onkeydown`
  handler on the button/panel that closes on `Escape` and returns focus to
  the avatar button.
- CSS: dropdown panel styled consistently with this app's existing
  `.panel`/surface tokens (`--color-surface-2`, `--shadow-flat`,
  `--radius-default`) rather than inventing a new visual language.

## Testing

- This is a `.svelte`-only change with interactive/focus behavior — per this
  codebase's established constraint (no component-test infrastructure, see
  the Nav-avatar-picture plan's Global Constraints), verify manually via
  Playwright with a minted session cookie: open the menu, confirm identity
  info renders, confirm click-outside closes it, confirm Escape closes it,
  confirm Sign out still reaches `/auth/logout`.
- No new automated test needed for `hooks.server.ts`'s `email` addition
  beyond extending the existing test's assertions (mirroring how `picture`
  was added there).

## Known gaps after this pass

- No focus trap inside the open dropdown (Tab can leave it) — a reasonable
  scope line for this app's actual usage (a small home-SIEM console, not a
  public-facing product), matching the level of accessibility effort already
  present elsewhere in this codebase (e.g. `RoleMappingForm`'s modal).
