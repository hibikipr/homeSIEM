<script lang="ts">
	import type { LogEntry } from '$lib/server/siemApiClient';
	import { extractSrcIp } from '$lib/search';
	import { severityColor } from '$lib/tail';

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
		try {
			const parsed = JSON.parse(line);
			if (typeof parsed !== 'object' || parsed === null) return [];
			return Object.entries(parsed).map(([k, v]) => [k, JSON.stringify(v)]);
		} catch {
			return [];
		}
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
			<button onclick={() => onRuleFromThis(entry)}>New rule</button>
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
		color: var(--color-muted);
	}
	.dot {
		display: inline-block;
		width: 8px;
		height: 8px;
		border-radius: 50%;
	}
	.raw {
		background: var(--color-surface-2);
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
		color: var(--color-muted);
	}
	dd {
		margin: 0;
		color: var(--color-accent-lighter);
		overflow-wrap: anywhere;
	}
	.actions {
		display: flex;
		gap: var(--space-2);
		margin-bottom: var(--space-3);
	}
	.actions button {
		background: var(--color-surface-2);
		color: var(--color-text);
		border: none;
		border-radius: var(--radius-sm);
		padding: var(--space-1) var(--space-3);
		font-size: var(--text-label);
		cursor: pointer;
	}
	.context {
		background: var(--color-accent-tint);
		border-radius: var(--radius-default);
		padding: var(--space-3);
		font-size: var(--text-table);
		color: var(--color-text-2);
	}
	.mono {
		font-family: var(--font-mono);
	}
</style>
