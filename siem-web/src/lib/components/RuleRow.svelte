<script lang="ts">
	import type { RuleResponse } from '$lib/server/siemApiClient';

	let { rule, selected }: { rule: RuleResponse; selected: boolean } = $props();
</script>

<a class="row" class:selected href="?state=rules&id={rule.id}">
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
		background: var(--color-surface-2);
		border-radius: var(--radius-default);
		padding: var(--space-4);
		text-decoration: none;
		color: inherit;
	}
	.row.selected {
		background: var(--color-accent-tint);
		box-shadow: 0 0 0 1px var(--color-accent-deep);
	}
	.header {
		display: flex;
		justify-content: space-between;
	}
	.name {
		font-size: 13.5px;
		font-weight: 500;
	}
	.enabled {
		font-size: var(--text-eyebrow);
		text-transform: uppercase;
		color: var(--color-severity-healthy);
	}
	.enabled.off {
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
