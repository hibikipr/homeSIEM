<script lang="ts">
	import { computeVolumeTiers } from '$lib/search';
	import type { VolumeBucket } from '$lib/server/siemApiClient';

	let { volume }: { volume: VolumeBucket[] } = $props();

	let tiers = $derived(computeVolumeTiers(volume));
	let maxCount = $derived(Math.max(1, ...volume.map((b) => b.count)));
</script>

<div class="ribbon">
	{#each volume as bucket, i (bucket.bucket_start)}
		<div
			class="bar tier-{tiers[i]}"
			style:height="{Math.max(2, (bucket.count / maxCount) * 100)}%"
			title="{bucket.count} events"
		></div>
	{/each}
</div>

<style>
	.ribbon {
		display: flex;
		align-items: flex-end;
		gap: 2px;
		height: 56px;
		background: var(--color-surface-2);
		border-radius: var(--radius-sm);
		padding: 0 var(--space-2);
		margin-top: var(--space-3);
	}
	.bar {
		flex: 1;
		min-width: 0;
		border-radius: 1px;
		background: var(--color-accent-tint-2);
	}
	.bar.tier-warning {
		background: var(--color-severity-warning);
	}
	.bar.tier-critical {
		background: var(--color-severity-critical);
	}
</style>
