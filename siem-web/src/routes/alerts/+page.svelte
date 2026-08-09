<script lang="ts">
	import { goto, invalidateAll } from '$app/navigation';
	import { resolve } from '$app/paths';
	import AlertInbox from '$lib/components/AlertInbox.svelte';
	import AlertDetail from '$lib/components/AlertDetail.svelte';
	import RuleDetail from '$lib/components/RuleDetail.svelte';
	import RuleFromEventForm from '$lib/components/RuleFromEventForm.svelte';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();
	let showRuleForm = $state(false);
	// Same role gate siem-api's own POST/PUT /rules routes enforce
	// (RoleAnalyst) - reused for both creating and toggling rules.
	let canManageRules = $derived(data.userRole === 'admin' || data.userRole === 'analyst');

	$effect(() => {
		const source = new EventSource(resolve('/api/alerts-proxy'));
		source.onmessage = () => {
			invalidateAll();
		};
		return () => source.close();
	});
</script>

<div class="alerts">
	<AlertInbox
		tab={data.tab}
		alerts={data.alerts}
		rules={data.rules}
		selectedId={data.selectedAlert?.id ?? data.selectedRule?.id ?? null}
		onNewRule={() => (showRuleForm = true)}
		canCreateRule={canManageRules}
	/>
	{#if data.selectedAlert && data.stats}
		<AlertDetail
			alert={data.selectedAlert}
			samples={data.selectedSamples}
			stats={data.stats}
			rule={data.rules.find((r) => r.id === data.selectedAlert?.rule_id)}
		/>
	{:else if data.selectedRule}
		<RuleDetail
			rule={data.selectedRule}
			canEdit={canManageRules}
			onToggled={() => invalidateAll()}
		/>
	{:else}
		<div class="empty">
			{data.tab === 'rules' ? 'Select a rule to see details.' : 'Select an alert to see details.'}
		</div>
	{/if}
</div>

{#if showRuleForm}
	<RuleFromEventForm
		defaultName=""
		defaultLogql=""
		onClose={() => (showRuleForm = false)}
		onCreated={(id) => {
			showRuleForm = false;
			// Switches to the Rules tab and selects the new rule, even if the
			// form was opened from the Open/Acked tab.
			goto(resolve(`/alerts?state=rules&id=${id}`));
		}}
	/>
{/if}

<style>
	.alerts {
		display: flex;
		gap: var(--space-6);
		padding: var(--space-5) var(--space-6);
	}
	.empty {
		flex: 1;
		display: flex;
		align-items: center;
		justify-content: center;
		color: var(--color-muted-2);
	}
</style>
