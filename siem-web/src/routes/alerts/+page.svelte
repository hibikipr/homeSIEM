<script lang="ts">
	import { goto, invalidateAll } from '$app/navigation';
	import { resolve } from '$app/paths';
	import AlertInbox from '$lib/components/AlertInbox.svelte';
	import AlertDetail from '$lib/components/AlertDetail.svelte';
	import RuleDetail from '$lib/components/RuleDetail.svelte';
	import RuleForm from '$lib/components/RuleForm.svelte';
	import type { RuleResponse } from '$lib/server/siemApiClient';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();
	let showRuleForm = $state(false);
	let editingRule = $state<RuleResponse | null>(null);
	// Same RoleAnalyst gate siem-api's own POST/PUT /rules and
	// POST /alerts/{id}/ack|mute routes all enforce - reused for rule
	// create/edit/delete/toggle AND the Alerts detail pane's
	// Acknowledge/Mute buttons, not just Rules despite the name.
	let canManageRules = $derived(data.userRole === 'admin' || data.userRole === 'analyst');
	// Below the mobile breakpoint the list/detail panes become a drill-in
	// (only one visible at a time) instead of a side-by-side split -
	// selection is already URL-driven, so this is just which pane shows.
	let hasSelection = $derived(Boolean(data.selectedAlert || data.selectedRule));

	$effect(() => {
		const source = new EventSource(resolve('/api/alerts-proxy'));
		source.onmessage = () => {
			invalidateAll();
		};
		return () => source.close();
	});
</script>

<div class="alerts" class:has-selection={hasSelection}>
	<div class="pane pane-list">
		<AlertInbox
			tab={data.tab}
			alerts={data.alerts}
			rules={data.rules}
			selectedId={data.selectedAlert?.id ?? data.selectedRule?.id ?? null}
			onNewRule={() => (showRuleForm = true)}
			canCreateRule={canManageRules}
		/>
	</div>
	<div class="pane pane-detail">
		<a href={resolve(`/alerts?state=${data.tab}`)} class="back-link">
			<i class="ph ph-arrow-left"></i> Back
		</a>
		{#if data.selectedAlert && data.stats}
			{#await Promise.all([data.sourceDisplayNames, data.liveSourcesByName])}
				<AlertDetail
					alert={data.selectedAlert}
					samples={data.selectedSamples}
					stats={data.stats}
					rule={data.rules.find((r) => r.id === data.selectedAlert?.rule_id)}
					sourceDisplayNames={{}}
					liveSources={{}}
					canEdit={canManageRules}
				/>
			{:then [sourceDisplayNames, liveSources]}
				<AlertDetail
					alert={data.selectedAlert}
					samples={data.selectedSamples}
					stats={data.stats}
					rule={data.rules.find((r) => r.id === data.selectedAlert?.rule_id)}
					{sourceDisplayNames}
					{liveSources}
					canEdit={canManageRules}
				/>
			{/await}
		{:else if data.selectedRule}
			<RuleDetail
				rule={data.selectedRule}
				canEdit={canManageRules}
				onToggled={() => invalidateAll()}
				onEdit={() => (editingRule = data.selectedRule ?? null)}
				onDeleted={() => goto(resolve(`/alerts?state=rules`))}
			/>
		{:else}
			<div class="empty">
				{data.tab === 'rules' ? 'Select a rule to see details.' : 'Select an alert to see details.'}
			</div>
		{/if}
	</div>
</div>

{#if showRuleForm}
	<RuleForm
		mode="create"
		defaultName=""
		defaultLogql=""
		onClose={() => (showRuleForm = false)}
		onSaved={(id) => {
			showRuleForm = false;
			// Switches to the Rules tab and selects the new rule, even if the
			// form was opened from the Open/Acked tab.
			goto(resolve(`/alerts?state=rules&id=${id}`));
		}}
	/>
{:else if editingRule}
	<RuleForm
		mode="edit"
		initial={editingRule}
		onClose={() => (editingRule = null)}
		onSaved={() => {
			editingRule = null;
			invalidateAll();
		}}
	/>
{/if}

<style>
	.alerts {
		display: flex;
		gap: var(--space-6);
		padding: var(--space-5) var(--space-6);
	}
	.pane-list {
		display: contents;
	}
	.pane-detail {
		display: contents;
	}
	.back-link {
		display: none;
	}
	.empty {
		flex: 1;
		display: flex;
		align-items: center;
		justify-content: center;
		color: var(--color-muted-2);
	}

	/* Mobile: drill-in instead of side-by-side - only the list or the
	   detail pane is visible at a time, based on whether a URL selection
	   is present, with a back link to clear it. */
	@media (max-width: 768px) {
		.alerts {
			flex-direction: column;
			padding: var(--space-5);
		}
		.pane-list,
		.pane-detail {
			display: block;
			width: 100%;
		}
		.alerts.has-selection .pane-list {
			display: none;
		}
		.alerts:not(.has-selection) .pane-detail {
			display: none;
		}
		.back-link {
			display: flex;
			align-items: center;
			gap: var(--space-2);
			margin-bottom: var(--space-4);
			color: var(--color-muted);
			text-decoration: none;
			font-size: var(--text-table);
		}
	}
</style>
