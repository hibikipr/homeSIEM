<script lang="ts">
	import { invalidateAll } from '$app/navigation';
	import type { AlertResponse, AlertSample, RuleResponse } from '$lib/server/siemApiClient';
	import type { AlertStats } from '$lib/alerts';
	import { extractMessage } from '$lib/logline';

	let {
		alert,
		samples,
		stats,
		rule
	}: {
		alert: AlertResponse;
		samples: AlertSample[];
		stats: AlertStats;
		rule: RuleResponse | undefined;
	} = $props();

	let acking = $state(false);
	let muting = $state(false);
	let error = $state<string | null>(null);

	async function acknowledge() {
		acking = true;
		error = null;
		try {
			const response = await fetch(`/api/alerts/${alert.id}/ack`, { method: 'POST' });
			if (!response.ok) {
				error = 'Failed to acknowledge alert.';
				return;
			}
			await invalidateAll();
		} finally {
			acking = false;
		}
	}

	async function mute() {
		muting = true;
		error = null;
		try {
			const response = await fetch(`/api/alerts/${alert.id}/mute`, { method: 'POST' });
			if (!response.ok) {
				error = 'Failed to mute rule.';
				return;
			}
			await invalidateAll();
		} finally {
			muting = false;
		}
	}
</script>

<div class="detail">
	<div class="header">
		<div class="title-block">
			<div class="eyebrow-row">
				<span class="eyebrow severity-{alert.severity}">{alert.severity}</span>
				{#if alert.state === 'open'}
					<span class="tag">unacknowledged</span>
				{/if}
			</div>
			<h1>{alert.title}</h1>
			<p class="body">{alert.body}</p>
		</div>
		<div class="actions">
			<button class="primary" onclick={acknowledge} disabled={acking || alert.state !== 'open'}>
				Acknowledge
			</button>
			<button
				class="ghost"
				disabled
				title="Not implemented — SOAR-style automated response is out of scope for v1"
			>
				Block at gateway
			</button>
			<button class="ghost" onclick={mute} disabled={muting}>Mute rule 1h</button>
			{#if error}
				<span class="error">{error}</span>
			{/if}
		</div>
	</div>

	<div class="stats">
		<div class="stat">
			<span class="label">Matched events</span>
			<span class="value">{stats.matchedEvents}</span>
		</div>
		<div class="stat">
			<span class="label">Distinct ports</span>
			<span class="value">{stats.distinctPorts.length}</span>
		</div>
		<div class="stat">
			<span class="label">Source IP</span>
			<span class="value">{stats.sourceIps[0] ?? '—'}</span>
		</div>
		<div class="stat">
			<span class="label">Reputation</span>
			<span class="value">{stats.reputation}</span>
		</div>
	</div>

	{#if stats.distinctPorts.length > 0}
		<div class="ports">
			<span class="label">Ports touched, in order</span>
			<div class="chips">
				{#each stats.distinctPorts as port (port)}
					<span class="chip">{port}</span>
				{/each}
			</div>
		</div>
	{/if}

	<div class="matched-events">
		<span class="label">Matched events</span>
		<div class="log-block">
			{#each samples as sample (sample.id)}
				<div class="log-line">{extractMessage(sample.line)}</div>
			{:else}
				<div class="log-line empty">No samples recorded yet.</div>
			{/each}
		</div>
	</div>

	<div class="rule-panel">
		<span class="label">Rule that fired</span>
		{#if rule}
			<div class="rule-name">{rule.name}</div>
			<div class="rule-meta">
				<span class="enabled" class:off={!rule.enabled}
					>{rule.enabled ? 'enabled' : 'disabled'}</span
				>
				<span class="destinations">{rule.destinations.join(', ')}</span>
			</div>
			<div class="logql-block">{rule.logql}</div>
		{:else}
			<div class="rule-name">rule #{alert.rule_id}</div>
		{/if}
	</div>
</div>

<style>
	.detail {
		flex: 1;
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: var(--space-5);
	}
	.header {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-4);
	}
	.title-block {
		flex: 1 1 380px;
	}
	.eyebrow-row {
		display: flex;
		align-items: center;
		gap: var(--space-3);
	}
	.eyebrow {
		font-size: var(--text-eyebrow);
		text-transform: uppercase;
		color: var(--color-severity-info-text);
	}
	.eyebrow.severity-warning {
		color: var(--color-severity-warning);
	}
	.eyebrow.severity-critical {
		color: var(--color-severity-critical);
	}
	.tag {
		font-size: var(--text-eyebrow);
		text-transform: uppercase;
		background: var(--color-accent-tint);
		color: var(--color-accent-lighter);
		border-radius: var(--radius-sm);
		padding: 0 var(--space-2);
	}
	h1 {
		font-size: 26px;
		font-weight: 500;
		margin: var(--space-2) 0 0;
	}
	.body {
		max-width: 68ch;
		color: var(--color-muted);
		margin-top: var(--space-2);
	}
	.actions {
		display: flex;
		gap: var(--space-3);
		align-items: flex-start;
	}
	.primary {
		background: transparent;
		border: 1px solid var(--color-accent);
		color: var(--color-text);
		border-radius: var(--radius-sm);
		padding: var(--space-2) var(--space-4);
		font-size: var(--text-table);
	}
	.ghost {
		background: none;
		border: 1px solid var(--color-line-2);
		color: var(--color-accent-light);
		border-radius: var(--radius-sm);
		padding: var(--space-2) var(--space-4);
		font-size: var(--text-table);
	}
	.ghost:disabled,
	.primary:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}
	.error {
		align-self: center;
		font-size: var(--text-table);
		color: var(--color-severity-critical);
	}
	.stats {
		display: grid;
		grid-template-columns: repeat(4, 1fr);
		gap: var(--space-4);
	}
	.stat {
		background: var(--color-surface-2);
		border-radius: var(--radius-default);
		padding: var(--space-4);
		display: flex;
		flex-direction: column;
		gap: var(--space-1);
	}
	.label {
		font-size: var(--text-eyebrow);
		text-transform: uppercase;
		color: var(--color-muted-2);
	}
	.value {
		font-size: 20px;
		font-weight: 500;
	}
	.chips {
		display: flex;
		gap: var(--space-2);
		margin-top: var(--space-2);
		flex-wrap: wrap;
	}
	.chip {
		font-family: var(--font-mono);
		font-size: var(--text-label);
		background: var(--color-accent-tint);
		color: var(--color-accent-lighter);
		border-radius: var(--radius-sm);
		padding: var(--space-1) var(--space-2);
	}
	.log-block {
		background: var(--color-surface-2);
		border-radius: var(--radius-default);
		padding: var(--space-3);
		margin-top: var(--space-2);
		font-family: var(--font-mono);
		font-size: var(--text-log-row);
		line-height: var(--line-height-log);
	}
	.log-line {
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}
	.log-line.empty {
		color: var(--color-muted-2);
	}
	.rule-panel {
		background: var(--color-surface-2);
		border-radius: var(--radius-default);
		padding: var(--space-4);
	}
	.rule-name {
		font-size: 13.5px;
		font-weight: 500;
		margin-top: var(--space-2);
	}
	.rule-meta {
		display: flex;
		gap: var(--space-3);
		font-size: var(--text-table);
		color: var(--color-muted);
		margin-top: var(--space-1);
	}
	.enabled {
		text-transform: uppercase;
		font-size: var(--text-eyebrow);
		color: var(--color-severity-healthy);
	}
	.enabled.off {
		color: var(--color-muted-2);
	}
	.logql-block {
		font-family: var(--font-mono);
		font-size: var(--text-label);
		color: var(--color-muted);
		background: var(--color-surface-3);
		border-radius: var(--radius-sm);
		padding: var(--space-2) var(--space-3);
		margin-top: var(--space-2);
		overflow-x: auto;
		white-space: nowrap;
	}
</style>
