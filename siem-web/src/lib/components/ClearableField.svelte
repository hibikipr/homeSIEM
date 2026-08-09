<script lang="ts">
	let {
		value = $bindable(),
		placeholder,
		wide = false,
		onClear
	}: {
		value: string;
		placeholder: string;
		wide?: boolean;
		// Called after clearing, so a parent using this as a live filter
		// (not just a plain form field) can re-run its search immediately -
		// found via feedback that clicking clear emptied the box but left
		// the previous results on screen until Enter was pressed separately.
		onClear?: () => void;
	} = $props();

	let inputEl: HTMLInputElement | undefined = $state();

	function clear() {
		value = '';
		inputEl?.focus();
		onClear?.();
	}
</script>

<div class="clearable" class:wide>
	<input class="field" bind:value bind:this={inputEl} {placeholder} />
	{#if value}
		<button type="button" class="clear-btn" aria-label="Clear {placeholder}" onclick={clear}>
			<i class="ph ph-x"></i>
		</button>
	{/if}
</div>

<style>
	.clearable {
		position: relative;
		display: inline-flex;
		width: 110px;
	}
	.clearable.wide {
		flex: 1 1 200px;
		width: auto;
	}
	.field {
		background: var(--color-surface-2);
		border: none;
		border-radius: var(--radius-sm);
		color: var(--color-text);
		padding: var(--space-1) var(--space-2);
		/* Room for the clear button so typed text never sits under it. */
		padding-right: var(--space-5);
		font-size: var(--text-table);
		width: 100%;
	}
	.clear-btn {
		position: absolute;
		right: var(--space-1);
		top: 50%;
		transform: translateY(-50%);
		background: transparent;
		border: none;
		color: var(--color-muted);
		cursor: pointer;
		font-size: var(--text-table);
		line-height: 1;
		padding: 0 var(--space-1);
		border-radius: var(--radius-sm);
	}
	.clear-btn:hover {
		color: var(--color-text);
		background: var(--color-surface);
	}
</style>
