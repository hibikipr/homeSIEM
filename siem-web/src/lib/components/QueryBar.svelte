<script lang="ts">
	import { untrack } from 'svelte';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import ClearableField from '#lib/components/ClearableField.svelte';
	import type { SearchFilters } from '#lib/search.ts';

	let {
		filters,
		logql,
		count,
		shown,
		onAlertOnThis,
		loadFailed = false

		// The real total over the full time range (a Loki-side aggregate,
		// not derived from how many entries were fetched - see
		// searchResponse.Count's doc in siem-api).
		// How many of those `count` events are actually in the current
		// page of entries (capped at `limit`, 1000 by default). Shown
		// separately from `count` whenever they differ so a capped page
		// never gets presented as if it were the whole result - see the
		// Search facet-undercount bug this was added alongside.
	}: {
		filters: SearchFilters;
		// null while the (streamed) search is still loading or failed.
		logql: string | null;
		count: number | null;
		shown: number;
		onAlertOnThis: () => void;
		loadFailed?: boolean;
	} = $props();

	// Editable copies of the URL's filters, seeded once: the Search page
	// remounts this component ({#key data.logql}) whenever the applied
	// query changes, so a new search always starts from fresh values.
	const seed = untrack(() => filters);
	let source = $state(seed.source);
	let host = $state(seed.host);
	let program = $state(seed.program);
	let severity = $state(seed.severity);
	let facility = $state(seed.facility);
	let q = $state(seed.q);
	let includeInternal = $state(seed.includeInternal);

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
		if (includeInternal) params.set('internal', 'true');
		params.set('range', filters.range);
		goto(resolve(`search?${params.toString()}`));
	}

	function submit(event: SubmitEvent) {
		event.preventDefault();
		applyFilters();
	}

	function toggleIncludeInternal() {
		includeInternal = !includeInternal;
		applyFilters();
	}

	function setRange(range: SearchFilters['range']) {
		// eslint-disable-next-line svelte/prefer-svelte-reactivity -- imperative URL construction, not reactive state
		const params = new URLSearchParams(window.location.search);
		params.delete('preview');
		params.set('range', range);
		goto(resolve(`search?${params.toString()}`));
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

	<label
		class="internal-toggle"
		title="This stack's own operational chatter (Loki internals, docker-socket-proxy polling, etc.) is excluded by default so real signal doesn't get buried under it - check this to include it anyway."
	>
		<input type="checkbox" checked={includeInternal} onchange={toggleIncludeInternal} />
		Include platform's own logs
	</label>

	<button
		type="button"
		class="action"
		onclick={onAlertOnThis}
		disabled={logql === null}
		aria-label="New rule from this query"
	>
		New rule
	</button>
</form>

<div class="meta">
	<span class="mono count">
		{#if count === null}
			{loadFailed ? "couldn't load" : 'loading…'}
		{:else if shown < count}
			showing {shown.toLocaleString()} of {count.toLocaleString()} events
		{:else}
			{count.toLocaleString()} events
		{/if}
	</span>
	<span class="logql mono">{logql ?? ''}</span>
</div>

<style>
	.query-bar {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--space-2);
		background: var(--color-surface);
		border: 1px solid var(--color-line);
		border-radius: var(--radius-default);
		padding: var(--space-3);
	}
	.go {
		background: var(--saber-blue);
		border: 1px solid var(--saber-blue);
		color: var(--color-on-saber);
		font-family: var(--font-ui);
		font-weight: 600;
		letter-spacing: 0.02em;
		cursor: pointer;
		transition:
			background-color 0.2s,
			border-color 0.2s,
			color 0.2s;
		border-radius: var(--radius-default);
		padding: var(--space-1) var(--space-3);
		font-size: var(--text-label);
	}
	.go:hover:not(:disabled) {
		background: var(--color-btn-primary-hover);
		border-color: var(--color-btn-primary-hover);
		box-shadow: var(--glow-primary-hover);
	}

	.range {
		display: flex;
		gap: var(--space-1);
		margin-left: var(--space-2);
	}
	.range-btn {
		background: transparent;
		border: 1px solid var(--color-line-2);
		color: var(--color-muted);
		border-radius: var(--radius-pill);
		padding: var(--space-1) var(--space-3);
		font-family: var(--font-mono);
		font-size: var(--text-label);
		cursor: pointer;
		transition:
			color 0.2s,
			border-color 0.2s,
			background-color 0.2s;
	}
	.range-btn:hover {
		border-color: var(--saber-blue);
		color: var(--color-text);
	}
	.range-btn.active {
		background: var(--row-selected-bg);
		border-color: var(--saber-blue);
		color: var(--color-severity-info-text);
	}
	.action {
		background: transparent;
		border: 1px solid var(--color-line-2);
		color: var(--color-text-2);
		border-radius: var(--radius-default);
		font-family: var(--font-ui);
		cursor: pointer;
		transition:
			color 0.2s,
			border-color 0.2s,
			background-color 0.2s;
		margin-left: auto;
		padding: var(--space-1) var(--space-3);
		font-size: var(--text-label);
	}
	.action:hover:not(:disabled) {
		border-color: var(--saber-blue);
		color: var(--color-text);
	}

	.internal-toggle {
		display: flex;
		align-items: center;
		gap: var(--space-1);
		font-size: var(--text-label);
		color: var(--color-muted);
		cursor: pointer;
		white-space: nowrap;
	}
	.meta {
		display: flex;
		gap: var(--space-3);
		align-items: baseline;
		margin-top: var(--space-2);
		font-size: var(--text-label);
	}
	.count {
		font-family: var(--font-mono);
		color: var(--color-text);
	}
	.logql {
		font-family: var(--font-mono);
		color: var(--color-muted-2);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
</style>
