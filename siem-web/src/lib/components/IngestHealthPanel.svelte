<script lang="ts">
	import type { IngestHealthResponse } from '$lib/server/siemApiClient';

	let { health }: { health: IngestHealthResponse } = $props();
</script>

<section class="panel">
	<h2>Ingest health</h2>
	{#if health.degraded}
		<p class="degraded">Ingest metrics unavailable — siem-ingest's API is unreachable.</p>
	{:else}
		<dl>
			{#each Object.entries(health.received_events_per_source) as [source, total] (source)}
				<div class="row">
					<dt>{source} received</dt>
					<dd class="mono">{total}</dd>
				</div>
			{/each}
			<div class="row">
				<dt>Loki sent</dt>
				<dd class="mono">{health.loki_sent_events_total}</dd>
			</div>
		</dl>
	{/if}
</section>

<style>
	.panel {
		background: var(--color-surface-2);
		border-radius: var(--radius-default);
		padding: var(--space-4);
	}
	h2 {
		font-size: var(--text-section-head);
		color: var(--color-muted);
		margin: 0 0 var(--space-3);
	}
	.degraded {
		font-size: var(--text-table);
		color: var(--color-severity-warning);
		margin: 0;
	}
	dl {
		margin: 0;
	}
	.row {
		display: flex;
		justify-content: space-between;
		font-size: var(--text-table);
		padding: var(--space-1) 0;
	}
	.row dt {
		color: var(--color-muted);
		text-transform: capitalize;
	}
	.mono {
		font-family: var(--font-mono);
		color: var(--color-text-2);
	}
</style>
