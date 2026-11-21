<script lang="ts">
	import { resolve } from '$app/paths';
	import type { AlertResponse, RuleResponse } from '#lib/server/siemApiClient.ts';
	import { applyOverride, alertOverrides } from '#lib/optimisticAlerts.svelte.ts';
	import AlertRow from './AlertRow.svelte';
	import RuleRow from './RuleRow.svelte';

	let {
		tab,
		alerts,
		rules,
		selectedId,
		onNewRule,
		canCreateRule
	}: {
		tab: 'open' | 'acked' | 'muted' | 'rules';
		alerts: AlertResponse[];
		rules: RuleResponse[];
		selectedId: number | null;
		onNewRule: () => void;
		canCreateRule: boolean;
	} = $props();

	// Rows follow in-flight optimistic ack/mute: an alert whose state no longer
	// matches this tab drops out right away.
	const shownAlerts = $derived(
		alertOverrides.size === 0 ? alerts : alerts.map(applyOverride).filter((a) => a.state === tab)
	);
	const ruleNames = $derived(new Map(rules.map((r) => [r.id, r.name])));
	const tabs: { label: string; value: 'open' | 'acked' | 'muted' | 'rules' }[] = [
		{ label: 'Open', value: 'open' },
		{ label: 'Acked', value: 'acked' },
		{ label: 'Muted', value: 'muted' },
		{ label: 'Rules', value: 'rules' }
	];
</script>

<div class="inbox">
	<div class="header">
		<span class="title">Alerts</span>
		{#if tab === 'rules' && canCreateRule}
			<button class="new-rule" onclick={onNewRule}>+ New rule</button>
		{/if}
		<div class="tabs">
			{#each tabs as t (t.value)}
				<a href={resolve(`alerts?state=${t.value}`)} class:active={tab === t.value}>{t.label}</a>
			{/each}
		</div>
	</div>
	<div class="rows">
		{#if tab === 'rules'}
			{#each rules as rule (rule.id)}
				<RuleRow {rule} selected={selectedId === rule.id} />
			{:else}
				<div class="empty-list">No rules configured yet.</div>
			{/each}
		{:else}
			{#each shownAlerts as alert (alert.id)}
				<AlertRow
					{alert}
					ruleName={ruleNames.get(alert.rule_id) ?? `rule #${alert.rule_id}`}
					selected={selectedId === alert.id}
				/>
			{:else}
				<div class="empty-list">
					{#if tab === 'acked'}
						No acknowledged alerts.
					{:else if tab === 'muted'}
						No muted alerts.
					{:else}
						No open alerts.
					{/if}
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
		font-family: var(--font-display);
		font-size: var(--text-page-title);
		font-weight: 400;
		letter-spacing: 0.06em;
		color: var(--color-text);
	}
	.tabs {
		display: flex;
		gap: var(--space-3);
		font-size: var(--text-table);
	}
	.tabs a {
		color: var(--color-muted);
		text-decoration: none;
		padding: var(--space-1) var(--space-2);
		border-bottom: 2px solid transparent;
		transition:
			color 0.2s,
			border-color 0.2s;
	}
	.tabs a:hover {
		color: var(--color-gold);
	}
	.tabs a.active {
		color: var(--color-text);
		border-bottom-color: var(--saber-blue);
	}
	.new-rule {
		background: transparent;
		border: 1px solid var(--color-line-2);
		color: var(--color-text-2);
		border-radius: var(--radius-default);
		padding: var(--space-1) var(--space-3);
		font-family: var(--font-ui);
		font-size: var(--text-table);
		cursor: pointer;
		transition:
			color 0.2s,
			border-color 0.2s;
	}
	.new-rule:hover {
		border-color: var(--saber-blue);
		color: var(--color-text);
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

	/* On mobile the parent page (alerts/+page.svelte) drops the side-by-side
	   split for a list/detail drill-in, at which point this fixed 376px only
	   fits some phones and overflows narrower ones - let it fill the pane. */
	@media (max-width: 768px) {
		.inbox {
			width: 100%;
		}
	}
</style>
