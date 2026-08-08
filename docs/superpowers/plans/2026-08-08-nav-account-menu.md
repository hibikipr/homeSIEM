# Nav bar account dropdown menu Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the Nav bar avatar's direct `<a href="/auth/logout">` link — which deletes the session cookie unconditionally on a single click, before the OIDC provider's own confirmation page even loads — with a dropdown menu showing read-only identity info and a single "Sign out" action.

**Architecture:** Pure `siem-web` frontend change. `locals.user.email` already exists (verified: `hooks.server.ts` already copies `claims.email` into it, and `app.d.ts`'s `Locals.user` type already includes it) — it's just never been threaded down to `Nav.svelte` as a prop. The only backend-adjacent change is one new prop pass-through in `+layout.svelte`; everything else is local `$state` and markup inside `Nav.svelte`.

**Tech Stack:** SvelteKit (Svelte 5 runes), TypeScript.

## Global Constraints

- No "Profile" or "Settings" links in the dropdown — only identity info (picture, display name, email, role) and "Sign out". This app has no self-service profile page and Settings is already a correctly-gated main-nav item.
- `/auth/logout`'s own behavior is unchanged — still a real navigation (an `<a href>`, not a `fetch`) to the existing `routes/auth/logout/+server.ts`, which deletes the cookie then redirects to the OIDC provider.
- This codebase has no Svelte component test infrastructure and none should be added. Verify manually via Playwright with a minted session cookie (real OIDC login isn't available in this sandbox), same technique used in the Nav-avatar-picture and Settings-Notifications plans.
- No focus trap inside the open menu — Tab can leave it. This is an accepted scope line for this app (a small home-SIEM console), matching the accessibility effort level already present elsewhere (e.g. `RoleMappingForm`'s modal has no focus trap either).

---

### Task 1: Account dropdown menu

**Files:**
- Modify: `siem-web/src/routes/+layout.svelte`
- Modify: `siem-web/src/lib/components/Nav.svelte`

**Interfaces:**
- Consumes: `data.user?.email` — already available on `App.Locals['user']` and threaded through `+layout.server.ts`'s existing `return { user: locals.user, ... }` unchanged (no edit needed there, same as `picture` needed none).

- [ ] **Step 1: Pass `userEmail` from the root layout**

In `siem-web/src/routes/+layout.svelte`, add the new prop to the `<Nav>` call:

```svelte
<Nav
	activeRoute={data.activeRoute}
	alertCount={0}
	ingestRate={0}
	userDisplayName={data.user?.displayName ?? ''}
	userEmail={data.user?.email ?? ''}
	userRole={data.user?.role ?? ''}
	userPicture={data.user?.picture ?? ''}
/>
```

- [ ] **Step 2: Add the `userEmail` prop and menu state to `Nav.svelte`**

In `siem-web/src/lib/components/Nav.svelte`, update the props block and add menu state:

```svelte
	let {
		activeRoute,
		alertCount,
		ingestRate,
		userDisplayName,
		userEmail,
		userRole,
		userPicture
	}: {
		activeRoute: string;
		alertCount: number;
		ingestRate: number;
		userDisplayName: string;
		userEmail: string;
		userRole: string;
		userPicture: string;
	} = $props();

	let imgFailed = $state(false);
	let menuOpen = $state(false);
	let menuEl: HTMLDivElement;
	let menuButtonEl: HTMLButtonElement;

	function toggleMenu() {
		menuOpen = !menuOpen;
	}

	function closeMenu() {
		menuOpen = false;
	}

	function handleWindowClick(event: MouseEvent) {
		if (!menuOpen) return;
		const target = event.target as Node;
		if (menuEl?.contains(target) || menuButtonEl?.contains(target)) return;
		closeMenu();
	}

	function handleMenuKeydown(event: KeyboardEvent) {
		if (event.key === 'Escape') {
			closeMenu();
			menuButtonEl?.focus();
		}
	}
```

- [ ] **Step 3: Add the window click listener**

In `siem-web/src/lib/components/Nav.svelte`, add directly above the `<header class="nav">` element:

```svelte
<svelte:window onclick={handleWindowClick} />
```

- [ ] **Step 4: Replace the avatar markup with the button + dropdown**

Replace this block:

```svelte
		<a href={resolve('/auth/logout')} class="avatar-link" aria-label="Log out">
			{#if userPicture && !imgFailed}
				<img class="avatar" src={userPicture} alt="" onerror={() => (imgFailed = true)} />
			{:else}
				<span class="avatar"></span>
			{/if}
		</a>
```

with:

```svelte
		<div class="account">
			<button
				bind:this={menuButtonEl}
				type="button"
				class="avatar-button"
				aria-haspopup="true"
				aria-expanded={menuOpen}
				aria-label="Account menu"
				onclick={toggleMenu}
				onkeydown={handleMenuKeydown}
			>
				{#if userPicture && !imgFailed}
					<img class="avatar" src={userPicture} alt="" onerror={() => (imgFailed = true)} />
				{:else}
					<span class="avatar"></span>
				{/if}
			</button>
			{#if menuOpen}
				<div bind:this={menuEl} class="account-menu" role="menu" onkeydown={handleMenuKeydown}>
					<div class="account-menu-identity">
						{#if userPicture && !imgFailed}
							<img class="avatar avatar-large" src={userPicture} alt="" onerror={() => (imgFailed = true)} />
						{:else}
							<span class="avatar avatar-large"></span>
						{/if}
						<div class="account-menu-text">
							<span class="account-menu-name">{userDisplayName}</span>
							<span class="account-menu-email">{userEmail}</span>
							<span class="account-menu-role">{userRole}</span>
						</div>
					</div>
					<a href={resolve('/auth/logout')} class="account-menu-signout" role="menuitem">
						Sign out
					</a>
				</div>
			{/if}
		</div>
```

- [ ] **Step 5: Update the CSS**

Replace the existing `.avatar-link` / `.avatar` / `img.avatar` rules:

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

with:

```css
	.account {
		position: relative;
	}
	.avatar-button {
		background: none;
		border: none;
		padding: 0;
		cursor: pointer;
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
	.account-menu {
		position: absolute;
		top: calc(100% + var(--space-2));
		right: 0;
		min-width: 220px;
		background: var(--color-surface-2);
		border-radius: var(--radius-default);
		box-shadow: var(--shadow-flat);
		padding: var(--space-3);
		display: flex;
		flex-direction: column;
		gap: var(--space-3);
		z-index: 20;
	}
	.account-menu-identity {
		display: flex;
		align-items: center;
		gap: var(--space-3);
	}
	.avatar-large {
		width: 40px;
		height: 40px;
		flex-shrink: 0;
	}
	.account-menu-text {
		display: flex;
		flex-direction: column;
		gap: 2px;
		min-width: 0;
	}
	.account-menu-name {
		font-size: var(--text-table);
		color: var(--color-text);
		font-weight: 500;
	}
	.account-menu-email {
		font-size: var(--text-label);
		color: var(--color-muted);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.account-menu-role {
		font-size: var(--text-label);
		color: var(--color-muted-2);
		text-transform: capitalize;
	}
	.account-menu-signout {
		display: block;
		text-align: left;
		padding: var(--space-2) var(--space-1);
		border-radius: var(--radius-sm);
		color: var(--color-text-2);
		text-decoration: none;
		font-size: var(--text-table);
		border-top: 1px solid var(--color-line-2);
		padding-top: var(--space-3);
	}
	.account-menu-signout:hover {
		color: var(--color-text);
		background: var(--row-hover-bg);
	}
```

- [ ] **Step 6: Manually verify in a real browser**

Per Global Constraints, no component test infrastructure — verify by hand, using the same minted-session-cookie technique as prior plans this session (real OIDC login isn't available in this sandbox):

1. Start the dev server: `cd siem-web && pnpm dev`.
2. Mint a valid session cookie directly via `mintSessionToken` (see `siem-web/src/lib/server/session.ts`), including a real `email` value, matching the dev server's `SESSION_SECRET`, and set it as the `siem_session` cookie via Playwright.
3. Navigate to any page, confirm the avatar renders as a `<button>` (not a link that navigates on click).
4. Click the avatar: confirm a dropdown opens showing the picture (or placeholder), display name, email, and role, plus a "Sign out" link — and confirm the page did NOT navigate away (still on the same URL).
5. Click somewhere outside the dropdown: confirm it closes.
6. Reopen it, press Escape: confirm it closes and focus returns to the avatar button.
7. Reopen it, click "Sign out": confirm it navigates to `/auth/logout`.
8. Confirm nothing else in the Nav bar (main nav links, ingest status, alert pill) changed visually.

- [ ] **Step 7: Run the full siem-web test suite, lint, and type-check**

Run: `cd siem-web && pnpm exec vitest run && pnpm lint && pnpm exec svelte-check`
Expected: All PASS, no new type errors, lint clean.

- [ ] **Step 8: Commit**

```bash
git add siem-web/src/routes/+layout.svelte siem-web/src/lib/components/Nav.svelte
git commit -m "Replace instant-logout avatar link with an account dropdown menu"
```
