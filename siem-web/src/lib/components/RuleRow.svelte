<script lang="ts">
	import { resolve } from '$app/paths';
	import type { RuleResponse } from '#lib/server/siemApiClient.ts';

	let { rule, selected }: { rule: RuleResponse; selected: boolean } = $props();
</script>

<a class="row" class:selected href={resolve(`alerts?state=rules&id=${rule.id}`)}>
	<div class="header">
		<span class="name">{rule.name}</span>
		<span class="enabled" class:off={!rule.enabled}>{rule.enabled ? 'enabled' : 'disabled'}</span>
	</div>
	<div class="shape">{rule.shape}</div>
	<div class="logql">{rule.logql}</div>
</a>

<style>
	.row {
		display: block;
		background: var(--color-surface);
		border: 1px solid var(--color-line);
		border-radius: var(--radius-default);
		padding: var(--space-4);
		text-decoration: none;
		color: inherit;
	}
	.row:hover {
		background: var(--row-hover-bg);
	}
	.row.selected {
		background: var(--row-selected-bg);
		box-shadow: 0 0 0 1px var(--color-accent-deep);
	}
	.header {
		display: flex;
		justify-content: space-between;
	}
	.name {
		font-size: 13.5px;
		font-weight: 600;
		letter-spacing: 0.02em;
	}
	.enabled {
		align-self: flex-start;
		font-size: var(--text-eyebrow);
		text-transform: uppercase;
		letter-spacing: 0.06em;
		background: var(--color-status-success-bg);
		color: var(--saber-green);
		border-radius: var(--radius-pill);
		padding: 1px var(--space-3);
	}
	.enabled.off {
		background: transparent;
		box-shadow: inset 0 0 0 1px var(--color-line-2);
		color: var(--color-muted-2);
	}
	.shape {
		font-size: var(--text-eyebrow);
		text-transform: uppercase;
		color: var(--color-muted-2);
		margin-top: var(--space-1);
	}
	.logql {
		font-family: var(--font-mono);
		font-size: var(--text-label);
		color: var(--color-muted);
		margin-top: var(--space-2);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
</style>
