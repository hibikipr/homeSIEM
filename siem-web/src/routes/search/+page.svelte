<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { SvelteSet } from 'svelte/reactivity';
	import QueryBar from '$lib/components/QueryBar.svelte';
	import VolumeRibbon from '$lib/components/VolumeRibbon.svelte';
	import FacetRail from '$lib/components/FacetRail.svelte';
	import ResultTable from '$lib/components/ResultTable.svelte';
	import EventInspector from '$lib/components/EventInspector.svelte';
	import RuleFromEventForm from '$lib/components/RuleFromEventForm.svelte';
	import ColumnToggle from '$lib/components/ColumnToggle.svelte';
	import type { LogEntry } from '$lib/server/siemApiClient';
	import type { PageData } from './$types';
	import { extractSrcIp, SEARCH_COLUMNS } from '$lib/search';
	import { loadHiddenColumns, saveHiddenColumns } from '$lib/columnPrefs';

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

	function selectRow(index: number) {
		// eslint-disable-next-line svelte/prefer-svelte-reactivity -- imperative URL construction, not reactive state
		const params = new URLSearchParams(window.location.search);
		params.set('preview', String(index));
		goto(resolve(`/search?${params.toString()}`), { noScroll: true, keepFocus: true });
	}

	function filterToSrc(srcIp: string) {
		// eslint-disable-next-line svelte/prefer-svelte-reactivity -- imperative URL construction, not reactive state
		const params = new URLSearchParams(window.location.search);
		params.delete('preview');
		params.set('q', srcIp);
		goto(resolve(`/search?${params.toString()}`));
	}

	function facetClick(field: string, value: string) {
		// eslint-disable-next-line svelte/prefer-svelte-reactivity -- imperative URL construction, not reactive state
		const params = new URLSearchParams(window.location.search);
		params.delete('preview');
		params.set(field, value);
		goto(resolve(`/search?${params.toString()}`));
	}

	function alertOnThis() {
		ruleFormSeed = { name: 'search-alert', logql: data.logql };
	}

	function ruleFromEvent(entry: LogEntry) {
		const srcIp = extractSrcIp(entry.Line);
		const logql = srcIp ? `${data.logql} |= "${srcIp}"` : data.logql;
		ruleFormSeed = { name: 'event-rule', logql };
	}
</script>

<div class="search-screen">
	{#key data.logql}
		<QueryBar
			filters={data.filters}
			logql={data.logql}
			count={data.count}
			onAlertOnThis={alertOnThis}
		/>
	{/key}
	<VolumeRibbon volume={data.volume} />
	<div class="table-toolbar">
		<ColumnToggle columns={SEARCH_COLUMNS} hidden={hiddenColumns} onToggle={toggleColumn} />
	</div>
	<div class="body">
		<FacetRail
			entries={data.entries}
			claimedSourceNames={data.claimedSourceNames}
			onFacetClick={facetClick}
		/>
		<ResultTable
			entries={data.entries}
			selectedIndex={data.previewIndex}
			onSelect={selectRow}
			displayTimezone={data.displayTimezone}
			{hiddenColumns}
		/>
		<EventInspector
			entry={data.selectedEntry}
			contextSummary={data.contextSummary}
			onFilterToSrc={filterToSrc}
			onRuleFromThis={ruleFromEvent}
		/>
	</div>
</div>

{#if ruleFormSeed}
	<RuleFromEventForm
		defaultName={ruleFormSeed.name}
		defaultLogql={ruleFormSeed.logql}
		onClose={() => (ruleFormSeed = null)}
		onCreated={(id) => {
			ruleFormSeed = null;
			// Rules only live in Alerts -> Rules, not on this page - land the
			// user there with the new rule selected instead of leaving them
			// on Search with no indication of where it went.
			goto(resolve(`/alerts?state=rules&id=${id}`));
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
</style>
