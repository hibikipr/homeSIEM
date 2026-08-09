<script lang="ts">
	import { resolve } from '$app/paths';
	import type { AlertResponse, RuleResponse } from '$lib/server/siemApiClient';
	import AlertRow from './AlertRow.svelte';
	import RuleRow from './RuleRow.svelte';

	let {
		tab,
		alerts,
		rules,
		selectedId,
		onNewRule
	}: {
		tab: 'open' | 'acked' | 'rules';
		alerts: AlertResponse[];
		rules: RuleResponse[];
		selectedId: number | null;
		onNewRule: () => void;
	} = $props();

	const ruleNames = $derived(new Map(rules.map((r) => [r.id, r.name])));
	const tabs: { label: string; value: 'open' | 'acked' | 'rules' }[] = [
		{ label: 'Open', value: 'open' },
		{ label: 'Acked', value: 'acked' },
		{ label: 'Rules', value: 'rules' }
	];
</script>

<div class="inbox">
	<div class="header">
		<span class="title">Alerts</span>
		<div class="tabs">
			{#each tabs as t (t.value)}
				<a href={resolve(`/alerts?state=${t.value}`)} class:active={tab === t.value}>{t.label}</a>
			{/each}
		</div>
		{#if tab === 'rules'}
			<button class="new-rule" onclick={onNewRule}>+ New rule</button>
		{/if}
	</div>
	<div class="rows">
		{#if tab === 'rules'}
			{#each rules as rule (rule.id)}
				<RuleRow {rule} selected={selectedId === rule.id} />
			{:else}
				<div class="empty-list">No rules configured yet.</div>
			{/each}
		{:else}
			{#each alerts as alert (alert.id)}
				<AlertRow
					{alert}
					ruleName={ruleNames.get(alert.rule_id) ?? `rule #${alert.rule_id}`}
					selected={selectedId === alert.id}
				/>
			{:else}
				<div class="empty-list">
					{tab === 'acked' ? 'No acknowledged alerts.' : 'No open alerts.'}
				</div>
			{/each}
		{/if}
	</div>
</div>

<style>
	.inbox {
		width: 376px;
		flex-shrink: 0;
		display: flex;
		flex-direction: column;
		gap: var(--space-4);
	}
	.header {
		display: flex;
		align-items: center;
		justify-content: space-between;
	}
	.title {
		font-size: var(--text-page-title);
		font-weight: 500;
	}
	.tabs {
		display: flex;
		gap: var(--space-3);
		font-size: var(--text-table);
	}
	.tabs a {
		color: var(--color-muted);
		text-decoration: none;
		padding: var(--space-1) var(--space-3);
		border-radius: var(--radius-sm);
	}
	.tabs a.active {
		background: var(--color-accent-tint);
		color: var(--color-accent-lighter);
	}
	.new-rule {
		background: none;
		border: 1px solid var(--color-line-2);
		color: var(--color-accent-light);
		border-radius: var(--radius-sm);
		padding: var(--space-1) var(--space-3);
		font-size: var(--text-table);
		cursor: pointer;
	}
	.rows {
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
	}
	.empty-list {
		color: var(--color-muted-2);
		font-size: var(--text-table);
		padding: var(--space-4);
		text-align: center;
	}
</style>
