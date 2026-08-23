<script lang="ts">
	import type { IngestHealthResponse } from '$lib/server/siemApiClient';

	let { health }: { health: IngestHealthResponse } = $props();

	let totalReceived = $derived(
		Object.values(health.received_events_per_source).reduce((sum, n) => sum + n, 0)
	);

	// What's left after accounting for the one deliberate, named filter in
	// the path (drop_blank_messages - see its doc on IngestHealthResponse).
	// Not necessarily zero even on a perfectly healthy deployment: Vector's
	// own metrics can lag a restart by a few events still in flight, and
	// this is a live snapshot comparison across component boundaries, not
	// an atomic one. Only surfaced when it's actually non-trivial, so a
	// few events of normal jitter don't read as a mystery to chase.
	let unexplainedGap = $derived(
		Math.max(
			0,
			totalReceived - health.loki_sent_events_total - health.blank_messages_filtered_total
		)
	);
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
					<dd class="mono">{total.toLocaleString()}</dd>
				</div>
			{/each}
			<div class="row total">
				<dt>Total received</dt>
				<dd class="mono">{totalReceived.toLocaleString()}</dd>
			</div>
			<div class="row">
				<dt>Loki sent</dt>
				<dd class="mono">{health.loki_sent_events_total.toLocaleString()}</dd>
			</div>
			<div class="row">
				<dt
					title="Events with an empty message get filtered before storage - correctly, since there's nothing to store. Counted at the source, before filtering, so they show up in 'received' but never 'Loki sent'."
				>
					Filtered — blank messages
				</dt>
				<dd class="mono">{health.blank_messages_filtered_total.toLocaleString()}</dd>
			</div>
			{#if unexplainedGap > 0}
				<div class="row">
					<dt
						title="Received minus Loki sent minus the blank-message filter above. A small amount here is normal jitter from comparing live metrics across component boundaries (e.g. a few events still in flight around a Vector restart) rather than an atomic snapshot - worth investigating only if this keeps growing over time."
					>
						Unexplained
					</dt>
					<dd class="mono">{unexplainedGap.toLocaleString()}</dd>
				</div>
			{/if}
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
	.row.total {
		border-top: 1px solid var(--color-line-2);
		margin-top: var(--space-1);
		padding-top: var(--space-2);
		font-weight: 500;
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
