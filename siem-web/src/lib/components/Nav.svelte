<script lang="ts">
	import { resolve } from '$app/paths';
	import type { Pathname } from '$app/types';

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
	let menuEl = $state<HTMLDivElement | undefined>();
	let menuButtonEl = $state<HTMLButtonElement | undefined>();

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

	function handleWindowKeydown(event: KeyboardEvent) {
		if (event.key === 'Escape' && menuOpen) {
			closeMenu();
		}
	}

	const navItems: { label: string; href: Pathname }[] = [
		{ label: 'Wall', href: '/' },
		{ label: 'Search', href: '/search' },
		{ label: 'Live tail', href: '/tail' },
		{ label: 'Alerts', href: '/alerts' },
		{ label: 'Sources', href: '/sources' },
		{ label: 'Settings', href: '/settings' }
	];

	const visibleNavItems = $derived(
		navItems.filter((item) => item.label !== 'Settings' || userRole === 'admin')
	);
</script>

<svelte:window onclick={handleWindowClick} onkeydown={handleWindowKeydown} />

<header class="nav">
	<div class="brand">
		<span class="brand-icon"><i class="ph ph-shield-check"></i></span>
		<span class="brand-name">homeSIEM</span>
	</div>

	<nav class="links">
		{#each visibleNavItems as item (item.href)}
			<a href={resolve(item.href)} class:active={activeRoute === resolve(item.href)}>
				{item.label}
				{#if item.label === 'Alerts' && alertCount > 0}
					<span class="pill">{alertCount}</span>
				{/if}
			</a>
		{/each}
	</nav>

	<div class="status">
		<span class="ingest-dot"></span>
		<span class="ingest-text">ingest live · {ingestRate}/min</span>
		<span class="user">
			{userDisplayName}
			<span class="role">{userRole}</span>
		</span>
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
				<!-- This is a static panel (identity info + one link), not a widget; the
					keydown handler below only catches Escape to dismiss it, so no
					interactive/menu role applies. -->
				<!-- svelte-ignore a11y_no_static_element_interactions -->
				<div bind:this={menuEl} class="account-menu" onkeydown={handleMenuKeydown}>
					<div class="account-menu-identity">
						{#if userPicture && !imgFailed}
							<img
								class="avatar avatar-large"
								src={userPicture}
								alt=""
								onerror={() => (imgFailed = true)}
							/>
						{:else}
							<span class="avatar avatar-large"></span>
						{/if}
						<div class="account-menu-text">
							<span class="account-menu-name">{userDisplayName}</span>
							<span class="account-menu-email">{userEmail}</span>
							<span class="account-menu-role">{userRole}</span>
						</div>
					</div>
					<a href={resolve('/auth/logout')} class="account-menu-signout"> Sign out </a>
				</div>
			{/if}
		</div>
	</div>
</header>

<style>
	.nav {
		display: flex;
		align-items: center;
		padding: var(--space-5) var(--space-6);
		gap: var(--space-6);
		background: linear-gradient(
				to right,
				transparent,
				var(--color-line) 48px,
				var(--color-line) calc(100% - 48px),
				transparent
			)
			bottom / 100% 1px no-repeat;
	}

	.brand {
		display: flex;
		align-items: center;
		gap: var(--space-3);
	}

	.brand-icon {
		width: 24px;
		height: 24px;
		border-radius: var(--radius-sm) calc(var(--radius-sm) + 3px);
		background: var(--color-accent-tint);
		color: var(--color-accent-light);
		display: flex;
		align-items: center;
		justify-content: center;
	}

	.brand-name {
		font-size: 20px;
		font-weight: 500;
		letter-spacing: -0.02em;
	}

	.links {
		display: flex;
		gap: var(--space-5);
		font-size: var(--text-table);
	}

	.links a {
		color: var(--color-muted);
		text-decoration: none;
		padding-bottom: var(--space-2);
		border-bottom: 2px solid transparent;
	}

	.links a.active {
		color: var(--color-text);
		border-bottom-color: var(--color-accent);
	}

	.pill {
		font-size: var(--text-eyebrow);
		background: var(--color-accent-light);
		color: var(--color-bg-alt);
		border-radius: var(--radius-sm);
		padding: 0 var(--space-2);
		margin-left: var(--space-1);
	}

	.status {
		margin-left: auto;
		display: flex;
		align-items: center;
		gap: var(--space-4);
		font-size: var(--text-table);
		color: var(--color-muted-2);
	}

	.ingest-dot {
		width: 6px;
		height: 6px;
		border-radius: 50%;
		background: var(--color-severity-healthy);
		display: inline-block;
	}

	.user .role {
		color: var(--color-muted-2);
		margin-left: var(--space-2);
	}

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
</style>
