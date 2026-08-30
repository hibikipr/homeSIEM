<script lang="ts">
	import type { LogEntry } from '$lib/server/siemApiClient';
	import { formatTimestamp } from '$lib/search';

	let { sourceName, samples }: { sourceName: string | null; samples: LogEntry[] } = $props();

	let selectedIndex = $state(0);

	// Switching which source is previewed reuses this same component
	// instance rather than remounting it - without this, picking a new
	// source while an older one's 6th sample was selected would silently
	// keep showing index 6 of the new source's list (or nothing, if it has
	// fewer than 7 samples) instead of resetting to its most recent one.
	$effect(() => {
		// The condition (always true - sourceName is string | null, never
		// undefined) is just what makes Svelte read sourceName here so it's
		// tracked as this effect's dependency; a bare `sourceName;`
		// expression statement does the same thing but trips
		// no-unused-expressions.
		if (sourceName !== undefined) selectedIndex = 0;
	});

	let selected = $derived(samples[selectedIndex] ?? null);

	const HISTORY_LINE_PREVIEW_CHARS = 80;

	// The full line is also what the "Raw line" card below shows for
	// whichever row is selected - truncating the actual text content (not
	// just visually, via CSS text-overflow) keeps this row a compact at-a-
	// glance preview instead of a duplicate of that card's full text.
	function truncateLine(line: string): string {
		return line.length > HISTORY_LINE_PREVIEW_CHARS
			? line.slice(0, HISTORY_LINE_PREVIEW_CHARS) + '…'
			: line;
	}

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
	{#if samples.length === 0}
		<p class="empty">No recent events from this source yet.</p>
	{:else}
		{#if samples.length > 1}
			<div class="history" role="listbox" aria-label="Recent samples">
				{#each samples as sample, i (sample.Timestamp + i)}
					<button
						type="button"
						role="option"
						aria-selected={i === selectedIndex}
						class="history-row"
						class:selected={i === selectedIndex}
						onclick={() => (selectedIndex = i)}
					>
						<span class="history-time mono">{formatTimestamp(sample.Timestamp)}</span>
						<span class="history-line mono">{truncateLine(sample.Line)}</span>
					</button>
				{/each}
			</div>
		{/if}
		<div class="cards">
			<div class="card">
				<div class="card-label">Raw line</div>
				<pre class="mono">{selected?.Line}</pre>
			</div>
			<div class="card">
				<div class="card-label">Extracted fields</div>
				<dl class="mono">
					{#each parsedFields(selected?.Line ?? '') as [key, value] (key)}
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
	.history {
		display: flex;
		flex-direction: column;
		max-height: 160px;
		overflow-y: auto;
		background: var(--color-surface-2);
		border-radius: var(--radius-default);
		margin-bottom: var(--space-3);
	}
	.history-row {
		display: flex;
		gap: var(--space-3);
		align-items: baseline;
		width: 100%;
		background: none;
		border: none;
		border-bottom: 1px solid var(--color-line-2);
		color: var(--color-text-2);
		text-align: left;
		padding: var(--space-2) var(--space-3);
		cursor: pointer;
	}
	.history-row:last-child {
		border-bottom: none;
	}
	.history-row:hover {
		background: var(--row-hover-bg);
	}
	.history-row.selected {
		background: var(--row-selected-bg);
	}
	.history-time {
		flex-shrink: 0;
		color: var(--color-muted);
		font-size: var(--text-label);
	}
	.history-line {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		font-size: var(--text-label);
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
