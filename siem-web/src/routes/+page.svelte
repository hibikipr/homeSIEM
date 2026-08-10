<script lang="ts">
	import StatRow from '$lib/components/StatRow.svelte';
	import EventsOverTime from '$lib/components/EventsOverTime.svelte';
	import HeatGrid from '$lib/components/HeatGrid.svelte';
	import TriageCard from '$lib/components/TriageCard.svelte';
	import CountryBar from '$lib/components/CountryBar.svelte';
	import Ticker from '$lib/components/Ticker.svelte';
	import InsightsPanel from '$lib/components/InsightsPanel.svelte';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();
</script>

<div class="wall">
	<div class="col-main">
		<StatRow eventCount24h={data.eventCount24h} openAlertCount={data.openAlertCount} />
		<EventsOverTime totals={data.hourlyTotals} />
		<HeatGrid rows={data.heatGrid} />
		<div class="triage-lane">
			{#each data.triageAlerts as alert (alert.id)}
				<TriageCard {alert} />
			{/each}
		</div>
	</div>
	<div class="col-side">
		<CountryBar countries={data.countryBreakdown} />
		<InsightsPanel insights={data.insights} />
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
</style>
