<script lang="ts">
	import { resolve } from '$app/paths';
	import type { AlertResponse } from '#lib/server/siemApiClient.ts';

	let { alert, ruleName, selected }: { alert: AlertResponse; ruleName: string; selected: boolean } =
		$props();

	function ageLabel(iso: string): string {
		const ms = Date.now() - new Date(iso).getTime();
		const minutes = Math.floor(ms / 60000);
		if (minutes < 60) return `${minutes}m`;
		return `${Math.floor(minutes / 60)}h`;
	}

	function mutedLabel(iso: string): string {
		const ms = new Date(iso).getTime() - Date.now();
		const minutes = Math.max(0, Math.ceil(ms / 60000));
		if (minutes < 60) return `muted ${minutes}m`;
		return `muted ${Math.floor(minutes / 60)}h`;
	}
</script>

<a
	class="row severity-{alert.severity}"
	class:selected
	href={resolve(`alerts?state=${alert.state}&id=${alert.id}`)}
>
	<div class="header">
		<span class="eyebrow">{alert.severity}</span>
		{#if alert.state === 'muted' && alert.muted_until}
			<span class="age muted">{mutedLabel(alert.muted_until)}</span>
		{:else}
			<span class="age">{ageLabel(alert.first_seen_at)}</span>
		{/if}
	</div>
	<div class="title">{alert.title}</div>
	<div class="body">{alert.body}</div>
	<div class="rule">{ruleName}</div>
</a>

<style>
	.row {
		display: block;
		background: var(--color-surface);
		border: 1px solid var(--color-line);
		border-radius: var(--radius-default);
		padding: var(--space-4);
		border-left: 3px solid var(--color-severity-info);
		text-decoration: none;
		color: inherit;
		transition:
			background-color 0.2s,
			border-color 0.2s;
	}
	.row:hover {
		background: var(--row-hover-bg);
	}
	.row.severity-warning {
		border-left-color: var(--color-severity-warning);
	}
	.row.severity-critical {
		border-left-color: var(--color-severity-critical);
	}
	.row.selected {
		background: var(--row-selected-bg);
		box-shadow: 0 0 0 1px var(--color-accent-deep);
	}
	.header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		font-size: var(--text-eyebrow);
		text-transform: uppercase;
	}
	.eyebrow {
		background: var(--row-selected-bg);
		color: var(--color-severity-info-text);
		border-radius: var(--radius-pill);
		padding: 1px var(--space-3);
		letter-spacing: 0.06em;
	}
	.row.severity-warning .eyebrow {
		background: var(--color-status-warn-bg);
		color: var(--color-gold);
	}
	.row.severity-critical .eyebrow {
		background: var(--color-status-danger-soft);
		color: var(--saber-red);
	}
	.age {
		font-family: var(--font-mono);
		color: var(--color-muted);
		text-transform: none;
	}
	.age.muted {
		color: var(--color-severity-info-text);
		font-weight: 600;
	}
	.title {
		font-size: 13.5px;
		font-weight: 600;
		letter-spacing: 0.02em;
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
