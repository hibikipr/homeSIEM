<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import type { SearchFilters } from '$lib/search';

	let {
		filters,
		logql,
		count,
		onAlertOnThis
	}: {
		filters: SearchFilters;
		logql: string;
		count: number;
		onAlertOnThis: () => void;
	} = $props();

	let source = $state(filters.source);
	let host = $state(filters.host);
	let program = $state(filters.program);
	let severity = $state(filters.severity);
	let facility = $state(filters.facility);
	let q = $state(filters.q);

	const RANGES: SearchFilters['range'][] = ['15m', '24h', '7d'];

	function submit(event: SubmitEvent) {
		event.preventDefault();
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

	function setRange(range: SearchFilters['range']) {
		// eslint-disable-next-line svelte/prefer-svelte-reactivity -- imperative URL construction, not reactive state
		const params = new URLSearchParams(window.location.search);
		params.set('range', range);
		goto(resolve(`/search?${params.toString()}`));
	}
</script>

<form class="query-bar" onsubmit={submit}>
	<input class="field" placeholder="source" bind:value={source} />
	<input class="field" placeholder="host" bind:value={host} />
	<input class="field" placeholder="program" bind:value={program} />
	<input class="field" placeholder="severity" bind:value={severity} />
	<input class="field" placeholder="facility" bind:value={facility} />
	<input class="field wide" placeholder="free text" bind:value={q} />
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

	<button type="button" class="action" disabled title="Saved searches aren't built yet">
		Save
	</button>
	<button type="button" class="action" onclick={onAlertOnThis}>Alert on this</button>
</form>

<div class="meta">
	<span class="mono count">{count.toLocaleString()} events</span>
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
	.field {
		background: var(--color-surface-2);
		border: none;
		border-radius: var(--radius-sm);
		color: var(--color-text);
		padding: var(--space-1) var(--space-2);
		font-size: var(--text-table);
		width: 110px;
	}
	.field.wide {
		flex: 1 1 200px;
		width: auto;
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
	.action:disabled {
		opacity: 0.5;
		cursor: not-allowed;
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
