<script lang="ts">
	import { invalidateAll } from '$app/navigation';
	import { resolve } from '$app/paths';
	import AlertInbox from '$lib/components/AlertInbox.svelte';
	import AlertDetail from '$lib/components/AlertDetail.svelte';
	import RuleDetail from '$lib/components/RuleDetail.svelte';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

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
	/>
	{#if data.selectedAlert && data.stats}
		<AlertDetail
			alert={data.selectedAlert}
			samples={data.selectedSamples}
			stats={data.stats}
			rule={data.rules.find((r) => r.id === data.selectedAlert?.rule_id)}
		/>
	{:else if data.selectedRule}
		<RuleDetail rule={data.selectedRule} />
	{:else}
		<div class="empty">
			{data.tab === 'rules' ? 'Select a rule to see details.' : 'Select an alert to see details.'}
		</div>
	{/if}
</div>

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
