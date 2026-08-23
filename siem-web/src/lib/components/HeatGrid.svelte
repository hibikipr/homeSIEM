<script lang="ts">
	import { heatTierColor, heatTierGlyph } from '$lib/wall';

	let {
		rows,
		sourceLabels = {}
	}: { rows: { source: string; hours: string[] }[]; sourceLabels?: Record<string, string> } =
		$props();

	const LEGEND_TIERS = ['critical', 'warning', 'busy', 'light', 'quiet', 'none'] as const;

	let hovered = $state<{ source: string; tier: string; hoursAgo: number } | null>(null);

	function ageLabel(hoursAgo: number): string {
		return hoursAgo === 0 ? 'this hour' : `${hoursAgo}h ago`;
	}

	// row.source/hovered.source stay the raw name throughout (that's the
	// data identity - the each-block key, the tooltip's keyed-off value) -
	// only the rendered text prefers an operator-set display_name.
	function labelFor(source: string): string {
		return sourceLabels[source] ?? source;
	}
</script>

<div class="heat-grid">
	{#each rows as row (row.source)}
		<div class="row">
			<span class="label">{labelFor(row.source)}</span>
			<div class="cells">
				{#each row.hours as tier, i (i)}
					<span
						class="cell tier-{tier}"
						style="background: {heatTierColor(tier)}"
						role="img"
						aria-label="{labelFor(row.source)}: {tier}, {ageLabel(row.hours.length - 1 - i)}"
						onpointerenter={() =>
							(hovered = { source: row.source, tier, hoursAgo: row.hours.length - 1 - i })}
						onpointerleave={() => (hovered = null)}
					>
						<!-- aria-hidden: the glyph is a redundant visual-only cue for
						     sighted users who can't rely on color (see wall.ts's
						     heatTierGlyph doc) - the cell's own aria-label above
						     already says the tier in words for anyone using a screen
						     reader, so exposing this text too would just repeat it. -->
						<span class="glyph" aria-hidden="true">{heatTierGlyph(tier)}</span>
					</span>
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
				<span class="legend-swatch" style="background: {heatTierColor(tier)}"
					><span class="glyph" aria-hidden="true">{heatTierGlyph(tier)}</span></span
				>
				{tier}
			</span>
		{/each}
	</div>
	{#if hovered}
		<div class="hover-tooltip">
			<span class="tooltip-source">{labelFor(hovered.source)}</span>
			<span class="tooltip-tier"
				><span class="tier-name">{hovered.tier}</span> · {ageLabel(hovered.hoursAgo)}</span
			>
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
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
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
		display: flex;
		align-items: center;
		justify-content: center;
		overflow: hidden;
	}
	.glyph {
		font-size: 10px;
		line-height: 1;
		/* White with a dark outline in every direction, rather than a
		   per-tier color keyed to each token's exact lightness (unknown
		   without rendering it) - readable against light or dark cell
		   backgrounds either way. Same reasoning as a map pin or video
		   caption needing to stay legible over an arbitrary background. */
		color: #fff;
		text-shadow:
			-1px -1px 0 rgba(0, 0, 0, 0.65),
			1px -1px 0 rgba(0, 0, 0, 0.65),
			-1px 1px 0 rgba(0, 0, 0, 0.65),
			1px 1px 0 rgba(0, 0, 0, 0.65);
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
		box-shadow: inset 0 0 0 1px var(--color-line-2);
		display: flex;
		align-items: center;
		justify-content: center;
		overflow: hidden;
	}
	.legend-swatch .glyph {
		font-size: 7px;
	}

	/* Mobile: .cells is flex:1 (fluid), which on a phone squashes 24 hourly
	   cells down to ~11px slivers - too small to read the glyph or tap
	   accurately. Give cells a real minimum width instead and let the grid
	   scroll horizontally (the .axis start/now labels get the same
	   min-width as .cells so "now" still lines up with the last cell). */
	@media (max-width: 768px) {
		.heat-grid {
			overflow-x: auto;
		}
		.cells {
			min-width: 480px;
		}
		.axis {
			min-width: 480px;
		}
	}
	.hover-tooltip {
		/* Anchored to the TOP-right corner, not bottom-right: the legend is
		   always the last flex child of .heat-grid, so its block-level box
		   spans the full container width at the bottom regardless of how
		   little of that width its packed-left content actually uses -
		   a bottom-right tooltip's box geometrically overlaps it even
		   though the two never visually collide. Pinning to the top
		   structurally rules that out for any row count, while the right
		   offset (instead of the unset default that caused the original
		   bug) keeps it clear of the .row .label column on the left. */
		position: absolute;
		top: 0;
		right: 0;
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
	}
	.tier-name {
		text-transform: capitalize;
	}
</style>
