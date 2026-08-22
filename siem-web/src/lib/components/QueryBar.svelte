<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import ClearableField from '$lib/components/ClearableField.svelte';
	import type { SearchFilters } from '$lib/search';

	let {
		filters,
		logql,
		count,
		shown,
		onAlertOnThis
	}: {
		filters: SearchFilters;
		logql: string;
		// The real total over the full time range (a Loki-side aggregate,
		// not derived from how many entries were fetched - see
		// searchResponse.Count's doc in siem-api).
		count: number;
		// How many of those `count` events are actually in the current
		// page of entries (capped at `limit`, 1000 by default). Shown
		// separately from `count` whenever they differ so a capped page
		// never gets presented as if it were the whole result - see the
		// Search facet-undercount bug this was added alongside.
		shown: number;
		onAlertOnThis: () => void;
	} = $props();

	let source = $state(filters.source);
	let host = $state(filters.host);
	let program = $state(filters.program);
	let severity = $state(filters.severity);
	let facility = $state(filters.facility);
	let q = $state(filters.q);

	const RANGES: SearchFilters['range'][] = ['15m', '24h', '7d'];

	// Shared by the form's own submit (Search button / Enter) and each
	// ClearableField's onClear below - clicking a field's clear button
	// must re-run a real search with that filter now empty, not just
	// empty the box and leave the stale results on screen until the user
	// separately hits Enter.
	function applyFilters() {
		// eslint-disable-next-line svelte/prefer-svelte-reactivity -- imperative URL construction, not reactive state
		const params = new URLSearchParams();
		if (source) params.set('source', source);
		if (host) params.set('host', host);
		if (program) params.set('program', program);
		if (severity) params.set('severity', severity);
		if (facility) params.set('facility', facility);
		if (q) params.set('q', q);
		params.set('range', filters.range);
		goto(resolve(`/search?${params.toString()}`));
	}

	function submit(event: SubmitEvent) {
		event.preventDefault();
		applyFilters();
	}

	function setRange(range: SearchFilters['range']) {
		// eslint-disable-next-line svelte/prefer-svelte-reactivity -- imperative URL construction, not reactive state
		const params = new URLSearchParams(window.location.search);
		params.delete('preview');
		params.set('range', range);
		goto(resolve(`/search?${params.toString()}`));
	}
</script>

<form class="query-bar" onsubmit={submit}>
	<ClearableField placeholder="source" bind:value={source} onClear={applyFilters} />
	<ClearableField placeholder="host" bind:value={host} onClear={applyFilters} />
	<ClearableField placeholder="program" bind:value={program} onClear={applyFilters} />
	<ClearableField placeholder="severity" bind:value={severity} onClear={applyFilters} />
	<ClearableField placeholder="facility" bind:value={facility} onClear={applyFilters} />
	<ClearableField placeholder="free text" bind:value={q} onClear={applyFilters} wide />
	<button type="submit" class="go">Search</button>

	<div class="range">
		{#each RANGES as r (r)}
			<button
				type="button"
				class="range-btn"
				class:active={filters.range === r}
				onclick={() => setRange(r)}
			>
				{r}
			</button>
		{/each}
	</div>

	<button
		type="button"
		class="action"
		onclick={onAlertOnThis}
		aria-label="New rule from this query"
	>
		New rule
	</button>
</form>

<div class="meta">
	<span class="mono count">
		{#if shown < count}
			showing {shown.toLocaleString()} of {count.toLocaleString()} events
		{:else}
			{count.toLocaleString()} events
		{/if}
	</span>
	<span class="logql mono">{logql}</span>
</div>

<style>
	.query-bar {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--space-2);
		background: var(--color-surface);
		box-shadow: inset 0 0 0 1px var(--color-accent-tint-2);
		border-radius: var(--radius-default);
		padding: var(--space-3);
	}
	.go {
		background: var(--color-accent-tint-2);
		color: var(--color-accent-lighter);
		border: none;
		border-radius: var(--radius-sm);
		padding: var(--space-1) var(--space-3);
		font-size: var(--text-label);
		cursor: pointer;
	}
	.range {
		display: flex;
		gap: var(--space-1);
		margin-left: var(--space-2);
	}
	.range-btn {
		background: transparent;
		box-shadow: inset 0 0 0 1px var(--color-line-2);
		color: var(--color-muted);
		border: none;
		border-radius: var(--radius-sm);
		padding: var(--space-1) var(--space-2);
		font-size: var(--text-label);
		cursor: pointer;
	}
	.range-btn.active {
		background: var(--color-accent-tint);
		box-shadow: inset 0 0 0 1px var(--color-accent-deep);
		color: var(--color-accent-lighter);
	}
	.action {
		margin-left: auto;
		background: var(--color-surface-2);
		color: var(--color-text);
		border: none;
		border-radius: var(--radius-sm);
		padding: var(--space-1) var(--space-3);
		font-size: var(--text-label);
		cursor: pointer;
	}
	.meta {
		display: flex;
		gap: var(--space-3);
		align-items: baseline;
		margin-top: var(--space-2);
		font-size: var(--text-label);
	}
	.count {
		color: var(--color-text);
	}
	.logql {
		color: var(--color-muted-2);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
</style>
