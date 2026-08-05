<script lang="ts">
	import { deriveFacetCounts, deriveCountryFacet } from '$lib/search';
	import type { LogEntry } from '$lib/server/siemApiClient';

	let {
		entries,
		onFacetClick
	}: {
		entries: LogEntry[];
		onFacetClick: (field: string, value: string) => void;
	} = $props();

	let severities = $derived(deriveFacetCounts(entries, 'severity'));
	let programs = $derived(deriveFacetCounts(entries, 'program'));
	let countries = $derived(deriveCountryFacet(entries));
</script>

<aside class="facets">
	<section>
		<h2>Severity</h2>
		{#each severities as facet (facet.value)}
			<button class="facet-row" onclick={() => onFacetClick('severity', facet.value)}>
				<span class="dot severity-{facet.value}"></span>
				<span class="name">{facet.value}</span>
				<span class="count mono">{facet.count}</span>
			</button>
		{/each}
	</section>
	<section>
		<h2>Program</h2>
		{#each programs as facet (facet.value)}
			<button class="facet-row" onclick={() => onFacetClick('program', facet.value)}>
				<span class="name">{facet.value}</span>
				<span class="count mono">{facet.count}</span>
			</button>
		{/each}
	</section>
	<section>
		<h2>Source country</h2>
		{#each countries as facet (facet.value)}
			<div class="facet-row display-only">
				<span class="name">{facet.value}</span>
				<span class="count mono">{facet.count}</span>
			</div>
		{/each}
	</section>
</aside>

<style>
	.facets {
		width: 184px;
		flex-shrink: 0;
		display: flex;
		flex-direction: column;
		gap: var(--space-4);
	}
	h2 {
		font-size: var(--text-eyebrow);
		text-transform: uppercase;
		color: var(--color-muted-2);
		margin: 0 0 var(--space-2);
	}
	.facet-row {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		width: 100%;
		background: none;
		border: none;
		color: var(--color-text-2);
		padding: var(--space-1) 0;
		font-size: var(--text-table);
		cursor: pointer;
		text-align: left;
	}
	.facet-row.display-only {
		cursor: default;
	}
	.facet-row:hover:not(.display-only) {
		color: var(--color-accent-light);
	}
	.name {
		flex: 1;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.count {
		color: var(--color-muted-2);
	}
	.dot {
		width: 7px;
		height: 7px;
		border-radius: 50%;
		flex-shrink: 0;
	}
	.dot.severity-critical {
		background: var(--color-severity-critical);
	}
	.dot.severity-warning {
		background: var(--color-severity-warning);
	}
	.dot.severity-info,
	.dot.severity-notice {
		background: var(--color-severity-info);
	}
</style>
