<script lang="ts">
	let { eventCount24h, openAlertCount }: { eventCount24h: number; openAlertCount: number } =
		$props();

	function formatCount(n: number): { value: string; unit: string } {
		if (n >= 1_000_000) return { value: (n / 1_000_000).toFixed(2), unit: 'M' };
		if (n >= 1_000) return { value: (n / 1_000).toFixed(1), unit: 'K' };
		return { value: String(n), unit: '' };
	}

	let events = $derived(formatCount(eventCount24h));
</script>

<div class="stat-row">
	<div class="stat">
		<div class="eyebrow">Events 24h</div>
		<div class="value">{events.value}<span class="unit">{events.unit}</span></div>
	</div>
	<div class="stat">
		<div class="eyebrow">Open alerts</div>
		<div class="value critical">{openAlertCount}</div>
	</div>
</div>

<style>
	.stat-row {
		display: flex;
		gap: var(--space-6);
		flex-wrap: wrap;
		padding: var(--space-5) var(--space-6);
	}
	.eyebrow {
		font-size: var(--text-eyebrow);
		text-transform: uppercase;
		letter-spacing: 0.1em;
		color: var(--color-muted-2);
	}
	.value {
		font-size: var(--text-big-stat);
		font-weight: 500;
		letter-spacing: -0.03em;
	}
	.value.critical {
		color: var(--color-severity-critical);
	}
	.unit {
		font-size: 22px;
		color: var(--color-muted);
	}
</style>
