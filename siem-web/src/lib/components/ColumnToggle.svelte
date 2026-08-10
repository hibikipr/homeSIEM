<script lang="ts">
	let {
		columns,
		hidden,
		onToggle
	}: {
		columns: { key: string; label: string }[];
		hidden: ReadonlySet<string>;
		onToggle: (key: string) => void;
	} = $props();

	let menuOpen = $state(false);
	let menuEl = $state<HTMLDivElement | undefined>();
	let menuButtonEl = $state<HTMLButtonElement | undefined>();

	function toggleMenu() {
		menuOpen = !menuOpen;
	}

	function closeMenu() {
		menuOpen = false;
	}

	function handleWindowClick(event: MouseEvent) {
		if (!menuOpen) return;
		const target = event.target as Node;
		if (menuEl?.contains(target) || menuButtonEl?.contains(target)) return;
		closeMenu();
	}

	function handleWindowKeydown(event: KeyboardEvent) {
		if (event.key === 'Escape' && menuOpen) {
			closeMenu();
			menuButtonEl?.focus();
		}
	}
</script>

<svelte:window onclick={handleWindowClick} onkeydown={handleWindowKeydown} />

<div class="column-toggle">
	<button
		bind:this={menuButtonEl}
		type="button"
		class="toggle-button"
		aria-haspopup="true"
		aria-expanded={menuOpen}
		onclick={toggleMenu}
	>
		Columns
	</button>
	{#if menuOpen}
		<div bind:this={menuEl} class="menu" role="menu">
			{#each columns as col (col.key)}
				<label class="option">
					<input
						type="checkbox"
						checked={!hidden.has(col.key)}
						onchange={() => onToggle(col.key)}
					/>
					{col.label}
				</label>
			{/each}
		</div>
	{/if}
</div>

<style>
	.column-toggle {
		position: relative;
	}
	.toggle-button {
		background: var(--color-surface-2);
		color: var(--color-text);
		border: none;
		border-radius: var(--radius-sm);
		padding: var(--space-1) var(--space-3);
		font-size: var(--text-label);
		cursor: pointer;
	}
	.menu {
		position: absolute;
		top: calc(100% + var(--space-2));
		right: 0;
		min-width: 160px;
		background: var(--color-surface-2);
		border-radius: var(--radius-default);
		box-shadow: var(--shadow-flat);
		padding: var(--space-3);
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
		z-index: 20;
	}
	.option {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		font-size: var(--text-label);
		color: var(--color-text);
		cursor: pointer;
		white-space: nowrap;
	}
</style>
