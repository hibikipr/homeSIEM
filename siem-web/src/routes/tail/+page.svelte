<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { SvelteSet } from 'svelte/reactivity';
	import TailViewport from '$lib/components/TailViewport.svelte';
	import ColumnToggle from '$lib/components/ColumnToggle.svelte';
	import type { LogEntry } from '$lib/server/siemApiClient';
	import {
		SYSLOG_SEVERITIES,
		TAIL_COLUMNS,
		TAIL_DEFAULT_HIDDEN_COLUMNS,
		filterBySeverity,
		serializeNdjson
	} from '$lib/tail';
	import { loadHiddenColumns, saveHiddenColumns } from '$lib/columnPrefs';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const COLUMN_STORAGE_KEY = 'homesiem.tail.hiddenColumns';
	const hiddenColumns = new SvelteSet(
		loadHiddenColumns(COLUMN_STORAGE_KEY, TAIL_DEFAULT_HIDDEN_COLUMNS)
	);

	function toggleColumn(key: string) {
		if (hiddenColumns.has(key)) {
			hiddenColumns.delete(key);
		} else {
			hiddenColumns.add(key);
		}
		saveHiddenColumns(COLUMN_STORAGE_KEY, hiddenColumns);
	}

	// Bound via bind:activeSeverities to TailViewport's $bindable prop; svelte-check's
	// non_reactive_update check requires $state here even though SvelteSet is
	// independently reactive on its own.
	// eslint-disable-next-line svelte/no-unnecessary-state-wrap
	let activeSeverities = $state(new SvelteSet<string>(SYSLOG_SEVERITIES));
	let paused = $state(false);
	let buffer = $state.raw<LogEntry[]>([]);
	let connected = $state(true);

	function toggleSeverity(sev: string) {
		if (activeSeverities.has(sev)) {
			activeSeverities.delete(sev);
		} else {
			activeSeverities.add(sev);
		}
	}

	function exportBuffer() {
		const filtered = filterBySeverity(buffer, activeSeverities);
		const ndjson = serializeNdjson(filtered);
		const blob = new Blob([ndjson], { type: 'application/x-ndjson' });
		const url = URL.createObjectURL(blob);
		const a = document.createElement('a');
		a.href = url;
		a.download = `live-tail-${new Date().toISOString()}.ndjson`;
		a.click();
		URL.revokeObjectURL(url);
	}

	// Search's severity filter is single-valued (see SearchFilters in
	// $lib/search), but Live tail's chips are a multi-select set - only
	// carry a severity over when the user has actually narrowed down to
	// exactly one, otherwise there's no single value that represents
	// "these N severities" and an unfiltered Search is the honest result.
	function searchThis() {
		// eslint-disable-next-line svelte/prefer-svelte-reactivity -- imperative URL construction, not reactive state
		const params = new URLSearchParams();
		if (activeSeverities.size === 1) {
			params.set('severity', [...activeSeverities][0]);
		}
		const query = params.toString();
		goto(resolve(query ? `/search?${query}` : '/search'));
	}
</script>

<div class="tail-screen">
	<header class="tail-header">
		<div class="left">
			<h1>Live tail</h1>
			<span class="status" class:disconnected={!connected}>
				<span class="dot"></span>
				{connected ? 'live' : 'disconnected · retrying'}
			</span>
		</div>
		<div class="chips">
			{#each SYSLOG_SEVERITIES as sev (sev)}
				<button
					class="chip"
					class:active={activeSeverities.has(sev)}
					onclick={() => toggleSeverity(sev)}
				>
					{sev}
				</button>
			{/each}
		</div>
		<div class="actions">
			<button class="action" onclick={() => (paused = !paused)}>
				{paused ? 'Resume' : 'Pause'}
			</button>
			<button class="action" onclick={exportBuffer}>Export</button>
			<button
				class="action"
				onclick={searchThis}
				title={activeSeverities.size === 1
					? `Open Search filtered to severity=${[...activeSeverities][0]}`
					: 'Open Search (severity filter only carries over when exactly one is selected here)'}
			>
				Search this
			</button>
			<ColumnToggle columns={TAIL_COLUMNS} hidden={hiddenColumns} onToggle={toggleColumn} />
		</div>
	</header>

	<TailViewport
		bind:activeSeverities
		bind:paused
		bind:buffer
		bind:connected
		displayTimezone={data.displayTimezone}
		{hiddenColumns}
	/>

	<footer class="tail-footer">
		<span>Buffer 5,000 lines · Wrap off · Sources: all ({data.sourceCount})</span>
		<span class="mono">udp/514 · tcp/601 · tls/6514</span>
	</footer>
</div>

<style>
	.tail-screen {
		padding: var(--space-5) var(--space-6);
	}
	.tail-header {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--space-4);
		margin-bottom: var(--space-4);
	}
	.left {
		display: flex;
		align-items: center;
		gap: var(--space-3);
	}
	h1 {
		font-size: var(--text-page-title);
		margin: 0;
	}
	.status {
		display: flex;
		align-items: center;
		gap: var(--space-1);
		font-size: var(--text-label);
		color: var(--color-severity-healthy);
	}
	.status.disconnected {
		color: var(--color-severity-warning);
	}
	.status .dot {
		width: 6px;
		height: 6px;
		border-radius: 50%;
		background: currentColor;
	}
	.chips {
		display: flex;
		gap: var(--space-2);
		flex-wrap: wrap;
	}
	.chip {
		background: transparent;
		box-shadow: inset 0 0 0 1px var(--color-line-2);
		color: var(--color-muted);
		border: none;
		border-radius: var(--radius-sm);
		padding: var(--space-1) var(--space-3);
		font-size: var(--text-label);
		cursor: pointer;
	}
	.chip.active {
		background: var(--color-accent-tint);
		box-shadow: inset 0 0 0 1px var(--color-accent-deep);
		color: var(--color-accent-lighter);
	}
	.actions {
		display: flex;
		gap: var(--space-2);
		margin-left: auto;
	}
	.action {
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
	.tail-footer {
		display: flex;
		justify-content: space-between;
		font-size: var(--text-label);
		color: var(--color-muted-2);
		background: var(--color-surface-3);
		padding: var(--space-2) var(--space-3);
		margin-top: var(--space-2);
		border-radius: var(--radius-sm);
	}
	.mono {
		font-family: var(--font-mono);
	}

	@media (max-width: 768px) {
		.tail-screen {
			padding: var(--space-5);
		}
	}
</style>
