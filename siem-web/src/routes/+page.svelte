<script lang="ts">
	import StatRow from '$lib/components/StatRow.svelte';
	import EventsOverTime from '$lib/components/EventsOverTime.svelte';
	import HeatGrid from '$lib/components/HeatGrid.svelte';
	import TriageCard from '$lib/components/TriageCard.svelte';
	import CountryBar from '$lib/components/CountryBar.svelte';
	import Ticker from '$lib/components/Ticker.svelte';
	import InsightsPanel from '$lib/components/InsightsPanel.svelte';
	import HostHealth from '$lib/components/HostHealth.svelte';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();
</script>

<div class="wall">
	<div class="col-main">
		<StatRow eventCount24h={data.eventCount24h} openAlertCount={data.openAlertCount} />
		<EventsOverTime totals={data.hourlyTotals} />
		{#await data.sourceLabels}
			<HeatGrid rows={data.heatGrid} sourceLabels={{}} />
		{:then sourceLabels}
			<HeatGrid rows={data.heatGrid} {sourceLabels} />
		{/await}
		{#if data.grafanaHostHealthUrl}
			<HostHealth url={data.grafanaHostHealthUrl} />
		{/if}
		<div class="triage-lane">
			{#each data.triageAlerts as alert (alert.id)}
				<TriageCard {alert} />
			{/each}
		</div>
	</div>
	<div class="col-side">
		{#await data.countryBreakdown}
			<CountryBar countries={[]} loading />
		{:then countryBreakdown}
			<CountryBar countries={countryBreakdown} />
		{/await}
		{#await data.insights}
			<InsightsPanel insights={[]} loading />
		{:then insights}
			<InsightsPanel {insights} />
		{/await}
		<Ticker />
	</div>
</div>

<style>
	.wall {
		display: grid;
		grid-template-columns: 1.62fr 1fr;
		gap: var(--space-6);
		padding: var(--space-5) var(--space-6);
	}
	.col-main,
	.col-side {
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: var(--space-5);
	}
	.triage-lane {
		display: grid;
		grid-template-columns: repeat(3, 1fr);
		gap: var(--space-4);
		align-items: start;
	}

	@media (max-width: 768px) {
		.wall {
			grid-template-columns: 1fr;
			padding: var(--space-5);
		}
		.triage-lane {
			grid-template-columns: 1fr;
		}
	}
</style>
