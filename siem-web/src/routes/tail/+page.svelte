<script lang="ts">
	import { SvelteSet } from 'svelte/reactivity';
	import TailViewport from '$lib/components/TailViewport.svelte';
	import type { LogEntry } from '$lib/server/siemApiClient';
	import { SYSLOG_SEVERITIES, filterBySeverity, serializeNdjson } from '$lib/tail';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	// Bound via bind:activeSeverities to TailViewport's $bindable prop; svelte-check's
	// non_reactive_update check requires $state here even though SvelteSet is
	// independently reactive on its own.
	// eslint-disable-next-line svelte/no-unnecessary-state-wrap
	let activeSeverities = $state(new SvelteSet<string>(SYSLOG_SEVERITIES));
	let paused = $state(false);
	let buffer = $state<LogEntry[]>([]);
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
</script>

<div class="tail-screen">
	<header class="tail-header">
		<div class="left">
			<h1>Live tail</h1>
			<span class="status" class:disconnected={!connected}>
				<span class="dot"></span>
				{connected ? 'following' : 'disconnected · retrying'}
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
			<button class="action" disabled title="Search screen isn't built yet">Search this</button>
		</div>
	</header>

	<TailViewport bind:activeSeverities bind:paused bind:buffer bind:connected />

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
</style>
