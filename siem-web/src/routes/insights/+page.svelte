<script lang="ts">
	import { invalidateAll } from '$app/navigation';
	import { severityColor } from '$lib/tail';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	let generating = $state(false);
	let generateError = $state<string | null>(null);
	let generateNotice = $state<string | null>(null);
	let dismissingId = $state<number | null>(null);
	let expandedId = $state<number | null>(null);

	async function generateNow() {
		generating = true;
		generateError = null;
		generateNotice = null;
		try {
			const response = await fetch('/api/insights/generate', { method: 'POST' });
			if (!response.ok) {
				const body = await response.json().catch(() => ({}));
				generateError = body.error ?? 'Failed to generate insights.';
				return;
			}
			const body = await response.json();
			if (body.generated === 0) {
				generateNotice = 'Pass complete: no new insights this time.';
			}
			await invalidateAll();
		} finally {
			generating = false;
		}
	}

	async function dismiss(id: number) {
		dismissingId = id;
		try {
			const response = await fetch(`/api/insights/${id}`, { method: 'PUT' });
			if (response.ok) {
				await invalidateAll();
			}
		} finally {
			dismissingId = null;
		}
	}

	function toggleExpand(id: number) {
		expandedId = expandedId === id ? null : id;
	}
</script>

<div class="insights-screen">
	<header class="screen-header">
		<h1>Insights</h1>
		<div class="actions">
			<button class="action" onclick={generateNow} disabled={generating}>
				{generating ? 'Generating…' : 'Generate now'}
			</button>
		</div>
	</header>
	{#if generateError}
		<p class="generate-error">{generateError}</p>
	{:else if generateNotice}
		<p class="generate-notice">{generateNotice}</p>
	{/if}

	{#if data.insights.length === 0}
		<p class="empty">
			No insights yet. Insights are generated on a schedule, or you can trigger a pass with
			"Generate now" above.
		</p>
	{:else}
		<ul class="list">
			{#each data.insights as insight (insight.id)}
				<li class="row" class:dismissed={insight.dismissed}>
					<button class="row-main" onclick={() => toggleExpand(insight.id)}>
						<span class="dot" style:background={severityColor(insight.severity)}></span>
						<div class="text">
							<div class="row-title-line">
								<span class="row-title">{insight.title}</span>
								<span class="category">{insight.category}</span>
								{#if insight.dismissed}
									<span class="dismissed-badge">Dismissed</span>
								{/if}
							</div>
							<span class="created-at">{new Date(insight.created_at).toLocaleString()}</span>
						</div>
					</button>
					{#if !insight.dismissed}
						<button
							class="dismiss"
							onclick={() => dismiss(insight.id)}
							disabled={dismissingId === insight.id}
						>
							Dismiss
						</button>
					{/if}
				</li>
				{#if expandedId === insight.id}
					<li class="detail">
						<p>{insight.detail}</p>
						{#if insight.evidence.length > 0}
							<table class="evidence">
								<thead>
									<tr>
										<th>Program</th>
										<th>Sample message</th>
										<th>Count</th>
									</tr>
								</thead>
								<tbody>
									{#each insight.evidence as ev, i (i)}
										<tr>
											<td>{ev.program}</td>
											<td class="sample">{ev.sample_message}</td>
											<td>{ev.count}</td>
										</tr>
									{/each}
								</tbody>
							</table>
						{/if}
					</li>
				{/if}
			{/each}
		</ul>
	{/if}
</div>

<style>
	.insights-screen {
		padding: var(--space-5) var(--space-6);
	}
	.screen-header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		margin-bottom: var(--space-4);
	}
	h1 {
		font-size: var(--text-page-title);
		margin: 0;
	}
	.action {
		background: var(--color-surface-2);
		color: var(--color-text);
		border: none;
		border-radius: var(--radius-sm);
		padding: var(--space-1) var(--space-3);
		font-size: var(--text-label);
		cursor: pointer;
	}
	.action:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}
	.generate-error {
		color: var(--color-severity-critical);
		font-size: 12px;
	}
	.generate-notice {
		color: var(--color-muted);
		font-size: 12px;
	}
	.empty {
		color: var(--color-muted);
		font-size: 13px;
	}
	.list {
		list-style: none;
		margin: 0;
		padding: 0;
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
	}
	.row {
		display: flex;
		align-items: center;
		gap: var(--space-3);
		background: var(--color-surface-2);
		border-radius: var(--radius-default);
		padding: var(--space-2) var(--space-3);
	}
	.row.dismissed {
		opacity: 0.55;
	}
	.row-main {
		flex: 1;
		display: flex;
		align-items: center;
		gap: var(--space-3);
		background: none;
		border: none;
		text-align: left;
		cursor: pointer;
		padding: 0;
		min-width: 0;
	}
	.dot {
		flex-shrink: 0;
		width: 8px;
		height: 8px;
		border-radius: 50%;
	}
	.text {
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: 2px;
	}
	.row-title-line {
		display: flex;
		align-items: center;
		gap: var(--space-2);
	}
	.row-title {
		font-size: 13px;
		font-weight: 500;
		color: var(--color-text);
	}
	.category {
		font-size: 10px;
		text-transform: uppercase;
		color: var(--color-muted-2);
	}
	.dismissed-badge {
		font-size: 10px;
		text-transform: uppercase;
		color: var(--color-muted-2);
		border: 1px solid var(--color-line-2);
		border-radius: var(--radius-sm);
		padding: 0 var(--space-1);
	}
	.created-at {
		font-size: 11px;
		color: var(--color-muted);
	}
	.dismiss {
		flex-shrink: 0;
		background: none;
		border: none;
		color: var(--color-accent-light);
		font-size: 11px;
		cursor: pointer;
	}
	.dismiss:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}
	.detail {
		background: var(--color-surface-3);
		border-radius: var(--radius-default);
		padding: var(--space-3) var(--space-4);
		font-size: 12px;
		color: var(--color-text-2);
	}
	.detail p {
		margin: 0 0 var(--space-2);
	}
	.evidence {
		width: 100%;
		border-collapse: collapse;
		font-size: 11.5px;
	}
	.evidence th {
		text-align: left;
		text-transform: uppercase;
		font-size: var(--text-eyebrow);
		color: var(--color-muted-2);
		padding: var(--space-1) var(--space-2);
	}
	.evidence td {
		padding: var(--space-1) var(--space-2);
		border-top: 1px solid var(--color-line-2);
	}
	.evidence .sample {
		font-family: var(--font-mono);
		color: var(--color-muted);
	}
</style>
