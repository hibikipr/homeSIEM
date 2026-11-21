<script lang="ts">
	import { invalidate } from '$app/navigation';
	import { DEP_SOURCES } from '#lib/invalidation.ts';
	import { SvelteSet } from 'svelte/reactivity';
	import type { SourceResponse } from '#lib/server/siemApiClient.ts';

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
			await invalidate(DEP_SOURCES);
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
		background: var(--color-surface);
		border: 1px solid var(--color-line);
		border-radius: var(--radius-default);
		padding: var(--space-4);
		margin-top: var(--space-4);
	}
	h2 {
		font-size: var(--text-section-head);
		font-weight: 600;
		letter-spacing: 0.02em;
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
		font-family: var(--font-mono);
		font-weight: 500;
	}
	.meta {
		font-family: var(--font-mono);
		color: var(--color-muted-2);
		font-size: var(--text-label);
	}
	button {
		background: var(--saber-blue);
		border: 1px solid var(--saber-blue);
		color: var(--color-on-saber);
		font-family: var(--font-ui);
		font-weight: 600;
		letter-spacing: 0.02em;
		cursor: pointer;
		transition:
			background-color 0.2s,
			border-color 0.2s,
			color 0.2s;
		border-radius: var(--radius-default);
		padding: var(--space-1) var(--space-3);
		font-size: var(--text-label);
	}
	button:hover:not(:disabled) {
		background: var(--color-btn-primary-hover);
		border-color: var(--color-btn-primary-hover);
		box-shadow: var(--glow-primary-hover);
	}

	button:disabled {
		opacity: 0.6;
		cursor: default;
	}
	.error {
		display: block;
		margin-top: var(--space-2);
		font-size: var(--text-label);
		color: var(--saber-red);
	}
</style>
