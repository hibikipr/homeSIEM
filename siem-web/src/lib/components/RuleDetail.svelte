<script lang="ts">
	import type { RuleResponse } from '#lib/server/siemApiClient.ts';
	import { formatSecondsAsMinutes } from '#lib/minutePresets.ts';

	let {
		rule,
		canEdit = false,
		onToggled,
		onEdit,
		onDeleted
	}: {
		rule: RuleResponse;
		canEdit?: boolean;
		// Called after a successful enable/disable so the parent can refresh
		// its own data (the rule list this detail pane was rendered from).
		onToggled?: () => void;
		// Opens the parent's edit form for this rule.
		onEdit?: () => void;
		// Called after a successful delete so the parent can clear the
		// selection and refresh its rule list - this rule no longer exists.
		onDeleted?: () => void;
	} = $props();

	let toggling = $state(false);
	let deleting = $state(false);
	let error = $state<string | null>(null);

	async function deleteRule() {
		if (!confirm(`Delete "${rule.name}"? This can't be undone.`)) return;

		deleting = true;
		error = null;
		try {
			const response = await fetch(`/api/rules/${rule.id}`, { method: 'DELETE' });
			if (!response.ok) {
				error =
					response.status === 403
						? "You don't have permission to delete rules."
						: 'Failed to delete rule.';
				return;
			}
			onDeleted?.();
		} catch {
			error = 'Network error — check your connection and try again.';
		} finally {
			deleting = false;
		}
	}

	async function toggleEnabled() {
		toggling = true;
		error = null;
		try {
			// PUT replaces the whole rule, not a partial patch - send every
			// existing field back unchanged except enabled, which flips.
			const response = await fetch(`/api/rules/${rule.id}`, {
				method: 'PUT',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({
					name: rule.name,
					shape: rule.shape,
					logql: rule.logql,
					window_sec: rule.window_sec,
					threshold: rule.threshold,
					group_by: rule.group_by,
					severity: rule.severity,
					destinations: rule.destinations,
					cooldown_sec: rule.cooldown_sec,
					interval_sec: rule.interval_sec,
					enabled: !rule.enabled
				})
			});
			if (!response.ok) {
				error =
					response.status === 403
						? "You don't have permission to edit rules."
						: 'Failed to update rule.';
				return;
			}
			onToggled?.();
		} catch {
			error = 'Network error — check your connection and try again.';
		} finally {
			toggling = false;
		}
	}
</script>

<div class="detail">
	<div class="header">
		<span class="eyebrow">{rule.shape}</span>
		<div class="status">
			<span class="enabled" class:off={!rule.enabled}>{rule.enabled ? 'enabled' : 'disabled'}</span>
			{#if canEdit}
				<button class="toggle" onclick={onEdit}>Edit</button>
				<button class="toggle" onclick={toggleEnabled} disabled={toggling}>
					{toggling ? '…' : rule.enabled ? 'Disable' : 'Enable'}
				</button>
				<button class="toggle delete" onclick={deleteRule} disabled={deleting}>
					{deleting ? '…' : 'Delete'}
				</button>
			{/if}
		</div>
	</div>
	<h1>{rule.name}</h1>
	<div class="meta">
		<span>severity: {rule.severity}</span>
		{#if rule.shape === 'absence'}
			<!-- Found in production: this used to show unconditionally as a
			     bare "window: 1m" for an absence rule, which read as
			     inconsistent against the 900-second heartbeat cited in the
			     rule's own alert text - they never reconciled because
			     AbsenceEvaluator doesn't use window_sec to decide staleness
			     at all (it checks each source's own HeartbeatSec). Rather
			     than hide a field that could carry real meaning,
			     window_sec is repurposed for this shape specifically as
			     the correlation window: sources going quiet within this
			     long of each other raise one combined alert instead of one
			     per source (rules.correlatedCandidate). Labeled
			     accordingly so it doesn't look like the old, unrelated
			     "detection window" meaning threshold/first_seen use. -->
			<span>correlation window: {formatSecondsAsMinutes(rule.window_sec)}</span>
		{:else if rule.shape !== 'insight'}
			<!-- insight doesn't use window_sec either (InsightEvaluator
			     reads only severity and last_run_at) and has no comparable
			     use to repurpose it for - stays hidden, matching RuleForm. -->
			<span>window: {formatSecondsAsMinutes(rule.window_sec)}</span>
		{/if}
		<span>cooldown: {formatSecondsAsMinutes(rule.cooldown_sec)}</span>
		<span>evaluates every: {formatSecondsAsMinutes(rule.interval_sec)}</span>
	</div>
	<div class="destinations">destinations: {rule.destinations.join(', ')}</div>
	<div class="logql-block">{rule.logql}</div>
	{#if error}
		<p class="error">{error}</p>
	{/if}
</div>

<style>
	.detail {
		flex: 1;
		min-width: 0;
		background: var(--color-surface);
		border: 1px solid var(--color-line);
		border-radius: var(--radius-default);
		padding: var(--space-5);
	}
	.header {
		display: flex;
		justify-content: space-between;
		align-items: center;
	}
	.eyebrow {
		font-size: var(--text-eyebrow);
		text-transform: uppercase;
		color: var(--color-muted-2);
	}
	.status {
		display: flex;
		align-items: center;
		gap: var(--space-2);
	}
	.enabled {
		font-size: var(--text-eyebrow);
		text-transform: uppercase;
		letter-spacing: 0.06em;
		background: var(--color-status-success-bg);
		color: var(--saber-green);
		border-radius: var(--radius-pill);
		padding: 1px var(--space-3);
	}
	.enabled.off {
		background: transparent;
		box-shadow: inset 0 0 0 1px var(--color-line-2);
		color: var(--color-muted-2);
	}
	.toggle {
		background: transparent;
		border: 1px solid var(--color-line-2);
		color: var(--color-text-2);
		border-radius: var(--radius-default);
		font-family: var(--font-ui);
		cursor: pointer;
		transition:
			color 0.2s,
			border-color 0.2s,
			background-color 0.2s;
		padding: var(--space-1) var(--space-3);
		font-size: var(--text-label);
	}
	.toggle:hover:not(:disabled) {
		border-color: var(--saber-blue);
		color: var(--color-text);
	}
	.toggle:disabled {
		opacity: 0.6;
		cursor: default;
	}
	.toggle.delete {
		color: var(--saber-red);
	}
	h1 {
		font-family: var(--font-display);
		font-size: 20px;
		font-weight: 400;
		letter-spacing: 0.06em;
		color: var(--color-text);
		text-wrap: balance;
		overflow-wrap: anywhere;
		margin: var(--space-2) 0 0;
	}
	.meta {
		display: flex;
		gap: var(--space-4);
		font-size: var(--text-table);
		color: var(--color-muted);
		margin-top: var(--space-3);
	}
	.destinations {
		font-size: var(--text-table);
		color: var(--color-muted);
		margin-top: var(--space-2);
	}
	.logql-block {
		font-family: var(--font-mono);
		font-size: var(--text-label);
		color: var(--color-muted);
		background: var(--color-surface-2);
		border: 1px solid var(--color-line);
		border-radius: var(--radius-sm);
		padding: var(--space-2) var(--space-3);
		margin-top: var(--space-3);
		overflow-x: auto;
		white-space: nowrap;
	}
	.error {
		color: var(--saber-red);
		font-size: var(--text-label);
		margin: var(--space-2) 0 0;
	}
</style>
