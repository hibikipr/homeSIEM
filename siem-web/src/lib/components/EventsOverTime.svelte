<script lang="ts">
	import {
		computeChartPoints,
		formatHourLabel,
		CHART_WIDTH,
		CHART_HEIGHT
	} from '$lib/eventsOverTime';

	let { totals }: { totals: { hour_start: string; count: number }[] } = $props();

	let points = $derived(computeChartPoints(totals));
	let hasData = $derived(totals.some((t) => t.count > 0));
	let maxCount = $derived(Math.max(1, ...totals.map((t) => t.count)));

	let linePath = $derived(points.map((p, i) => `${i === 0 ? 'M' : 'L'} ${p.x} ${p.y}`).join(' '));
	let areaPath = $derived(
		points.length > 0
			? `M ${points[0].x} ${CHART_HEIGHT} ` +
					points.map((p) => `L ${p.x} ${p.y}`).join(' ') +
					` L ${points[points.length - 1].x} ${CHART_HEIGHT} Z`
			: ''
	);

	let axisLabelPoints = $derived(points.filter((_, i) => i % 4 === 0));

	function axisLabelTransform(index: number, total: number): string {
		if (index === 0) return 'translateX(0)';
		if (index === total - 1) return 'translateX(-100%)';
		return 'translateX(-50%)';
	}

	let svgEl: SVGSVGElement | undefined = $state();
	let hoveredIndex = $state<number | null>(null);

	function handlePointerMove(event: PointerEvent) {
		if (!svgEl || points.length === 0) return;
		const rect = svgEl.getBoundingClientRect();
		const scaleX = CHART_WIDTH / rect.width;
		const localX = (event.clientX - rect.left) * scaleX;
		const index = Math.max(
			0,
			Math.min(points.length - 1, Math.round((localX / CHART_WIDTH) * (points.length - 1)))
		);
		hoveredIndex = index;
	}

	function handlePointerLeave() {
		hoveredIndex = null;
	}

	let hoveredPoint = $derived(hoveredIndex !== null ? points[hoveredIndex] : null);
	let tooltipTransform = $derived.by(() => {
		if (hoveredIndex === null) return 'translateX(-50%)';
		if (hoveredIndex === 0) return 'translateX(0)';
		if (hoveredIndex === points.length - 1) return 'translateX(-100%)';
		return 'translateX(-50%)';
	});
</script>

<div class="events-over-time">
	<div class="eyebrow">Events, last 24h</div>
	{#if !hasData}
		<div class="empty">No events in the last 24h</div>
	{:else}
		<div class="chart-wrap">
			<svg
				bind:this={svgEl}
				viewBox="0 0 {CHART_WIDTH} {CHART_HEIGHT}"
				preserveAspectRatio="none"
				class="chart"
				role="img"
				aria-label="Events over the last 24 hours, {maxCount} maximum per hour"
				onpointermove={handlePointerMove}
				onpointerleave={handlePointerLeave}
			>
				<path d={areaPath} class="area" />
				<path d={linePath} class="line" />
				{#if hoveredPoint}
					<line
						x1={hoveredPoint.x}
						y1="0"
						x2={hoveredPoint.x}
						y2={CHART_HEIGHT}
						class="crosshair"
					/>
					<circle cx={hoveredPoint.x} cy={hoveredPoint.y} r="4" class="marker" />
				{/if}
			</svg>
			{#if hoveredPoint}
				<div
					class="tooltip"
					style:left="{(hoveredPoint.x / CHART_WIDTH) * 100}%"
					style:transform={tooltipTransform}
				>
					<span class="tooltip-hour">{formatHourLabel(hoveredPoint.hourStart)}</span>
					<span class="tooltip-count">{hoveredPoint.count} events</span>
				</div>
			{/if}
		</div>
		<div class="axis">
			{#each axisLabelPoints as point, i (point.hourStart)}
				<span
					class="axis-label"
					style:left="{(point.x / CHART_WIDTH) * 100}%"
					style:transform={axisLabelTransform(i, axisLabelPoints.length)}
				>
					{formatHourLabel(point.hourStart)}
				</span>
			{/each}
		</div>
		<div class="max-label">max {maxCount}/h</div>
	{/if}
</div>

<style>
	.events-over-time {
		background: var(--color-surface-2);
		border-radius: var(--radius-default);
		box-shadow: var(--shadow-flat);
		padding: var(--space-4);
		position: relative;
	}
	.eyebrow {
		font-size: var(--text-eyebrow);
		text-transform: uppercase;
		letter-spacing: 0.1em;
		color: var(--color-muted-2);
		margin-bottom: var(--space-3);
	}
	.empty {
		font-size: var(--text-table);
		color: var(--color-muted);
		padding: var(--space-5) 0;
		text-align: center;
	}
	.chart-wrap {
		position: relative;
	}
	.chart {
		width: 100%;
		height: 140px;
		display: block;
		cursor: crosshair;
	}
	.area {
		fill: var(--color-accent-tint-2);
		opacity: 0.5;
		stroke: none;
	}
	.line {
		fill: none;
		stroke: var(--color-accent-light);
		stroke-width: 2;
	}
	.crosshair {
		stroke: var(--color-line-2);
		stroke-width: 1;
	}
	.marker {
		fill: var(--color-accent-light);
		vector-effect: non-scaling-stroke;
	}
	.tooltip {
		position: absolute;
		top: var(--space-2);
		display: flex;
		flex-direction: column;
		gap: 2px;
		background: var(--color-surface-3);
		box-shadow: var(--shadow-flat);
		border-radius: var(--radius-sm);
		padding: var(--space-2) var(--space-3);
		pointer-events: none;
		white-space: nowrap;
	}
	.tooltip-hour {
		font-size: var(--text-label);
		color: var(--color-muted);
	}
	.tooltip-count {
		font-size: var(--text-table);
		color: var(--color-text);
	}
	.axis {
		position: relative;
		height: var(--space-5);
	}
	.axis-label {
		position: absolute;
		font-size: var(--text-label);
		color: var(--color-muted-2);
		white-space: nowrap;
	}
	.max-label {
		position: absolute;
		top: var(--space-4);
		right: var(--space-4);
		font-size: var(--text-label);
		color: var(--color-muted-2);
	}
</style>
