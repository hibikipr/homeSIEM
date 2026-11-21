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
		background: var(--color-surface-2);
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
		font-size: var(--text-eyebrow);
		text-transform: uppercase;
		color: var(--color-severity-info-text);
	}
	.card.severity-critical .header {
		color: var(--color-severity-critical);
	}
	.card.severity-warning .header {
		color: var(--color-severity-warning);
	}
	.age {
		color: var(--color-muted);
		text-transform: none;
	}
	.title {
		font-size: 14px;
		font-weight: 500;
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
		gap: var(--space-3);
	}
	.primary {
		display: inline-block;
		background: transparent;
		border: 1px solid var(--color-accent);
		color: var(--color-text);
		border-radius: var(--radius-sm);
		padding: var(--space-1) var(--space-3);
		font-size: 11px;
		text-decoration: none;
	}
	.ghost {
		background: none;
		border: none;
		color: var(--color-accent-light);
		font-size: 11px;
	}
	.ghost:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}
	.error {
		display: block;
		margin-top: var(--space-2);
		font-size: 11px;
		color: var(--color-severity-critical);
	}
</style>
