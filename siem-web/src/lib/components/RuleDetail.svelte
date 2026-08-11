<script lang="ts">
	import type { RuleResponse } from '$lib/server/siemApiClient';

	let {
		rule,
		canEdit = false,
		onToggled,
		onEdit
	}: {
		rule: RuleResponse;
		canEdit?: boolean;
		// Called after a successful enable/disable so the parent can refresh
		// its own data (the rule list this detail pane was rendered from).
		onToggled?: () => void;
		// Opens the parent's edit form for this rule.
		onEdit?: () => void;
	} = $props();

	let toggling = $state(false);
	let error = $state<string | null>(null);

	async function toggleEnabled() {
		toggling = true;
		error = null;
		try {
			// PUT replaces the whole rule, not a partial patch - send every
			// existing field back unchanged except enabled, which flips.
			const response = await fetch(`/api/rules/${rule.id}`, {
				method: 'PUT',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({
					name: rule.name,
					shape: rule.shape,
					logql: rule.logql,
					window_sec: rule.window_sec,
					threshold: rule.threshold,
					group_by: rule.group_by,
					severity: rule.severity,
					destinations: rule.destinations,
					cooldown_sec: rule.cooldown_sec,
					interval_sec: rule.interval_sec,
					enabled: !rule.enabled
				})
			});
			if (!response.ok) {
				error =
					response.status === 403
						? "You don't have permission to edit rules."
						: 'Failed to update rule.';
				return;
			}
			onToggled?.();
		} catch {
			error = 'Network error — check your connection and try again.';
		} finally {
			toggling = false;
		}
	}
</script>

<div class="detail">
	<div class="header">
		<span class="eyebrow">{rule.shape}</span>
		<div class="status">
			<span class="enabled" class:off={!rule.enabled}>{rule.enabled ? 'enabled' : 'disabled'}</span>
			{#if canEdit}
				<button class="toggle" onclick={onEdit}>Edit</button>
				<button class="toggle" onclick={toggleEnabled} disabled={toggling}>
					{toggling ? '…' : rule.enabled ? 'Disable' : 'Enable'}
				</button>
			{/if}
		</div>
	</div>
	<h1>{rule.name}</h1>
	<div class="meta">
		<span>severity: {rule.severity}</span>
		<span>window: {rule.window_sec}s</span>
		<span>cooldown: {rule.cooldown_sec}s</span>
		<span>evaluates every: {rule.interval_sec}s</span>
	</div>
	<div class="destinations">destinations: {rule.destinations.join(', ')}</div>
	<div class="logql-block">{rule.logql}</div>
	{#if error}
		<p class="error">{error}</p>
	{/if}
</div>

<style>
	.detail {
		flex: 1;
		min-width: 0;
		background: var(--color-surface-2);
		border-radius: var(--radius-default);
		padding: var(--space-5);
	}
	.header {
		display: flex;
		justify-content: space-between;
		align-items: center;
	}
	.eyebrow {
		font-size: var(--text-eyebrow);
		text-transform: uppercase;
		color: var(--color-muted-2);
	}
	.status {
		display: flex;
		align-items: center;
		gap: var(--space-2);
	}
	.enabled {
		font-size: var(--text-eyebrow);
		text-transform: uppercase;
		color: var(--color-severity-healthy);
	}
	.enabled.off {
		color: var(--color-muted-2);
	}
	.toggle {
		background: var(--color-surface-3);
		color: var(--color-text);
		border: none;
		border-radius: var(--radius-sm);
		padding: var(--space-1) var(--space-2);
		font-size: var(--text-label);
		cursor: pointer;
	}
	.toggle:disabled {
		opacity: 0.6;
		cursor: default;
	}
	h1 {
		font-size: 26px;
		font-weight: 500;
		margin: var(--space-2) 0 0;
	}
	.meta {
		display: flex;
		gap: var(--space-4);
		font-size: var(--text-table);
		color: var(--color-muted);
		margin-top: var(--space-3);
	}
	.destinations {
		font-size: var(--text-table);
		color: var(--color-muted);
		margin-top: var(--space-2);
	}
	.logql-block {
		font-family: var(--font-mono);
		font-size: var(--text-label);
		color: var(--color-muted);
		background: var(--color-surface-3);
		border-radius: var(--radius-sm);
		padding: var(--space-2) var(--space-3);
		margin-top: var(--space-3);
		overflow-x: auto;
		white-space: nowrap;
	}
	.error {
		color: var(--color-severity-critical);
		font-size: var(--text-label);
		margin: var(--space-2) 0 0;
	}
</style>
