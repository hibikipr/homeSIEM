<script lang="ts">
	import type { LogEntry } from '$lib/server/siemApiClient';

	let { sourceName, sample }: { sourceName: string | null; sample: LogEntry | null } = $props();

	function parsedFields(line: string): [string, string][] {
		try {
			const parsed = JSON.parse(line);
			if (typeof parsed !== 'object' || parsed === null) return [];
			return Object.entries(parsed).map(([k, v]) => [k, JSON.stringify(v)]);
		} catch {
			return [];
		}
	}
</script>

<section class="preview">
	<h2>Parser preview{sourceName ? ` — ${sourceName}` : ''}</h2>
	{#if !sample}
		<p class="empty">No recent events from this source yet.</p>
	{:else}
		<div class="cards">
			<div class="card">
				<div class="card-label">Raw line</div>
				<pre class="mono">{sample.Line}</pre>
			</div>
			<div class="card">
				<div class="card-label">Extracted fields</div>
				<dl class="mono">
					{#each parsedFields(sample.Line) as [key, value] (key)}
						<dt>{key}</dt>
						<dd>{value}</dd>
					{/each}
				</dl>
			</div>
		</div>
	{/if}
</section>

<style>
	.preview {
		margin-top: var(--space-5);
	}
	h2 {
		font-size: var(--text-section-head);
		color: var(--color-muted);
		margin: 0 0 var(--space-3);
	}
	.empty {
		color: var(--color-muted-2);
		font-size: var(--text-body);
	}
	.cards {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: var(--space-4);
	}
	.card {
		background: var(--color-surface-2);
		border-radius: var(--radius-default);
		padding: var(--space-4);
		overflow: auto;
	}
	.card-label {
		font-size: var(--text-eyebrow);
		text-transform: uppercase;
		color: var(--color-muted-2);
		margin-bottom: var(--space-2);
	}
	.mono {
		font-family: var(--font-mono);
		font-size: var(--text-log-row);
	}
	pre.mono {
		white-space: pre-wrap;
		word-break: break-word;
		margin: 0;
	}
	dl.mono {
		margin: 0;
		display: grid;
		grid-template-columns: auto 1fr;
		gap: var(--space-1) var(--space-3);
	}
	dt {
		color: var(--color-muted);
	}
	dd {
		margin: 0;
		color: var(--color-text-2);
		overflow-wrap: anywhere;
	}
</style>
