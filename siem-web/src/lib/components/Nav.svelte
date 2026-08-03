<script lang="ts">
	import { resolve } from '$app/paths';
	import type { Pathname } from '$app/types';

	let {
		activeRoute,
		alertCount,
		ingestRate,
		userDisplayName,
		userRole
	}: {
		activeRoute: string;
		alertCount: number;
		ingestRate: number;
		userDisplayName: string;
		userRole: string;
	} = $props();

	// Only `/` (Wall) exists in this sub-project; the other five screens are separate
	// future sub-projects, so their paths aren't in SvelteKit's generated `Pathname`
	// union yet. They're asserted to `Pathname` so `resolve()` (which applies `base`)
	// can still be used uniformly — drop each assertion as its route lands.
	const navItems: { label: string; href: Pathname }[] = [
		{ label: 'Wall', href: '/' },
		{ label: 'Search', href: '/search' as Pathname },
		{ label: 'Live tail', href: '/tail' as Pathname },
		{ label: 'Alerts', href: '/alerts' },
		{ label: 'Sources', href: '/sources' as Pathname },
		{ label: 'Settings', href: '/settings' as Pathname }
	];
</script>

<header class="nav">
	<div class="brand">
		<span class="brand-icon"><i class="ph ph-shield-check"></i></span>
		<span class="brand-name">homeSIEM</span>
	</div>

	<nav class="links">
		{#each navItems as item (item.href)}
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
		<a href={resolve('/auth/logout')} class="avatar" aria-label="Log out"></a>
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

	.avatar {
		width: 28px;
		height: 28px;
		border-radius: 50%;
		background: var(--color-line-2);
		display: inline-block;
	}
</style>
