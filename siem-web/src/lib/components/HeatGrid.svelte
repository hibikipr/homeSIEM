<script lang="ts">
	import { heatTierColor } from '$lib/wall';

	let { rows }: { rows: { source: string; hours: string[] }[] } = $props();

	const LEGEND_TIERS = ['critical', 'warning', 'busy', 'light', 'quiet', 'none'] as const;

	let hovered = $state<{ source: string; tier: string; hoursAgo: number } | null>(null);

	function ageLabel(hoursAgo: number): string {
		return hoursAgo === 0 ? 'this hour' : `${hoursAgo}h ago`;
	}
</script>

<div class="heat-grid">
	{#each rows as row (row.source)}
		<div class="row">
			<span class="label">{row.source}</span>
			<div class="cells">
				{#each row.hours as tier, i (i)}
					<span
						class="cell"
						style="background: {heatTierColor(tier)}"
						onpointerenter={() =>
							(hovered = { source: row.source, tier, hoursAgo: row.hours.length - 1 - i })}
						onpointerleave={() => (hovered = null)}
					></span>
				{/each}
			</div>
		</div>
	{/each}
	{#if rows.length > 0}
		<div class="axis">
			<span class="axis-start">{rows[0].hours.length - 1} hours ago</span>
			<span class="axis-end">now</span>
		</div>
	{/if}
	<div class="legend">
		{#each LEGEND_TIERS as tier (tier)}
			<span class="legend-item">
				<span class="legend-swatch" style="background: {heatTierColor(tier)}"></span>
				{tier}
			</span>
		{/each}
	</div>
	{#if hovered}
		<div class="hover-tooltip">
			<span class="tooltip-source">{hovered.source}</span>
			<span class="tooltip-tier">{hovered.tier} · {ageLabel(hovered.hoursAgo)}</span>
		</div>
	{/if}
</div>

<style>
	.heat-grid {
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
		position: relative;
	}
	.row {
		display: flex;
		align-items: center;
		gap: var(--space-2);
	}
	.label {
		width: 96px;
		font-family: var(--font-mono);
		font-size: var(--text-label);
		color: var(--color-muted);
		flex-shrink: 0;
	}
	.cells {
		display: flex;
		gap: 3px;
		flex: 1;
	}
	.cell {
		flex: 1;
		height: 19px;
		border-radius: 3px;
	}
	.axis {
		display: flex;
		justify-content: space-between;
		margin-top: var(--space-1);
		margin-left: calc(96px + var(--space-2));
		font-size: var(--text-label);
		color: var(--color-muted-2);
	}
	.legend {
		display: flex;
		gap: var(--space-4);
		margin-top: var(--space-3);
		flex-wrap: wrap;
	}
	.legend-item {
		display: flex;
		align-items: center;
		gap: var(--space-1);
		font-size: var(--text-label);
		color: var(--color-muted);
		text-transform: capitalize;
	}
	.legend-swatch {
		width: 10px;
		height: 10px;
		border-radius: 2px;
		flex-shrink: 0;
	}
	.hover-tooltip {
		position: absolute;
		background: var(--color-surface-3);
		box-shadow: var(--shadow-flat);
		border-radius: var(--radius-sm);
		padding: var(--space-2) var(--space-3);
		font-size: var(--text-label);
		display: flex;
		flex-direction: column;
		gap: 2px;
		pointer-events: none;
		z-index: 10;
	}
	.tooltip-source {
		color: var(--color-muted);
	}
	.tooltip-tier {
		color: var(--color-text);
		text-transform: capitalize;
	}
</style>
