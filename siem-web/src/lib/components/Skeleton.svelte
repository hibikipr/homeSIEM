<script lang="ts">
	let { height = '1rem', width = '100%' }: { height?: string; width?: string } = $props();
</script>

<div class="skeleton" style:height style:width aria-hidden="true"></div>

<style>
	.skeleton {
		background: var(--color-surface-2);
		border-radius: var(--radius-sm);
		animation: pulse 1.4s ease-in-out infinite;
	}
	@keyframes pulse {
		50% {
			opacity: 0.5;
		}
	}
	@media (prefers-reduced-motion: reduce) {
		.skeleton {
			animation: none;
		}
	}
</style>
