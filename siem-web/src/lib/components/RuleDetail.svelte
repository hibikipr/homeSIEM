<script lang="ts">
	import type { RuleResponse } from '$lib/server/siemApiClient';

	let { rule }: { rule: RuleResponse } = $props();
</script>

<div class="detail">
	<div class="header">
		<span class="eyebrow">{rule.shape}</span>
		<span class="enabled" class:off={!rule.enabled}>{rule.enabled ? 'enabled' : 'disabled'}</span>
	</div>
	<h1>{rule.name}</h1>
	<div class="meta">
		<span>severity: {rule.severity}</span>
		<span>window: {rule.window_sec}s</span>
		<span>cooldown: {rule.cooldown_sec}s</span>
	</div>
	<div class="destinations">destinations: {rule.destinations.join(', ')}</div>
	<div class="logql-block">{rule.logql}</div>
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
	}
	.eyebrow {
		font-size: var(--text-eyebrow);
		text-transform: uppercase;
		color: var(--color-muted-2);
	}
	.enabled {
		font-size: var(--text-eyebrow);
		text-transform: uppercase;
		color: var(--color-severity-healthy);
	}
	.enabled.off {
		color: var(--color-muted-2);
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
</style>
