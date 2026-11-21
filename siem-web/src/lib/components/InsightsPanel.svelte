<script lang="ts">
	import { invalidate } from '$app/navigation';
	import { DEP_INSIGHTS } from '#lib/invalidation.ts';
	import { resolve } from '$app/paths';
	import { severityColor } from '#lib/tail.ts';
	import { uniquePrograms } from '#lib/insights.ts';
	import type { Insight } from '#lib/server/siemApiClient.ts';

	let { insights, loading = false }: { insights: Insight[]; loading?: boolean } = $props();

	const PANEL_CAP = 5;
	let visible = $derived(insights.slice(0, PANEL_CAP));

	let dismissing = $state<number | null>(null);

	async function dismiss(id: number) {
		dismissing = id;
		try {
			const response = await fetch(`/api/insights/${id}`, { method: 'PUT' });
			if (response.ok) {
				await invalidate(DEP_INSIGHTS);
			}
		} finally {
			dismissing = null;
		}
	}
</script>

<div class="panel">
	<div class="panel-head">
		<span class="title">Insights</span>
		<a class="view-all" href={resolve('insights')}>View all →</a>
	</div>
	{#if loading}
		<p class="empty">Loading…</p>
	{:else if visible.length === 0}
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
							{#if insight.recommended_fix}
								<span class="fix-badge" title="A recommended fix is available">Fix</span>
							{/if}
						</div>
						<div class="row-detail">{insight.detail}</div>
						{#if uniquePrograms(insight.evidence).length > 0}
							<div class="row-programs">{uniquePrograms(insight.evidence).join(', ')}</div>
						{/if}
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
		background: var(--color-surface);
		border: 1px solid var(--color-line);
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
		font-weight: 600;
		letter-spacing: 0.06em;
		text-transform: uppercase;
		color: var(--color-muted-2);
	}
	.view-all {
		font-size: 11px;
		color: var(--saber-blue);
		text-decoration: none;
		transition: color 0.2s;
	}
	.view-all:hover {
		color: var(--color-gold);
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
		font-weight: 600;
		letter-spacing: 0.02em;
		color: var(--color-text);
	}
	.occurrence-badge {
		font-size: 10px;
		font-weight: 600;
		font-family: var(--font-mono);
		color: var(--color-muted-2);
		background: var(--color-surface-3);
		border-radius: var(--radius-pill);
		padding: 0 var(--space-2);
		margin-left: var(--space-1);
	}
	.fix-badge {
		font-size: 10px;
		font-weight: 600;
		text-transform: uppercase;
		color: var(--saber-blue);
		border: 1px solid var(--saber-blue);
		border-radius: var(--radius-pill);
		padding: 0 var(--space-2);
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
	.row-programs {
		font-family: var(--font-mono);
		font-size: 10.5px;
		color: var(--color-muted-2);
		margin-top: 2px;
	}
	.dismiss {
		flex-shrink: 0;
		background: none;
		border: none;
		color: var(--color-muted-2);
		cursor: pointer;
		font-family: var(--font-ui);
		font-size: 11px;
		padding: 0 var(--space-1);
		transition: color 0.2s;
	}
	.dismiss:hover:not(:disabled) {
		color: var(--color-gold);
	}
	.dismiss:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}
</style>
