<script lang="ts">
	import { resolve } from '$app/paths';
	import { untrack } from 'svelte';
	import type { LogEntry } from '$lib/server/siemApiClient';
	import { filterBySeverity, severityColor } from '$lib/tail';

	const MAX_BUFFER = 5000;

	let {
		activeSeverities = $bindable(),
		paused = $bindable(false),
		buffer = $bindable([]),
		// eslint-disable-next-line no-useless-assignment -- read by the parent via bind:connected, not locally
		connected = $bindable(true)
	}: {
		activeSeverities: Set<string>;
		paused: boolean;
		buffer: LogEntry[];
		connected: boolean;
	} = $props();

	let rendered = $state<LogEntry[]>([]);
	let autoFollow = $state(true);
	let newSinceDetach = $state(0);
	let viewportEl: HTMLDivElement;

	function queueScrollToBottom() {
		requestAnimationFrame(() => {
			if (viewportEl) viewportEl.scrollTop = viewportEl.scrollHeight;
		});
	}

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
		const atBottom = viewportEl.scrollHeight - viewportEl.scrollTop - viewportEl.clientHeight < 4;
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
</script>

<div class="viewport-wrap">
	<div class="viewport" bind:this={viewportEl} onscroll={handleScroll}>
		<table>
			<thead>
				<tr>
					<th class="col-time">Time</th>
					<th class="col-severity"></th>
					<th class="col-host">Host</th>
					<th class="col-program">Program</th>
					<th class="col-facility">Facility</th>
					<th>Message</th>
				</tr>
			</thead>
			<tbody>
				{#each rendered as entry, i (i)}
					<tr>
						<td class="col-time mono">{entry.Timestamp}</td>
						<td class="col-severity">
							<span class="dot" style:background={severityColor(entry.Labels.severity ?? 'info')}
							></span>
						</td>
						<td class="col-host mono">{entry.Labels.host ?? ''}</td>
						<td class="col-program mono">{entry.Labels.program ?? ''}</td>
						<td class="col-facility mono">{entry.Labels.facility ?? ''}</td>
						<td class="mono message">{entry.Line}</td>
					</tr>
				{:else}
					<tr class="empty-row">
						<td colspan="6">Waiting for events…</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
	{#if !autoFollow && newSinceDetach > 0}
		<button class="new-pill" onclick={reattach}>{newSinceDetach} new</button>
	{/if}
</div>

<style>
	.viewport-wrap {
		position: relative;
	}
	.viewport {
		background: var(--color-bg-alt);
		box-shadow: inset var(--shadow-flat);
		border-radius: var(--radius-default);
		height: 60vh;
		overflow-y: auto;
	}
	table {
		width: 100%;
		border-collapse: collapse;
		font-size: 12px;
		line-height: 2.05;
	}
	thead th {
		position: sticky;
		top: 0;
		background: var(--color-surface-3);
		text-align: left;
		font-size: var(--text-eyebrow);
		text-transform: uppercase;
		color: var(--color-muted-2);
		padding: var(--space-2) var(--space-3);
	}
	tbody td {
		padding: 0 var(--space-3);
		white-space: nowrap;
	}
	.mono {
		font-family: var(--font-mono);
	}
	.col-time {
		width: 150px;
		color: var(--color-muted);
	}
	.col-severity {
		width: 16px;
	}
	.col-host {
		width: 96px;
	}
	.col-program {
		width: 86px;
		color: var(--color-accent-light);
	}
	.col-facility {
		width: 64px;
		color: var(--color-muted-2);
	}
	.message {
		white-space: normal;
		word-break: break-word;
		color: var(--color-text-2);
	}
	.dot {
		display: inline-block;
		width: 8px;
		height: 8px;
		border-radius: 50%;
	}
	.empty-row td {
		text-align: center;
		white-space: normal;
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
