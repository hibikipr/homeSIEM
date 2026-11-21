<script lang="ts">
	import type { LogEntry } from '#lib/server/siemApiClient.ts';
	import { extractSrcIp } from '#lib/search.ts';
	import { severityColor } from '#lib/tail.ts';
	import { parseLogLine } from '#lib/logline.ts';

	let {
		entry,
		contextSummary,
		onFilterToSrc,
		onRuleFromThis
	}: {
		entry: LogEntry | null;
		contextSummary: { count: number } | null;
		onFilterToSrc: (srcIp: string) => void;
		onRuleFromThis: (entry: LogEntry) => void;
	} = $props();

	function parsedFields(line: string): [string, string][] {
		const parsed = parseLogLine(line);
		if (!parsed) return [];
		return Object.entries(parsed).map(([k, v]) => [k, JSON.stringify(v)]);
	}
</script>

<aside class="inspector">
	{#if !entry}
		<p class="empty">Select an event to see its detail.</p>
	{:else}
		{@const srcIp = extractSrcIp(entry.Line)}
		<div class="header">
			<span class="dot" style:background={severityColor(entry.Labels.severity ?? 'info')}></span>
			<span class="title">Event detail</span>
		</div>
		<pre class="raw mono">{entry.Line}</pre>
		<dl class="fields mono">
			{#each parsedFields(entry.Line) as [key, value] (key)}
				<dt>{key}</dt>
				<dd>{value}</dd>
			{/each}
		</dl>
		<div class="actions">
			{#if srcIp}
				<button onclick={() => onFilterToSrc(srcIp)}>Filter to SRC</button>
			{/if}
			<button onclick={() => onRuleFromThis(entry)} aria-label="New rule from this event">
				New rule
			</button>
		</div>
		{#if contextSummary}
			{@const displayCount =
				contextSummary.count >= 5000 ? '5,000+' : contextSummary.count.toLocaleString()}
			<div class="context">
				{displayCount} matching event{contextSummary.count === 1 ? '' : 's'} from this source IP in the
				last 24h.
			</div>
		{/if}
	{/if}
</aside>

<style>
	.inspector {
		width: 284px;
		flex-shrink: 0;
	}
	.empty {
		color: var(--color-muted-2);
		font-size: var(--text-body);
	}
	.header {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		margin-bottom: var(--space-3);
	}
	.title {
		font-size: var(--text-section-head);
		font-weight: 600;
		letter-spacing: 0.02em;
		color: var(--color-muted);
	}
	.dot {
		display: inline-block;
		width: 8px;
		height: 8px;
		border-radius: 50%;
	}
	.raw {
		font-family: var(--font-mono);
		background: var(--color-surface-2);
		border: 1px solid var(--color-line);
		border-radius: var(--radius-default);
		padding: var(--space-3);
		font-size: var(--text-log-row);
		white-space: pre-wrap;
		word-break: break-word;
		margin: 0 0 var(--space-3);
	}
	.fields {
		margin: 0 0 var(--space-3);
		display: grid;
		grid-template-columns: 96px 1fr;
		gap: var(--space-1) var(--space-3);
		font-size: var(--text-label);
	}
	dt {
		font-family: var(--font-mono);
		color: var(--color-muted);
	}
	dd {
		margin: 0;
		font-family: var(--font-mono);
		color: var(--color-severity-info-text);
		overflow-wrap: anywhere;
	}
	.actions {
		display: flex;
		gap: var(--space-2);
		margin-bottom: var(--space-3);
	}
	.actions button {
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
		padding: var(--space-1) var(--space-3);
		font-size: var(--text-label);
	}
	.actions button:hover {
		border-color: var(--saber-blue);
		color: var(--color-text);
	}
	.context {
		background: var(--row-selected-bg);
		border: 1px solid var(--color-line);
		border-radius: var(--radius-default);
		padding: var(--space-3);
		font-size: var(--text-table);
		color: var(--color-text-2);
	}
	.mono {
		font-family: var(--font-mono);
	}

	@media (max-width: 768px) {
		.inspector {
			width: 100%;
		}
	}
</style>
