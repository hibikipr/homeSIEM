<script lang="ts">
	import type { CountryCount } from '$lib/wall';

	let { countries }: { countries: CountryCount[] } = $props();
	let max = $derived(Math.max(1, ...countries.map((c) => c.count)));
</script>

<div class="country-bar">
	<div class="eyebrow">Where it's coming from</div>
	{#if countries.length === 0}
		<div class="empty">No international traffic in this sample.</div>
	{:else}
		{#each countries as c (c.country)}
			<div class="row">
				<span class="name">{c.country}</span>
				<div class="track">
					<div class="fill" style="width: {(c.count / max) * 100}%"></div>
				</div>
				<span class="count">{c.count}</span>
			</div>
		{/each}
	{/if}
</div>

<style>
	.country-bar {
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
	}
	.eyebrow {
		font-size: var(--text-eyebrow);
		text-transform: uppercase;
		color: var(--color-muted-2);
		margin-bottom: var(--space-2);
	}
	.row {
		display: flex;
		align-items: center;
		gap: var(--space-3);
	}
	.name {
		width: 112px;
		font-size: var(--text-table);
	}
	.track {
		flex: 1;
		height: 6px;
		background: var(--color-surface);
		border-radius: var(--radius-sm);
		overflow: hidden;
	}
	.fill {
		height: 100%;
		background: var(--color-accent);
	}
	.count {
		font-family: var(--font-mono);
		font-size: var(--text-label);
		color: var(--color-muted);
	}
	.empty {
		font-size: var(--text-label);
		color: var(--color-muted-2);
	}
</style>
