<script lang="ts">
	import { alertOverrides, setAlertState } from '#lib/optimisticAlerts.svelte.ts';
	import { resolve } from '$app/paths';
	import type { AlertResponse } from '#lib/server/siemApiClient.ts';

	let { alert }: { alert: AlertResponse } = $props();

	let muting = $state(false);
	let error = $state<string | null>(null);
	// Optimistic: once muted the alert leaves the triage lane immediately; a
	// failed request clears the override and the card comes back with its error.
	let hidden = $derived(alertOverrides.has(alert.id));

	function ageLabel(iso: string): string {
		const ms = Date.now() - new Date(iso).getTime();
		const minutes = Math.floor(ms / 60000);
		if (minutes < 60) return `${minutes}m`;
		return `${Math.floor(minutes / 60)}h`;
	}

	async function mute() {
		muting = true;
		error = null;
		try {
			if (!(await setAlertState(alert.id, 'mute'))) {
				error = 'Failed to mute alert.';
			}
		} finally {
			muting = false;
		}
	}
</script>

{#if !hidden}
	<div class="card severity-{alert.severity}">
		<div class="header">
			<span class="eyebrow">{alert.severity}</span>
			<span class="age">{ageLabel(alert.first_seen_at)}</span>
		</div>
		<div class="title">{alert.title}</div>
		<div class="body">{alert.body}</div>
		<div class="actions">
			<a class="primary" href={resolve(`alerts?id=${alert.id}`)}>Investigate</a>

			<button class="ghost" onclick={mute} disabled={muting}>Mute 1h</button>
		</div>
		{#if error}
			<span class="error">{error}</span>
		{/if}
	</div>
{/if}

<style>
	.card {
		background: var(--color-surface);
		border: 1px solid var(--color-line);
		border-radius: var(--radius-default);
		padding: var(--space-4);
		box-shadow: inset 0 2px 0 var(--color-severity-info);
	}
	.card.severity-critical {
		box-shadow: inset 0 2px 0 var(--color-severity-critical);
	}
	.card.severity-warning {
		box-shadow: inset 0 2px 0 var(--color-severity-warning);
	}
	.header {
		display: flex;
		justify-content: space-between;
		align-items: center;
		font-size: var(--text-eyebrow);
		text-transform: uppercase;
	}
	.eyebrow {
		background: var(--row-selected-bg);
		color: var(--color-severity-info-text);
		border-radius: var(--radius-pill);
		padding: 1px var(--space-3);
		letter-spacing: 0.06em;
	}
	.card.severity-critical .eyebrow {
		background: var(--color-status-danger-soft);
		color: var(--saber-red);
	}
	.card.severity-warning .eyebrow {
		background: var(--color-status-warn-bg);
		color: var(--color-gold);
	}
	.age {
		font-family: var(--font-mono);
		color: var(--color-muted);
		text-transform: none;
	}
	.title {
		font-size: 14px;
		font-weight: 600;
		letter-spacing: 0.02em;
		margin-top: var(--space-2);
	}
	.body {
		font-size: 11.5px;
		color: var(--color-muted);
		margin-top: var(--space-1);
	}
	.actions {
		margin-top: var(--space-3);
		display: flex;
		align-items: center;
		gap: var(--space-3);
	}
	.primary {
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
		display: inline-block;
		padding: var(--space-1) var(--space-3);
		font-size: 11px;
		text-decoration: none;
	}
	.primary:hover:not(:disabled) {
		background: var(--color-btn-primary-hover);
		border-color: var(--color-btn-primary-hover);
		box-shadow: var(--glow-primary-hover);
	}
	.ghost {
		background: none;
		border: none;
		color: var(--color-text-2);
		font-family: var(--font-ui);
		font-size: 11px;
		cursor: pointer;
		transition: color 0.2s;
	}
	.ghost:hover:not(:disabled) {
		color: var(--color-gold);
	}
	.ghost:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}
	.error {
		display: block;
		margin-top: var(--space-2);
		font-size: 11px;
		color: var(--saber-red);
	}
</style>
