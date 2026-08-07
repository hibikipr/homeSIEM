# Nav bar user picture Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the placeholder avatar circle in the Nav bar with the logged-in user's real profile picture, sourced from PocketID's OIDC `picture` claim.

**Architecture:** Thread `picture` through the exact same request-scoped pipeline every other identity field (email, display name, groups) already flows through — OIDC claims → session JWT → `locals.user` → root layout → a new `Nav.svelte` prop. No new siem-api endpoint, no server-side storage, no proxying: the OIDC `picture` claim is a directly-fetchable image URL by spec, rendered as a plain `<img src>`.

**Tech Stack:** SvelteKit (Svelte 5 runes), TypeScript, `jose` (JWT), `openid-client`, Vitest.

## Global Constraints

- No new siem-api endpoint or database column — the picture URL lives only in the session JWT, identical in kind to `email`/`display_name`/`groups`.
- No image proxying/caching — render the PocketID URL directly in an `<img src>`.
- `picture` is a required (non-optional) `string` field everywhere it's threaded through (`OidcClaims`, `SessionClaims`, `Locals.user`), defaulting to `''` when the OIDC provider doesn't supply it — matching this codebase's existing convention of required-with-empty-default fields (`email`, `displayName`) rather than optional/`undefined` fields.
- This codebase has no Svelte component test infrastructure currently (removed twice already: PR #18 dropped an unused `jsdom` devDependency that had been added for exactly this purpose, and PR #19's Settings rewrite replaced its component test with a plain SSR string-render, later superseded again). Do not add `@testing-library/svelte`, `jsdom`, or any new component-test setup. Cover the new logic with plain Vitest unit tests on the non-Svelte pieces (`oidc.ts`, `session.ts`, `hooks.server.ts`) exactly as the existing test suite already does; verify `Nav.svelte`'s rendering manually via the dev server in a real browser before calling the UI task done.
- Explicitly out of scope this pass: showing the picture anywhere else in the app (Sources "claimed by", Alerts "acked by"). Noted as a known follow-up, not forgotten — do not implement it.

---

### Task 1: Thread the `picture` claim through OIDC → session JWT → `locals.user`

**Files:**
- Modify: `siem-web/src/lib/server/oidc.ts`
- Modify: `siem-web/src/lib/server/oidc.test.ts`
- Modify: `siem-web/src/lib/server/session.ts`
- Modify: `siem-web/src/lib/server/session.test.ts`
- Modify: `siem-web/src/routes/auth/callback/+server.ts`
- Modify: `siem-web/src/hooks.server.ts`
- Modify: `siem-web/src/hooks.server.test.ts`
- Modify: `siem-web/src/app.d.ts`

**Interfaces:**
- Produces: `OidcClaims.picture: string`, `SessionClaims.picture: string`, `App.Locals['user'].picture: string` — all required, `''` when absent. Task 2 reads `data.user?.picture` in `+layout.svelte`.

- [ ] **Step 1: Write the failing tests for `extractOidcClaims`**

Add to `siem-web/src/lib/server/oidc.test.ts`, inside the existing `describe('extractOidcClaims', ...)` block (after the last existing `it`):

```ts
	it('maps the picture claim when present', () => {
		const claims = extractOidcClaims({
			sub: 'oidc-sub-1',
			email: 'alice@townsville.cc',
			name: 'Alice',
			picture: 'https://pocketid.townsville.cc/api/users/oidc-sub-1/profile-picture.png'
		});
		expect(claims.picture).toBe(
			'https://pocketid.townsville.cc/api/users/oidc-sub-1/profile-picture.png'
		);
	});

	it('defaults picture to an empty string when absent or non-string', () => {
		expect(extractOidcClaims({ sub: 'oidc-sub-1' }).picture).toBe('');
		expect(extractOidcClaims({ sub: 'oidc-sub-1', picture: 42 }).picture).toBe('');
	});
```

Also update the first test (`'maps a full claims object'`) so its `toEqual` still matches once `picture` becomes a real field on the returned object — change its `expect(claims).toEqual({...})` block to:

```ts
		expect(claims).toEqual({
			sub: 'oidc-sub-1',
			email: 'alice@townsville.cc',
			displayName: 'Alice',
			groups: ['siem-analysts', 'homelab'],
			picture: ''
		});
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd siem-web && pnpm exec vitest run src/lib/server/oidc.test.ts`
Expected: FAIL — `claims.picture` is `undefined`, and the updated `toEqual` block fails because the real return value has no `picture` key yet.

- [ ] **Step 3: Add `picture` to `OidcClaims` and `extractOidcClaims`**

In `siem-web/src/lib/server/oidc.ts`, change the `OidcClaims` interface:

```ts
export interface OidcClaims {
	sub: string;
	email: string;
	displayName: string;
	groups: string[];
	picture: string;
}
```

And in `extractOidcClaims`, add the extraction (same permissive-parsing style as `email`/`displayName`) and include it in the returned object:

```ts
export function extractOidcClaims(raw: Record<string, unknown>): OidcClaims {
	if (typeof raw.sub !== 'string') {
		throw new Error('ID token missing sub claim');
	}
	const groups = Array.isArray(raw.groups)
		? raw.groups.filter((g): g is string => typeof g === 'string')
		: [];
	const email = typeof raw.email === 'string' ? raw.email : '';
	const displayName = typeof raw.name === 'string' ? raw.name : email || raw.sub;
	const picture = typeof raw.picture === 'string' ? raw.picture : '';

	return { sub: raw.sub, email, displayName, groups, picture };
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd siem-web && pnpm exec vitest run src/lib/server/oidc.test.ts`
Expected: PASS

- [ ] **Step 5: Write the failing test for `mintSessionToken`/`verifySessionToken` round-tripping `picture`**

In `siem-web/src/lib/server/session.test.ts`, update `testClaims` to include `picture` (it's a required field on `SessionClaims` after Step 6, so this file won't compile without it regardless — doing it now makes the "round-trips claims" test exercise the new field too):

```ts
const testClaims: SessionClaims = {
	sub: 'oidc-sub-1',
	userId: 42,
	email: 'alice@townsville.cc',
	displayName: 'Alice',
	groups: ['siem-analysts'],
	role: 'analyst',
	picture: 'https://pocketid.townsville.cc/api/users/oidc-sub-1/profile-picture.png'
};
```

The existing `'round-trips claims'` test already does `expect(claims).toEqual(testClaims)`, so no new test case is needed — updating `testClaims` is sufficient to cover the new field through the existing round-trip assertion.

- [ ] **Step 6: Run the test to verify it fails**

Run: `cd siem-web && pnpm exec vitest run src/lib/server/session.test.ts`
Expected: FAIL to compile (`SessionClaims` doesn't have a `picture` field yet, so the `testClaims` literal has an excess property) — or, once Step 3's sibling change makes `picture` compile-required elsewhere, a runtime mismatch on the round-trip. Either way, confirm it doesn't currently pass before moving on.

- [ ] **Step 7: Add `picture` to `SessionClaims`, `mintSessionToken`, `verifySessionToken`**

In `siem-web/src/lib/server/session.ts`:

```ts
export interface SessionClaims {
	sub: string;
	userId: number;
	email: string;
	displayName: string;
	groups: string[];
	role: string;
	picture: string;
}
```

```ts
export async function mintSessionToken(claims: SessionClaims, secret: Uint8Array): Promise<string> {
	return new SignJWT({
		user_id: claims.userId,
		email: claims.email,
		display_name: claims.displayName,
		groups: claims.groups,
		role: claims.role,
		picture: claims.picture
	})
		.setProtectedHeader({ alg: 'HS256' })
		.setSubject(claims.sub)
		.setIssuedAt()
		.setExpirationTime('12h')
		.sign(secret);
}
```

```ts
export async function verifySessionToken(
	token: string,
	secret: Uint8Array
): Promise<SessionClaims> {
	const { payload } = await jwtVerify(token, secret, { algorithms: ['HS256'] });

	if (typeof payload.sub !== 'string') {
		throw new Error('session token missing sub claim');
	}

	return {
		sub: payload.sub,
		userId: payload.user_id as number,
		email: payload.email as string,
		displayName: payload.display_name as string,
		groups: (payload.groups as string[]) ?? [],
		role: payload.role as string,
		picture: (payload.picture as string) ?? ''
	};
}
```

- [ ] **Step 8: Run the test to verify it passes**

Run: `cd siem-web && pnpm exec vitest run src/lib/server/session.test.ts`
Expected: PASS

- [ ] **Step 9: Update `hooks.server.test.ts`'s inline claims literal**

`SessionClaims` is now required to include `picture`, so the inline object passed to `mintSessionToken` in the last test case (`'attaches locals.user and locals.sessionToken...'`) won't compile without it. In `siem-web/src/hooks.server.test.ts`, update that call:

```ts
		const token = await mintSessionToken(
			{
				sub: 'oidc-sub-1',
				userId: 42,
				email: 'alice@townsville.cc',
				displayName: 'Alice',
				groups: ['siem-analysts'],
				role: 'analyst',
				picture: 'https://pocketid.townsville.cc/api/users/oidc-sub-1/profile-picture.png'
			},
			secret
		);
```

And extend the existing `toMatchObject` assertion to cover it:

```ts
		expect(locals.user).toMatchObject({
			userId: 42,
			displayName: 'Alice',
			role: 'analyst',
			picture: 'https://pocketid.townsville.cc/api/users/oidc-sub-1/profile-picture.png'
		});
```

- [ ] **Step 10: Wire `picture` through `hooks.server.ts` and `app.d.ts`**

In `siem-web/src/app.d.ts`, add `picture` to the inline `Locals.user` type:

```ts
			user?: {
				userId: number;
				email: string;
				displayName: string;
				groups: string[];
				role: string;
				picture: string;
			};
```

In `siem-web/src/hooks.server.ts`, add it to the `locals.user` assignment:

```ts
		event.locals.user = {
			userId: claims.userId,
			email: claims.email,
			displayName: claims.displayName,
			groups: claims.groups,
			role: claims.role,
			picture: claims.picture
		};
```

- [ ] **Step 11: Wire `picture` through the OIDC callback route**

In `siem-web/src/routes/auth/callback/+server.ts`, add `picture` to the `mintSessionToken` call (it already has `claims.picture` available from `completeLogin`'s return value — no other change needed in this file):

```ts
	const token = await mintSessionToken(
		{
			sub: claims.sub,
			userId: session.user_id,
			email: claims.email,
			displayName: session.display_name,
			groups: claims.groups,
			role: session.role,
			picture: claims.picture
		},
		secret
	);
```

- [ ] **Step 12: Run the full siem-web test suite and type-check**

Run: `cd siem-web && pnpm exec vitest run && pnpm exec svelte-check`
Expected: All tests PASS, no new type errors. (`svelte-check` will still report this repo's pre-existing, unrelated warnings if any — only confirm no *new* errors from these changes.)

- [ ] **Step 13: Commit**

```bash
git add siem-web/src/lib/server/oidc.ts siem-web/src/lib/server/oidc.test.ts \
  siem-web/src/lib/server/session.ts siem-web/src/lib/server/session.test.ts \
  siem-web/src/routes/auth/callback/+server.ts \
  siem-web/src/hooks.server.ts siem-web/src/hooks.server.test.ts \
  siem-web/src/app.d.ts
git commit -m "Thread OIDC picture claim through session JWT to locals.user"
```

---

### Task 2: Render the real picture in `Nav.svelte`

**Files:**
- Modify: `siem-web/src/routes/+layout.svelte`
- Modify: `siem-web/src/lib/components/Nav.svelte`

**Interfaces:**
- Consumes: `data.user?.picture` (from Task 1's `App.Locals['user']`, flows through `+layout.server.ts`'s existing `return { user: locals.user, ... }` unchanged — no edit needed there).
- Produces: `Nav.svelte`'s new `userPicture: string` prop.

- [ ] **Step 1: Pass `userPicture` from the root layout**

In `siem-web/src/routes/+layout.svelte`, add the new prop to the `<Nav>` call:

```svelte
<Nav
	activeRoute={data.activeRoute}
	alertCount={0}
	ingestRate={0}
	userDisplayName={data.user?.displayName ?? ''}
	userRole={data.user?.role ?? ''}
	userPicture={data.user?.picture ?? ''}
/>
```

- [ ] **Step 2: Add the `userPicture` prop and fallback state to `Nav.svelte`**

In `siem-web/src/lib/components/Nav.svelte`, update the props block:

```svelte
	let {
		activeRoute,
		alertCount,
		ingestRate,
		userDisplayName,
		userRole,
		userPicture
	}: {
		activeRoute: string;
		alertCount: number;
		ingestRate: number;
		userDisplayName: string;
		userRole: string;
		userPicture: string;
	} = $props();

	let imgFailed = $state(false);
```

- [ ] **Step 3: Replace the placeholder avatar markup**

Replace this line:

```svelte
			<a href={resolve('/auth/logout')} class="avatar" aria-label="Log out"></a>
```

with:

```svelte
			<a href={resolve('/auth/logout')} class="avatar-link" aria-label="Log out">
				{#if userPicture && !imgFailed}
					<img class="avatar" src={userPicture} alt="" onerror={() => (imgFailed = true)} />
				{:else}
					<span class="avatar"></span>
				{/if}
			</a>
```

- [ ] **Step 4: Update the CSS**

Replace the existing `.avatar` rule:

```css
	.avatar {
		width: 28px;
		height: 28px;
		border-radius: 50%;
		background: var(--color-line-2);
		display: inline-block;
	}
```

with:

```css
	.avatar-link {
		display: inline-block;
		line-height: 0;
	}
	.avatar {
		width: 28px;
		height: 28px;
		border-radius: 50%;
		background: var(--color-line-2);
		display: inline-block;
	}
	img.avatar {
		object-fit: cover;
	}
```

- [ ] **Step 5: Manually verify in a real browser**

This codebase has no Svelte component test infrastructure (see Global Constraints) — verify by hand:

1. Run: `cd siem-web && pnpm dev`
2. Log in through the real OIDC flow against the dev PocketID instance (or whatever the local dev auth setup is).
3. Confirm the Nav bar shows the real profile picture, cropped into the circle (not stretched).
4. Temporarily edit `userPicture` in `+layout.svelte` to an intentionally broken URL (e.g. append `-broken` to the path), reload, and confirm it falls back to the plain placeholder circle rather than a broken-image icon. Revert the temporary edit afterward.
5. Confirm the avatar (in either state) still logs the user out when clicked.

- [ ] **Step 6: Run the full siem-web test suite and type-check**

Run: `cd siem-web && pnpm exec vitest run && pnpm exec svelte-check`
Expected: All tests PASS, no new type errors.

- [ ] **Step 7: Commit**

```bash
git add siem-web/src/routes/+layout.svelte siem-web/src/lib/components/Nav.svelte
git commit -m "Show the real OIDC profile picture in the Nav bar avatar"
```
