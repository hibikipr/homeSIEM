<script lang="ts">
	import { invalidateAll } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { severityColor } from '$lib/tail';
	import type { Insight } from '$lib/server/siemApiClient';

	let { insights }: { insights: Insight[] } = $props();

	const PANEL_CAP = 5;
	let visible = $derived(insights.slice(0, PANEL_CAP));

	let dismissing = $state<number | null>(null);

	async function dismiss(id: number) {
		dismissing = id;
		try {
			const response = await fetch(`/api/insights/${id}`, { method: 'PUT' });
			if (response.ok) {
				await invalidateAll();
			}
		} finally {
			dismissing = null;
		}
	}
</script>

<div class="panel">
	<div class="panel-head">
		<span class="title">Insights</span>
		<a class="view-all" href={resolve('/insights')}>View all →</a>
	</div>
	{#if visible.length === 0}
		<p class="empty">No insights yet.</p>
	{:else}
		<ul class="list">
			{#each visible as insight (insight.id)}
				<li class="row">
					<span class="dot" style:background={severityColor(insight.severity)}></span>
					<div class="text">
						<div class="row-title">
							{insight.title}
							{#if insight.occurrence_count > 1}
								<span class="occurrence-badge">×{insight.occurrence_count}</span>
							{/if}
						</div>
						<div class="row-detail">{insight.detail}</div>
					</div>
					<button
						class="dismiss"
						onclick={() => dismiss(insight.id)}
						disabled={dismissing === insight.id}
						aria-label={`Dismiss: ${insight.title}`}
					>
						✕
					</button>
				</li>
			{/each}
		</ul>
	{/if}
</div>

<style>
	.panel {
		background: var(--color-surface-2);
		border-radius: var(--radius-default);
		padding: var(--space-4);
	}
	.panel-head {
		display: flex;
		justify-content: space-between;
		align-items: baseline;
		margin-bottom: var(--space-3);
	}
	.title {
		font-size: var(--text-eyebrow);
		text-transform: uppercase;
		color: var(--color-muted-2);
	}
	.view-all {
		font-size: 11px;
		color: var(--color-accent-light);
		text-decoration: none;
	}
	.empty {
		font-size: 12px;
		color: var(--color-muted);
		margin: 0;
	}
	.list {
		list-style: none;
		margin: 0;
		padding: 0;
		display: flex;
		flex-direction: column;
		gap: var(--space-3);
	}
	.row {
		display: flex;
		align-items: flex-start;
		gap: var(--space-2);
	}
	.dot {
		flex-shrink: 0;
		width: 8px;
		height: 8px;
		border-radius: 50%;
		margin-top: 5px;
	}
	.text {
		flex: 1;
		min-width: 0;
	}
	.row-title {
		font-size: 12.5px;
		font-weight: 500;
		color: var(--color-text);
	}
	.occurrence-badge {
		font-size: 10px;
		font-weight: 600;
		color: var(--color-muted-2);
		background: var(--color-surface-3);
		border-radius: var(--radius-sm);
		padding: 0 var(--space-1);
		margin-left: var(--space-1);
	}
	.row-detail {
		font-size: 11px;
		color: var(--color-muted);
		margin-top: 2px;
		overflow: hidden;
		text-overflow: ellipsis;
		display: -webkit-box;
		-webkit-line-clamp: 2;
		line-clamp: 2;
		-webkit-box-orient: vertical;
	}
	.dismiss {
		flex-shrink: 0;
		background: none;
		border: none;
		color: var(--color-muted-2);
		cursor: pointer;
		font-size: 11px;
		padding: 0 var(--space-1);
	}
	.dismiss:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}
</style>
