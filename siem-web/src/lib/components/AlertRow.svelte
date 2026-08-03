<script lang="ts">
	import { resolve } from '$app/paths';
	import type { AlertResponse } from '$lib/server/siemApiClient';

	let { alert, ruleName, selected }: { alert: AlertResponse; ruleName: string; selected: boolean } =
		$props();

	function ageLabel(iso: string): string {
		const ms = Date.now() - new Date(iso).getTime();
		const minutes = Math.floor(ms / 60000);
		if (minutes < 60) return `${minutes}m`;
		return `${Math.floor(minutes / 60)}h`;
	}
</script>

<a
	class="row severity-{alert.severity}"
	class:selected
	href={resolve(`/alerts?state=${alert.state === 'acked' ? 'acked' : 'open'}&id=${alert.id}`)}
>
	<div class="header">
		<span class="eyebrow">{alert.severity}</span>
		<span class="age">{ageLabel(alert.first_seen_at)}</span>
	</div>
	<div class="title">{alert.title}</div>
	<div class="body">{alert.body}</div>
	<div class="rule">{ruleName}</div>
</a>

<style>
	.row {
		display: block;
		background: var(--color-surface-2);
		border-radius: var(--radius-default);
		padding: var(--space-4);
		border-left: 3px solid var(--color-severity-critical);
		text-decoration: none;
		color: inherit;
	}
	.row.severity-warning {
		border-left-color: var(--color-severity-warning);
	}
	.row.severity-low,
	.row.severity-medium {
		border-left-color: var(--color-severity-info);
	}
	.row.selected {
		background: var(--color-accent-tint);
		box-shadow: 0 0 0 1px var(--color-accent-deep);
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
		font-size: 13.5px;
		font-weight: 500;
		margin-top: var(--space-2);
	}
	.body {
		font-size: var(--text-table);
		color: var(--color-muted);
		margin-top: var(--space-1);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.rule {
		font-family: var(--font-mono);
		font-size: 10.5px;
		color: var(--color-muted-2);
		margin-top: var(--space-2);
	}
</style>
