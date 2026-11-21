<script lang="ts">
	import StatRow from '#lib/components/StatRow.svelte';
	import EventsOverTime from '#lib/components/EventsOverTime.svelte';
	import HeatGrid from '#lib/components/HeatGrid.svelte';
	import TriageCard from '#lib/components/TriageCard.svelte';
	import CountryBar from '#lib/components/CountryBar.svelte';
	import Ticker from '#lib/components/Ticker.svelte';
	import InsightsPanel from '#lib/components/InsightsPanel.svelte';
	import HostHealth from '#lib/components/HostHealth.svelte';
	import Skeleton from '#lib/components/Skeleton.svelte';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();
</script>

<div class="wall">
	<div class="col-main">
		{#await data.stats}
			<StatRow eventCount24h={null} openAlertCount={data.openAlertCount} />
			<Skeleton height="120px" />
			<Skeleton height="200px" />
		{:then stats}
			{#if stats}
				<StatRow eventCount24h={stats.eventCount24h} openAlertCount={data.openAlertCount} />
				<EventsOverTime totals={stats.hourlyTotals} />
				{#await data.sourceLabels}
					<HeatGrid rows={stats.heatGrid} sourceLabels={{}} />
				{:then sourceLabels}
					<HeatGrid rows={stats.heatGrid} {sourceLabels} />
				{/await}
			{:else}
				<StatRow eventCount24h={null} openAlertCount={data.openAlertCount} />
				<p class="load-error">Couldn't load event stats.</p>
			{/if}
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
	.load-error {
		margin: 0;
		color: var(--color-muted);
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
