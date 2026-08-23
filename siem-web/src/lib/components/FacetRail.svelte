<script lang="ts">
	import { deriveFacetCounts, deriveCountryFacet, mergeSourceFacet } from '$lib/search';
	import type { KnownSource, FacetCount } from '$lib/search';
	import { severityColor } from '$lib/tail';
	import type { LogEntry, ApiFacetCount } from '$lib/server/siemApiClient';

	let {
		entries,
		claimedSources,
		facets,
		onFacetClick
	}: {
		entries: LogEntry[];
		claimedSources: KnownSource[];
		// Real Loki-side aggregate counts from siem-api (see the Facets
		// field doc on searchResponse) - undefined only if the backend
		// omitted them (facets=false, or an older siem-api build), in
		// which case each facet below falls back to deriving from `entries`
		// alone, same as this component did before backend aggregates
		// existed. That fallback is capped at whatever page of entries the
		// caller fetched (1000 by default) and can badly undercount a rare
		// severity/program/source drowned out by a noisy one in the same
		// window - see deriveFacetCounts' own doc.
		facets?: Record<string, ApiFacetCount[]>;
		onFacetClick: (field: string, value: string) => void;
	} = $props();

	function facetOrFallback(label: string): FacetCount[] {
		return facets?.[label] ?? deriveFacetCounts(entries, label);
	}

	let sources = $derived(mergeSourceFacet(facetOrFallback('source'), claimedSources));
	let severities = $derived(facetOrFallback('severity'));
	let programs = $derived(facetOrFallback('program'));
	// Prefer the backend's real Loki-side aggregate (see queryCountryFacetCounts)
	// over deriveCountryFacet's page-of-entries derivation - geoip-bearing
	// events are a small fraction of any default search's results, so that
	// derivation came back empty in ordinary use even when countries were
	// genuinely present over the full time range.
	let countries = $derived(facets?.['country'] ?? deriveCountryFacet(entries));
</script>

<aside class="facets">
	<section>
		<h2>Source</h2>
		{#each sources as facet (facet.value)}
			<button
				class="facet-row"
				class:zero={facet.count === 0}
				title={facet.count === 0 ? 'Known source, not in the current results' : undefined}
				onclick={() => onFacetClick('source', facet.value)}
			>
				<span class="name">{facet.label ?? facet.value}</span>
				<span class="count mono">{facet.count}</span>
			</button>
		{/each}
	</section>
	<section>
		<h2>Severity</h2>
		{#each severities as facet (facet.value)}
			<button class="facet-row" onclick={() => onFacetClick('severity', facet.value)}>
				<span class="dot" style:background={severityColor(facet.value)}></span>
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
	.facet-row.zero {
		opacity: 0.5;
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
</style>
