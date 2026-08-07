# Nav bar: show the real user picture from OIDC — design

Status: approved
Scope: replaces the placeholder avatar circle in `Nav.svelte` with the logged-in
user's actual profile picture, sourced from PocketID's OIDC `picture` claim.

## Context

PocketID (this deployment's OIDC provider) publishes a `picture` claim in the ID
token whose value is a directly-fetchable image URL (e.g.
`https://pocketid.townsville.cc/api/users/<sub>/profile-picture.png`) — this is
the standard OIDC claim, meant to be used directly as an `<img src>`, not proxied
or re-hosted. The only avatar placeholder anywhere in `siem-web` is a single
empty circle (`.avatar` in `Nav.svelte`); nothing else in the app currently shows
a user picture.

## Goals

- Replace that one placeholder with the real picture when available.
- No new siem-api endpoint, no server-side storage of the picture URL — thread
  it through the exact same request-scoped pipeline every other identity field
  (email, display name, groups) already flows through: OIDC claims → session
  JWT → `locals.user` → root layout → `Nav.svelte` prop.
- Fall back to today's plain placeholder circle whenever there's no picture to
  show: the claim was absent from the ID token, or the image fails to load
  (unreachable PocketID, broken URL, revoked/expired path, etc).

## Non-goals (this pass)

- Showing the picture anywhere else in the app (Sources' "claimed by", Alerts'
  "acked by", or any other per-user display). Explicitly deferred to a later
  pass — noted here so it's not forgotten, not because it's undesirable.
- Proxying or caching the image through siem-web/siem-api. The OIDC spec's
  `picture` claim is meant to be fetched directly by the browser.

## Design

**`siem-web/src/lib/server/oidc.ts`**
- `OidcClaims` gains `picture: string` (empty string when absent).
- `extractOidcClaims` reads `raw.picture` if it's a string, else `''` — same
  permissive-parsing style already used for `email`/`displayName`.

**`siem-web/src/lib/server/session.ts`**
- `SessionClaims` gains `picture: string`.
- `mintSessionToken` includes it in the signed JWT payload (`picture` claim,
  same flat style as `email`/`display_name`/`groups`/`role`).
- `verifySessionToken` reads it back (`(payload.picture as string) ?? ''`).

**`siem-web/src/routes/auth/callback/+server.ts`**
- Passes `claims.picture` through to `mintSessionToken`, same as the other
  claim fields already do.

**`siem-web/src/hooks.server.ts`**
- `locals.user` gains `picture`.

**`siem-web/src/routes/+layout.server.ts` / `+layout.svelte`**
- `data.user` already flows through wholesale from the server load; the root
  layout passes a new `userPicture={data.user?.picture ?? ''}` prop to `Nav`,
  matching the existing `userDisplayName`/`userRole` pattern exactly.

**`siem-web/src/lib/components/Nav.svelte`**
- New `userPicture: string` prop.
- The `.avatar` element becomes conditional: an `<img class="avatar" src={userPicture}>`
  when `userPicture` is non-empty, falling back to today's plain circle
  `<a class="avatar">` otherwise (no picture claim at all — e.g. any future
  non-OIDC login path).
- The `<img>` gets an `onerror` handler that swaps it for the placeholder
  circle at runtime, covering a picture URL that *was* present in the claim
  but fails to actually load (PocketID down, image deleted, network hiccup).
  Implemented with a small `$state` flag (`imgFailed`) flipped by `onerror`,
  rather than mutating the DOM directly, to stay idiomatic Svelte 5.
- Both the image and the fallback keep the same `28px` circular styling and
  stay wrapped in the logout link's clickable area exactly as today (clicking
  the avatar still logs out — unchanged behavior, only the visual differs).

## Testing

- Unit tests for `extractOidcClaims` (picture present / absent / non-string
  value) alongside the existing claim-extraction tests in `oidc.test.ts` (or
  wherever those live already).
- Unit tests for `mintSessionToken`/`verifySessionToken` round-tripping
  `picture` in `session.test.ts`.
- A Svelte component test for `Nav.svelte` (existing test file, if any, or new)
  covering: picture present → renders `<img>` with the right `src`; picture
  absent → renders the placeholder; `onerror` fires → falls back to the
  placeholder.

## Known gaps after this pass

- The picture isn't shown anywhere else in the app yet (Sources/Alerts
  attribution) — deferred, see Non-goals.
- No local caching/proxying — if PocketID is slow or briefly unreachable, the
  browser will show the fallback placeholder until the image loads, same as
  any other external image would behave.
