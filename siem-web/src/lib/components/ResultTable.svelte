<script lang="ts">
	import type { LogEntry } from '$lib/server/siemApiClient';
	import { computeVisibleRange, formatTimestamp, isScrolledToBottom } from '$lib/search';
	import { severityColor } from '$lib/tail';
	import { extractMessage, formatTimestampInZone } from '$lib/logline';

	const ROW_HEIGHT = 28;

	let {
		entries,
		selectedIndex,
		onSelect,
		displayTimezone,
		hiddenColumns
	}: {
		entries: LogEntry[];
		selectedIndex: number | null;
		onSelect: (index: number) => void;
		displayTimezone: string;
		hiddenColumns: ReadonlySet<string>;
	} = $props();

	let containerEl: HTMLDivElement;
	let scrollTop = $state(0);
	let containerHeight = $state(0);

	function handleScroll() {
		if (!containerEl) return;
		scrollTop = containerEl.scrollTop;
	}

	function measure() {
		if (containerEl) containerHeight = containerEl.clientHeight;
	}

	$effect(() => {
		measure();
	});

	// Keep the scroll position sane across two situations that can otherwise
	// leave the table looking broken:
	//   1. `entries` gets a new array reference (a new search ran) — reset
	//      scroll back to the top rather than leaving the user scrolled deep
	//      into a result set that no longer matches what's on screen.
	//   2. `selectedIndex` points at a row that isn't currently mounted in the
	//      visible window (e.g. deep-linking to `?preview=3000`) — scroll it
	//      into view, centered, but only when it's actually out of view so we
	//      don't fight the user's own scrolling on every render.
	let previousEntries: LogEntry[] | undefined;
	$effect(() => {
		const currentEntries = entries;
		const idx = selectedIndex;
		if (!containerEl) return;

		if (currentEntries !== previousEntries) {
			previousEntries = currentEntries;
			scrollTop = 0;
			containerEl.scrollTop = 0;
		}

		if (idx !== null) {
			const rowTop = idx * ROW_HEIGHT;
			const rowBottom = rowTop + ROW_HEIGHT;
			const visibleTop = containerEl.scrollTop;
			const visibleBottom = visibleTop + containerHeight;
			const isVisible = rowTop >= visibleTop && rowBottom <= visibleBottom;
			if (!isVisible) {
				const target = Math.max(0, idx * ROW_HEIGHT - containerHeight / 2);
				containerEl.scrollTop = target;
				scrollTop = target;
			}
		}
	});

	let range = $derived(computeVisibleRange(scrollTop, containerHeight, ROW_HEIGHT, entries.length));
	let visibleEntries = $derived(entries.slice(range.startIndex, range.endIndex));
	let atBottom = $derived(
		isScrolledToBottom(scrollTop, containerHeight, entries.length * ROW_HEIGHT)
	);

	// Results are sorted oldest-first (see siem-api's Loki query), so the most
	// recent matching event is always the last row — jump straight there
	// instead of scrolling through a potentially long result set.
	function jumpToLatest() {
		if (!containerEl) return;
		const target = entries.length * ROW_HEIGHT;
		containerEl.scrollTop = target;
		scrollTop = target;
	}
</script>

<svelte:window onresize={measure} />

<div class="table-wrap">
	<div class="header-row">
		{#if !hiddenColumns.has('time')}<span class="col-time">Time (UTC)</span>{/if}
		{#if !hiddenColumns.has('localTime')}<span class="col-time">Time ({displayTimezone})</span>{/if}
		{#if !hiddenColumns.has('severity')}<span class="col-severity"></span>{/if}
		{#if !hiddenColumns.has('host')}<span class="col-host">Host</span>{/if}
		{#if !hiddenColumns.has('program')}<span class="col-program">Program</span>{/if}
		{#if !hiddenColumns.has('message')}<span class="col-message">Message</span>{/if}
	</div>
	<div class="scroll-container" bind:this={containerEl} onscroll={handleScroll}>
		<div class="spacer" style:height="{entries.length * ROW_HEIGHT}px">
			{#each visibleEntries as entry, i (range.startIndex + i)}
				<button
					class="row"
					class:selected={range.startIndex + i === selectedIndex}
					class:stripe={(range.startIndex + i) % 2 === 1}
					style:top="{(range.startIndex + i) * ROW_HEIGHT}px"
					onclick={() => onSelect(range.startIndex + i)}
				>
					{#if !hiddenColumns.has('time')}
						<span class="col-time mono">{formatTimestamp(entry.Timestamp)}</span>
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
					{#if !hiddenColumns.has('message')}
						{@const message = extractMessage(entry.Line)}
						<span class="col-message" title={message}>{message}</span>
					{/if}
				</button>
			{/each}
		</div>
	</div>
	{#if !atBottom && entries.length > 0}
		<button class="jump-pill" onclick={jumpToLatest}>Jump to latest ↓</button>
	{/if}
</div>

<style>
	.table-wrap {
		position: relative;
		flex: 1 1 auto;
		min-width: 0;
		background: var(--color-surface-2);
		border-radius: var(--radius-default);
		box-shadow: inset var(--shadow-flat);
		overflow: hidden;
	}
	.jump-pill {
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
	.header-row {
		display: flex;
		gap: var(--space-3);
		padding: var(--space-2) var(--space-3);
		background: var(--color-surface-3);
		font-size: var(--text-eyebrow);
		text-transform: uppercase;
		color: var(--color-muted-2);
	}
	.scroll-container {
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
		background: none;
		border: none;
		width: 100%;
		text-align: left;
		cursor: pointer;
		font-size: 12px;
	}
	.row.stripe {
		background: rgba(255, 255, 255, 0.015);
	}
	.row:hover {
		background: var(--row-hover-bg);
	}
	.row.selected {
		background: var(--row-selected-bg);
	}
	.row.selected:hover {
		background: var(--row-selected-bg);
	}
	.mono {
		font-family: var(--font-mono);
	}
	.col-time {
		width: 190px;
		color: var(--color-muted);
		flex-shrink: 0;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}
	.col-severity {
		width: 14px;
		flex-shrink: 0;
	}
	.col-host {
		width: 88px;
		flex-shrink: 0;
		color: var(--color-text-3);
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}
	.col-program {
		width: 78px;
		flex-shrink: 0;
		color: var(--color-accent-light);
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
</style>
