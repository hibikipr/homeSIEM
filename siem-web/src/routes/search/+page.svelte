<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { SvelteSet } from 'svelte/reactivity';
	import QueryBar from '#lib/components/QueryBar.svelte';
	import VolumeRibbon from '#lib/components/VolumeRibbon.svelte';
	import FacetRail from '#lib/components/FacetRail.svelte';
	import ResultTable from '#lib/components/ResultTable.svelte';
	import EventInspector from '#lib/components/EventInspector.svelte';
	import RuleForm from '#lib/components/RuleForm.svelte';
	import ColumnToggle from '#lib/components/ColumnToggle.svelte';
	import Skeleton from '#lib/components/Skeleton.svelte';
	import type { SearchResults } from './+page.server';
	import type { LogEntry } from '#lib/server/siemApiClient.ts';
	import type { PageData } from './$types';
	import { extractSrcIp, SEARCH_COLUMNS } from '#lib/search.ts';
	import { loadHiddenColumns, saveHiddenColumns } from '#lib/columnPrefs.ts';

	let { data }: { data: PageData } = $props();

	let ruleFormSeed = $state<{ name: string; logql: string } | null>(null);

	const COLUMN_STORAGE_KEY = 'homesiem.search.hiddenColumns';
	const hiddenColumns = new SvelteSet(loadHiddenColumns(COLUMN_STORAGE_KEY));

	function toggleColumn(key: string) {
		if (hiddenColumns.has(key)) {
			hiddenColumns.delete(key);
		} else {
			hiddenColumns.add(key);
		}
		saveHiddenColumns(COLUMN_STORAGE_KEY, hiddenColumns);
	}

	// Below the mobile breakpoint the facets/table/inspector panes become a
	// drill-in (only one visible at a time) instead of a 3-way split -
	// selection is already URL-driven, so this is just which pane shows.
	let hasSelection = $derived(data.previewIndex !== null);

	// The slow Loki-backed result streams in; the frame (query bar, toolbar,
	// facet rail, table, inspector) renders immediately around it. The
	// server promise resolves to null on failure (never rejects).
	let results = $state<SearchResults | null>(null);
	let status = $state<'loading' | 'ready' | 'error'>('loading');
	$effect(() => {
		const pending = data.results;
		status = 'loading';
		results = null;
		pending.then((r) => {
			if (pending !== data.results) return;
			results = r;
			status = r ? 'ready' : 'error';
		});
	});
	let entries = $derived(results?.entries ?? []);
	let selectedEntry = $derived(
		results && data.previewIndex !== null ? (results.entries[data.previewIndex] ?? null) : null
	);

	function selectRow(index: number) {
		// eslint-disable-next-line svelte/prefer-svelte-reactivity -- imperative URL construction, not reactive state
		const params = new URLSearchParams(window.location.search);
		params.set('preview', String(index));
		goto(resolve(`search?${params.toString()}`), { reset: false });
	}

	function closeSelection() {
		// eslint-disable-next-line svelte/prefer-svelte-reactivity -- imperative URL construction, not reactive state
		const params = new URLSearchParams(window.location.search);
		params.delete('preview');
		goto(resolve(`search?${params.toString()}`), { reset: false });
	}

	function filterToSrc(srcIp: string) {
		// eslint-disable-next-line svelte/prefer-svelte-reactivity -- imperative URL construction, not reactive state
		const params = new URLSearchParams(window.location.search);
		params.delete('preview');
		params.set('q', srcIp);
		goto(resolve(`search?${params.toString()}`));
	}

	function facetClick(field: string, value: string) {
		// eslint-disable-next-line svelte/prefer-svelte-reactivity -- imperative URL construction, not reactive state
		const params = new URLSearchParams(window.location.search);
		params.delete('preview');
		params.set(field, value);
		goto(resolve(`search?${params.toString()}`));
	}

	function alertOnThis() {
		if (results) ruleFormSeed = { name: 'search-alert', logql: results.logql };
	}

	function ruleFromEvent(entry: LogEntry) {
		const srcIp = extractSrcIp(entry.Line);
		if (!results) return;
		const logql = srcIp ? `${results.logql} |= "${srcIp}"` : results.logql;
		ruleFormSeed = { name: 'event-rule', logql };
	}
</script>

<div class="search-screen">
	{#key JSON.stringify(data.filters)}
		<QueryBar
			filters={data.filters}
			logql={results?.logql ?? null}
			count={results?.count ?? null}
			shown={entries.length}
			loadFailed={status === 'error'}
			onAlertOnThis={alertOnThis}
		/>
	{/key}
	{#if status === 'ready' && results}
		<VolumeRibbon volume={results.volume} />
	{:else}
		<div class="ribbon-slot"><Skeleton height="56px" /></div>
	{/if}
	<div class="table-toolbar">
		<ColumnToggle columns={SEARCH_COLUMNS} hidden={hiddenColumns} onToggle={toggleColumn} />
	</div>
	<div class="body" class:has-selection={hasSelection}>
		<div class="pane pane-list">
			{#if status === 'error'}
				<p class="load-error">Couldn't load search results.</p>
			{:else if status === 'loading'}
				<div class="loading-pane" aria-busy="true">
					<Skeleton height="1.2rem" width="8rem" />
					<Skeleton height="1.2rem" width="6rem" />
					<Skeleton height="1.2rem" width="7rem" />
					<Skeleton height="260px" />
				</div>
			{:else}
				{#await data.claimedSources}
					<FacetRail
						{entries}
						claimedSources={[]}
						facets={results?.facets}
						onFacetClick={facetClick}
					/>
				{:then claimedSources}
					<FacetRail
						{entries}
						{claimedSources}
						facets={results?.facets}
						onFacetClick={facetClick}
					/>
				{/await}
				<ResultTable
					{entries}
					selectedIndex={data.previewIndex}
					onSelect={selectRow}
					displayTimezone={data.displayTimezone}
					{hiddenColumns}
				/>
			{/if}
		</div>
		<div class="pane pane-detail">
			<button type="button" class="back-link" onclick={closeSelection}>
				<i class="ph ph-arrow-left"></i> Back
			</button>
			{#await data.contextSummary}
				<EventInspector
					entry={selectedEntry}
					contextSummary={null}
					onFilterToSrc={filterToSrc}
					onRuleFromThis={ruleFromEvent}
				/>
			{:then contextSummary}
				<EventInspector
					entry={selectedEntry}
					{contextSummary}
					onFilterToSrc={filterToSrc}
					onRuleFromThis={ruleFromEvent}
				/>
			{/await}
		</div>
	</div>
</div>

{#if ruleFormSeed}
	<RuleForm
		mode="create"
		defaultName={ruleFormSeed.name}
		defaultLogql={ruleFormSeed.logql}
		onClose={() => (ruleFormSeed = null)}
		onSaved={(id) => {
			ruleFormSeed = null;
			// Rules only live in Alerts -> Rules, not on this page - land the
			// user there with the new rule selected instead of leaving them
			// on Search with no indication of where it went.
			goto(resolve(`alerts?state=rules&id=${id}`));
		}}
	/>
{/if}

<style>
	.search-screen {
		padding: var(--space-5) var(--space-6);
	}
	.table-toolbar {
		display: flex;
		justify-content: flex-end;
		margin-top: var(--space-4);
	}
	.body {
		display: flex;
		gap: var(--space-5);
		margin-top: var(--space-2);
		align-items: flex-start;
	}
	.pane-list,
	.pane-detail {
		display: contents;
	}
	.back-link {
		display: none;
	}
	.ribbon-slot {
		margin-top: var(--space-3);
	}
	.loading-pane {
		display: flex;
		flex-direction: column;
		gap: var(--space-3);
		flex: 1 1 auto;
		min-width: 0;
	}
	.load-error {
		margin: 0;
		color: var(--color-muted);
	}

	/* Mobile: drill-in instead of a 3-way split - facets+table show until a
	   row is selected (URL-driven, same as Alerts), then only the
	   inspector shows, with a back control to clear the selection. */
	@media (max-width: 768px) {
		.search-screen {
			padding: var(--space-5);
		}
		.body {
			flex-direction: column;
		}
		.pane-list {
			display: flex;
			flex-direction: column;
			gap: var(--space-5);
			width: 100%;
		}
		.pane-detail {
			display: block;
			width: 100%;
		}
		.body.has-selection .pane-list {
			display: none;
		}
		.body:not(.has-selection) .pane-detail {
			display: none;
		}
		.back-link {
			display: flex;
			align-items: center;
			gap: var(--space-2);
			margin-bottom: var(--space-4);
			background: none;
			border: none;
			padding: 0;
			color: var(--color-muted);
			font-size: var(--text-table);
			cursor: pointer;
		}
		.back-link:hover {
			color: var(--color-gold);
		}
	}
</style>
