<script lang="ts">
	import type { AlertResponse } from '$lib/server/siemApiClient';

	let { alert }: { alert: AlertResponse } = $props();

	function ageLabel(iso: string): string {
		const ms = Date.now() - new Date(iso).getTime();
		const minutes = Math.floor(ms / 60000);
		if (minutes < 60) return `${minutes}m`;
		return `${Math.floor(minutes / 60)}h`;
	}
</script>

<div class="card severity-{alert.severity}">
	<div class="header">
		<span class="eyebrow">{alert.severity}</span>
		<span class="age">{ageLabel(alert.first_seen_at)}</span>
	</div>
	<div class="title">{alert.title}</div>
	<div class="body">{alert.body}</div>
	<div class="actions">
		<button class="primary">Investigate</button>
		<button class="ghost">Mute 1h</button>
	</div>
</div>

<style>
	.card {
		background: var(--color-surface-2);
		border-radius: var(--radius-default);
		padding: var(--space-4);
		box-shadow: inset 0 2px 0 var(--color-severity-critical);
	}
	.card.severity-warning {
		box-shadow: inset 0 2px 0 var(--color-severity-warning);
	}
	.card.severity-low,
	.card.severity-medium {
		box-shadow: inset 0 2px 0 var(--color-severity-info);
	}
	.header {
		display: flex;
		justify-content: space-between;
		font-size: var(--text-eyebrow);
		text-transform: uppercase;
		color: var(--color-severity-critical);
	}
	.age {
		color: var(--color-muted);
		text-transform: none;
	}
	.title {
		font-size: 14px;
		font-weight: 500;
		margin-top: var(--space-2);
	}
	.body {
		font-size: 11.5px;
		color: var(--color-muted);
		margin-top: var(--space-1);
	}
	.actions {
		margin-top: var(--space-3);
		display: flex;
		gap: var(--space-3);
	}
	.primary {
		background: transparent;
		border: 1px solid var(--color-accent);
		color: var(--color-text);
		border-radius: var(--radius-sm);
		padding: var(--space-1) var(--space-3);
		font-size: 11px;
	}
	.ghost {
		background: none;
		border: none;
		color: var(--color-accent-light);
		font-size: 11px;
	}
</style>
