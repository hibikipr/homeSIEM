<script lang="ts">
	import { invalidateAll } from '$app/navigation';
	import { SvelteSet } from 'svelte/reactivity';
	import type { SourceResponse } from '$lib/server/siemApiClient';

	let {
		sources,
		canClaim
	}: {
		sources: SourceResponse[];
		canClaim: boolean;
	} = $props();

	let pending = new SvelteSet<number>();
	let error = $state<string | null>(null);

	async function claim(id: number) {
		pending.add(id);
		error = null;
		try {
			const response = await fetch(`/api/sources/${id}/claim`, { method: 'POST' });
			if (!response.ok) {
				error = 'Failed to claim source.';
				return;
			}
			await invalidateAll();
		} finally {
			pending.delete(id);
		}
	}
</script>

<section class="panel">
	<h2>Unclaimed senders</h2>
	{#if sources.length === 0}
		<p class="empty">Every known sender has been claimed.</p>
	{:else}
		<ul>
			{#each sources as source (source.id)}
				<li>
					<div>
						<div class="name">{source.name}</div>
						<div class="mono meta">{source.address} · {source.transport}</div>
					</div>
					{#if canClaim}
						<button onclick={() => claim(source.id)} disabled={pending.has(source.id)}>
							{pending.has(source.id) ? 'Claiming…' : 'Claim'}
						</button>
					{/if}
				</li>
			{/each}
		</ul>
	{/if}
	{#if error}
		<span class="error">{error}</span>
	{/if}
</section>

<style>
	.panel {
		background: var(--color-surface-2);
		border-radius: var(--radius-default);
		padding: var(--space-4);
		margin-top: var(--space-4);
	}
	h2 {
		font-size: var(--text-section-head);
		color: var(--color-muted);
		margin: 0 0 var(--space-3);
	}
	.empty {
		font-size: var(--text-table);
		color: var(--color-muted-2);
		margin: 0;
	}
	ul {
		list-style: none;
		margin: 0;
		padding: 0;
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
	}
	li {
		display: flex;
		justify-content: space-between;
		align-items: center;
		font-size: var(--text-table);
	}
	.name {
		font-weight: 500;
	}
	.meta {
		color: var(--color-muted-2);
		font-size: var(--text-label);
	}
	button {
		background: var(--color-accent-tint-2);
		color: var(--color-accent-lighter);
		border: none;
		border-radius: var(--radius-sm);
		padding: var(--space-1) var(--space-3);
		font-size: var(--text-label);
		cursor: pointer;
	}
	button:disabled {
		opacity: 0.6;
		cursor: default;
	}
	.error {
		display: block;
		margin-top: var(--space-2);
		font-size: var(--text-label);
		color: var(--color-severity-critical);
	}
</style>
