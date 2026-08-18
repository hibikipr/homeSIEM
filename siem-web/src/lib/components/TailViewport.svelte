<script lang="ts">
	import { resolve } from '$app/paths';
	import { untrack } from 'svelte';
	import type { LogEntry } from '$lib/server/siemApiClient';
	import { filterBySeverity, severityColor } from '$lib/tail';
	import { extractField, extractMessage, formatTimestampInZone } from '$lib/logline';
	import { computeVisibleRange, isScrolledToBottom } from '$lib/search';

	const MAX_BUFFER = 5000;
	const ROW_HEIGHT = 28;

	let {
		activeSeverities = $bindable(),
		paused = $bindable(false),
		buffer = $bindable([]),
		// eslint-disable-next-line no-useless-assignment -- read by the parent via bind:connected, not locally
		connected = $bindable(true),
		displayTimezone,
		hiddenColumns
	}: {
		activeSeverities: Set<string>;
		paused: boolean;
		buffer: LogEntry[];
		connected: boolean;
		displayTimezone: string;
		hiddenColumns: ReadonlySet<string>;
	} = $props();

	let rendered = $state<LogEntry[]>([]);
	let autoFollow = $state(true);
	let newSinceDetach = $state(0);
	let viewportEl: HTMLDivElement;
	let scrollTop = $state(0);
	let containerHeight = $state(0);

	function queueScrollToBottom() {
		requestAnimationFrame(() => {
			if (viewportEl) viewportEl.scrollTop = viewportEl.scrollHeight;
		});
	}

	function measure() {
		if (viewportEl) containerHeight = viewportEl.clientHeight;
	}

	$effect(() => {
		measure();
	});

	function appendEntry(entry: LogEntry) {
		buffer = [...buffer, entry].slice(-MAX_BUFFER);
		if (paused) return;
		if (!activeSeverities.has(entry.Labels.severity ?? 'info')) return;

		rendered = [...rendered, entry].slice(-MAX_BUFFER);
		if (autoFollow) {
			queueScrollToBottom();
		} else {
			newSinceDetach += 1;
		}
	}

	// Re-derive the full rendered list from the buffer whenever the severity
	// filter changes or the view un-pauses — new-message-driven updates are
	// handled incrementally by appendEntry above. This effect only tracks
	// `activeSeverities` and `paused`; `buffer` is read via `untrack` so
	// reassigning it in appendEntry does not retrigger this effect (that
	// would double-process every message).
	$effect(() => {
		void activeSeverities.size;
		void paused;
		if (!paused) {
			rendered = filterBySeverity(
				untrack(() => buffer),
				activeSeverities
			);
			if (untrack(() => autoFollow)) queueScrollToBottom();
		}
	});

	$effect(() => {
		const source = new EventSource(resolve('/api/tail-proxy'));
		source.onopen = () => {
			connected = true;
		};
		source.onerror = () => {
			connected = false;
		};
		source.onmessage = (event) => {
			try {
				const entry: LogEntry = JSON.parse(event.data);
				appendEntry(entry);
			} catch {
				// malformed SSE payload — skip this line rather than breaking the stream
			}
		};
		return () => source.close();
	});

	function handleScroll() {
		if (!viewportEl) return;
		scrollTop = viewportEl.scrollTop;
		const atBottom = isScrolledToBottom(
			viewportEl.scrollTop,
			viewportEl.clientHeight,
			viewportEl.scrollHeight
		);
		if (atBottom) {
			autoFollow = true;
			newSinceDetach = 0;
		} else {
			autoFollow = false;
		}
	}

	function reattach() {
		autoFollow = true;
		newSinceDetach = 0;
		queueScrollToBottom();
	}

	// Virtualized rendering: at up to MAX_BUFFER (5000) rows, mounting one
	// DOM row per buffered entry made the tab increasingly sluggish the
	// longer a session ran (every incoming SSE message re-diffed a
	// potentially-thousands-of-rows-deep DOM tree) - only the rows actually
	// in (or just outside) the visible scroll window are ever mounted, same
	// approach ResultTable.svelte already uses for Search's result list.
	let range = $derived(
		computeVisibleRange(scrollTop, containerHeight, ROW_HEIGHT, rendered.length)
	);
	let visibleEntries = $derived(rendered.slice(range.startIndex, range.endIndex));
</script>

<svelte:window onresize={measure} />

<div class="viewport-wrap">
	<div class="header-row">
		{#if !hiddenColumns.has('time')}<span class="col-time">Time (UTC)</span>{/if}
		{#if !hiddenColumns.has('localTime')}<span class="col-time">Time ({displayTimezone})</span>{/if}
		{#if !hiddenColumns.has('severity')}<span class="col-severity"></span>{/if}
		{#if !hiddenColumns.has('host')}<span class="col-host">Host</span>{/if}
		{#if !hiddenColumns.has('program')}<span class="col-program">Program</span>{/if}
		{#if !hiddenColumns.has('facility')}<span class="col-facility">Facility</span>{/if}
		{#if !hiddenColumns.has('message')}<span class="col-message">Message</span>{/if}
	</div>
	<div class="viewport" bind:this={viewportEl} onscroll={handleScroll}>
		{#if rendered.length === 0}
			<div class="empty-row">
				{#if buffer.length === 0}
					Waiting for events…
				{:else if paused && filterBySeverity(buffer, activeSeverities).length > 0}
					Paused — resume to see buffered events.
				{:else}
					No events match the current filter.
				{/if}
			</div>
		{:else}
			<div class="spacer" style:height="{rendered.length * ROW_HEIGHT}px">
				{#each visibleEntries as entry, i (range.startIndex + i)}
					<div class="row" style:top="{(range.startIndex + i) * ROW_HEIGHT}px">
						{#if !hiddenColumns.has('time')}
							<span class="col-time mono">{entry.Timestamp}</span>
						{/if}
						{#if !hiddenColumns.has('localTime')}
							<span class="col-time mono"
								>{formatTimestampInZone(entry.Timestamp, displayTimezone)}</span
							>
						{/if}
						{#if !hiddenColumns.has('severity')}
							<span class="col-severity">
								<span class="dot" style:background={severityColor(entry.Labels.severity ?? 'info')}
								></span>
							</span>
						{/if}
						{#if !hiddenColumns.has('host')}
							<span class="col-host mono" title={entry.Labels.host ?? ''}
								>{entry.Labels.host ?? ''}</span
							>
						{/if}
						{#if !hiddenColumns.has('program')}
							<span class="col-program mono" title={entry.Labels.program ?? ''}
								>{entry.Labels.program ?? ''}</span
							>
						{/if}
						{#if !hiddenColumns.has('facility')}
							{@const facility = extractField(entry.Line, 'facility') ?? ''}
							<span class="col-facility mono" title={facility}>{facility}</span>
						{/if}
						{#if !hiddenColumns.has('message')}
							{@const message = extractMessage(entry.Line)}
							<span class="col-message" title={message}>{message}</span>
						{/if}
					</div>
				{/each}
			</div>
		{/if}
	</div>
	{#if !autoFollow}
		<button class="new-pill" onclick={reattach}>
			{newSinceDetach > 0 ? `${newSinceDetach} new` : 'Jump to now'}
		</button>
	{/if}
</div>

<style>
	.viewport-wrap {
		position: relative;
	}
	.header-row {
		display: flex;
		gap: var(--space-3);
		padding: var(--space-2) var(--space-3);
		background: var(--color-surface-3);
		font-size: var(--text-eyebrow);
		text-transform: uppercase;
		color: var(--color-muted-2);
	}
	.viewport {
		background: var(--color-bg-alt);
		box-shadow: inset var(--shadow-flat);
		border-radius: var(--radius-default);
		height: 60vh;
		overflow-y: auto;
		position: relative;
	}
	.spacer {
		position: relative;
	}
	.row {
		position: absolute;
		left: 0;
		right: 0;
		display: flex;
		align-items: center;
		gap: var(--space-3);
		height: 28px;
		padding: 0 var(--space-3);
		font-size: 12px;
	}
	.row:nth-child(odd) {
		background: rgba(255, 255, 255, 0.015);
	}
	.mono {
		font-family: var(--font-mono);
	}
	.col-time {
		width: 150px;
		flex-shrink: 0;
		color: var(--color-muted);
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}
	.col-severity {
		width: 16px;
		flex-shrink: 0;
	}
	.col-host {
		width: 96px;
		flex-shrink: 0;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}
	.col-program {
		width: 86px;
		flex-shrink: 0;
		color: var(--color-accent-light);
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}
	.col-facility {
		width: 64px;
		flex-shrink: 0;
		color: var(--color-muted-2);
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}
	.col-message {
		flex: 1;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		color: var(--color-text-2);
	}
	.dot {
		display: inline-block;
		width: 8px;
		height: 8px;
		border-radius: 50%;
	}
	.empty-row {
		display: flex;
		align-items: center;
		justify-content: center;
		height: 100%;
		text-align: center;
		color: var(--color-muted-2);
		padding: var(--space-6);
	}
	.new-pill {
		position: absolute;
		bottom: var(--space-4);
		left: 50%;
		transform: translateX(-50%);
		background: var(--color-accent);
		color: var(--color-bg);
		border: none;
		border-radius: 999px;
		padding: var(--space-1) var(--space-4);
		font-size: var(--text-label);
		font-weight: 500;
		cursor: pointer;
		box-shadow: var(--shadow-raised);
	}
</style>
