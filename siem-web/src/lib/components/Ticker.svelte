<script lang="ts">
	import { resolve } from '$app/paths';

	interface TickerEntry {
		time: string;
		severity: string;
		host: string;
		program: string;
		message: string;
	}

	let entries = $state<TickerEntry[]>([]);

	$effect(() => {
		const source = new EventSource(resolve('/api/tail-proxy'));
		source.onmessage = (event) => {
			try {
				const raw = JSON.parse(event.data);
				entries = [
					{
						time: raw.Timestamp ?? '',
						severity: raw.Labels?.severity ?? 'info',
						host: raw.Labels?.host ?? '',
						program: raw.Labels?.program ?? '',
						message: raw.Line ?? ''
					},
					...entries
				].slice(0, 50);
			} catch {
				// malformed SSE payload — skip this line rather than breaking the ticker
			}
		};
		return () => source.close();
	});
</script>

<div class="ticker">
	<div class="eyebrow">Ticker</div>
	{#each entries as entry, i (i)}
		<div class="row">
			<span class="time">{entry.time}</span>
			<span class="dot severity-{entry.severity}"></span>
			<span class="line">{entry.host} {entry.program}: {entry.message}</span>
		</div>
	{/each}
</div>

<style>
	.ticker {
		background: var(--color-bg-alt);
		box-shadow: inset var(--shadow-flat);
		border-radius: var(--radius-default);
		font-family: var(--font-mono);
		font-size: 11px;
		line-height: 1.9;
		padding: var(--space-3);
		max-height: 400px;
		overflow: hidden;
	}
	.eyebrow {
		font-size: var(--text-eyebrow);
		text-transform: uppercase;
		color: var(--color-muted-2);
		margin-bottom: var(--space-2);
	}
	.row {
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
		display: flex;
		gap: var(--space-2);
		align-items: center;
	}
	.time {
		color: var(--color-muted);
	}
	.dot {
		width: 6px;
		height: 6px;
		border-radius: 50%;
		background: var(--color-severity-info);
		flex-shrink: 0;
	}
	.dot.severity-critical {
		background: var(--color-severity-critical);
	}
	.dot.severity-warning {
		background: var(--color-severity-warning);
	}
</style>
