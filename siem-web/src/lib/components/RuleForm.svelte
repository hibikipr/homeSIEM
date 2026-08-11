<script lang="ts">
	import { RULE_TEMPLATES, parseGroupBy, type RuleShape } from '$lib/ruleTemplates';
	import type { AlertSeverity } from '$lib/severity';
	import type { RuleResponse } from '$lib/server/siemApiClient';

	let {
		mode,
		initial = null,
		defaultName = '',
		defaultLogql = '',
		onClose,
		onSaved
	}: {
		mode: 'create' | 'edit';
		// The rule being edited - required (and used to seed every field) in
		// edit mode, always null in create mode.
		initial?: RuleResponse | null;
		// Create mode only - lets Search seed a new rule's name/query from
		// the current search/event context.
		defaultName?: string;
		defaultLogql?: string;
		onClose: () => void;
		// Called with the rule's id after a successful create or update.
		onSaved: (ruleId: number) => void;
	} = $props();

	const BLANK_SHAPE: RuleShape = 'threshold';
	const BLANK_WINDOW_SEC = 60;
	const BLANK_THRESHOLD = 5;
	const BLANK_GROUP_BY = '';
	const BLANK_SEVERITY: AlertSeverity = 'warning';
	const BLANK_COOLDOWN_SEC = 3600;
	const BLANK_INTERVAL_SEC = 60;

	let name = $state(initial?.name ?? defaultName);
	let logql = $state(initial?.logql ?? defaultLogql);
	let shape = $state<RuleShape>((initial?.shape as RuleShape | undefined) ?? BLANK_SHAPE);
	let windowSec = $state(initial?.window_sec ?? BLANK_WINDOW_SEC);
	let threshold = $state(initial?.threshold ?? BLANK_THRESHOLD);
	let groupBy = $state(initial?.group_by.join(', ') ?? BLANK_GROUP_BY);
	let severity = $state<AlertSeverity>(initial?.severity ?? BLANK_SEVERITY);
	let cooldownSec = $state(initial?.cooldown_sec ?? BLANK_COOLDOWN_SEC);
	let intervalSec = $state(initial?.interval_sec ?? BLANK_INTERVAL_SEC);
	let submitting = $state(false);
	let error = $state<string | null>(null);

	// Templates only make sense as a starting point for a brand-new rule -
	// applying one over an existing rule's already-tuned settings would be
	// destructive, so the picker itself is hidden in edit mode (see markup).
	function applyTemplate(event: Event) {
		const index = Number((event.target as HTMLSelectElement).value);
		if (index < 0) {
			name = defaultName;
			logql = defaultLogql;
			shape = BLANK_SHAPE;
			windowSec = BLANK_WINDOW_SEC;
			threshold = BLANK_THRESHOLD;
			groupBy = BLANK_GROUP_BY;
			severity = BLANK_SEVERITY;
			return;
		}
		const template = RULE_TEMPLATES[index];
		name = template.name;
		logql = template.logql;
		shape = template.shape;
		windowSec = template.windowSec;
		threshold = template.threshold;
		groupBy = template.groupBy;
		severity = template.severity;
	}

	async function submit(event: SubmitEvent) {
		event.preventDefault();
		submitting = true;
		error = null;
		try {
			const body = JSON.stringify({
				name,
				shape,
				logql,
				window_sec: windowSec,
				threshold,
				group_by: parseGroupBy(groupBy),
				severity,
				// destinations isn't user-editable yet (no UI for picking
				// ntfy/inapp) - preserve whatever the rule already had rather
				// than silently resetting it to just ["inapp"] on every edit.
				destinations: initial?.destinations ?? ['inapp'],
				cooldown_sec: cooldownSec,
				interval_sec: intervalSec,
				enabled: initial?.enabled ?? true
			});
			const response =
				mode === 'edit' && initial
					? await fetch(`/api/rules/${initial.id}`, {
							method: 'PUT',
							headers: { 'Content-Type': 'application/json' },
							body
						})
					: await fetch('/api/search/rules', {
							method: 'POST',
							headers: { 'Content-Type': 'application/json' },
							body
						});
			if (!response.ok) {
				error =
					response.status === 403
						? `You don't have permission to ${mode === 'edit' ? 'edit' : 'create'} rules.`
						: `Failed to ${mode === 'edit' ? 'update' : 'create'} rule.`;
				return;
			}
			const saved = (await response.json()) as { id: number };
			onSaved(saved.id);
		} catch {
			error = 'Network error — check your connection and try again.';
		} finally {
			submitting = false;
		}
	}
</script>

<div class="overlay">
	<form class="rule-form" onsubmit={submit}>
		<h2>{mode === 'edit' ? 'Edit rule' : 'Create rule'}</h2>
		{#if mode === 'create'}
			<label>
				Template
				<select onchange={applyTemplate}>
					<option value="-1">Blank / custom</option>
					{#each RULE_TEMPLATES as template, index (template.name)}
						<option value={index}>{template.label}</option>
					{/each}
				</select>
			</label>
		{/if}
		<label>
			Name
			<input bind:value={name} required />
		</label>
		<label>
			Rule type
			<select bind:value={shape}>
				<option value="threshold">threshold</option>
				<option value="absence">absence</option>
				<option value="first_seen">first_seen</option>
			</select>
		</label>
		{#if shape !== 'absence'}
			<label>
				LogQL
				<textarea bind:value={logql} required></textarea>
			</label>
		{/if}
		{#if shape !== 'absence'}
			<label>
				Window (seconds)
				<input type="number" bind:value={windowSec} min="1" />
			</label>
		{/if}
		{#if shape === 'threshold'}
			<label>
				Threshold
				<input type="number" bind:value={threshold} min="1" />
			</label>
		{/if}
		{#if shape !== 'absence'}
			<label>
				Group by (comma-separated)
				<input bind:value={groupBy} placeholder="source, host" />
			</label>
		{/if}
		<label>
			Severity
			<select bind:value={severity}>
				<option value="critical">critical</option>
				<option value="warning">warning</option>
				<option value="info">info</option>
			</select>
		</label>
		<label>
			Cooldown (seconds)
			<input type="number" bind:value={cooldownSec} min="0" />
		</label>
		<label>
			Evaluation interval (seconds)
			<input type="number" bind:value={intervalSec} min="1" />
		</label>
		{#if error}
			<p class="error">{error}</p>
		{/if}
		<div class="actions">
			<button type="button" onclick={onClose}>Cancel</button>
			<button type="submit" disabled={submitting}>
				{#if submitting}
					{mode === 'edit' ? 'Saving…' : 'Creating…'}
				{:else}
					{mode === 'edit' ? 'Save changes' : 'Create rule'}
				{/if}
			</button>
		</div>
	</form>
</div>

<style>
	.overlay {
		position: fixed;
		inset: 0;
		background: rgba(0, 0, 0, 0.5);
		display: flex;
		align-items: center;
		justify-content: center;
		z-index: 10;
	}
	.rule-form {
		background: var(--color-surface);
		border-radius: var(--radius-lg);
		box-shadow: var(--shadow-raised);
		padding: var(--space-6);
		width: 360px;
		max-height: 90vh;
		overflow-y: auto;
		display: flex;
		flex-direction: column;
		gap: var(--space-3);
	}
	h2 {
		margin: 0;
		font-size: var(--text-section-head);
	}
	label {
		display: flex;
		flex-direction: column;
		gap: var(--space-1);
		font-size: var(--text-label);
		color: var(--color-muted);
	}
	input,
	select,
	textarea {
		background: var(--color-surface-2);
		border: none;
		border-radius: var(--radius-sm);
		color: var(--color-text);
		padding: var(--space-2);
		font-size: var(--text-table);
		font-family: inherit;
	}
	textarea {
		font-family: var(--font-mono);
		font-size: var(--text-label);
		min-height: 60px;
		resize: vertical;
	}
	.error {
		color: var(--color-severity-critical);
		font-size: var(--text-label);
		margin: 0;
	}
	.actions {
		display: flex;
		justify-content: flex-end;
		gap: var(--space-2);
		margin-top: var(--space-2);
	}
	.actions button {
		border: none;
		border-radius: var(--radius-sm);
		padding: var(--space-1) var(--space-3);
		font-size: var(--text-label);
		cursor: pointer;
	}
	.actions button[type='submit'] {
		background: var(--color-accent-tint-2);
		color: var(--color-accent-lighter);
	}
	.actions button[type='button'] {
		background: var(--color-surface-2);
		color: var(--color-text);
	}
	.actions button:disabled {
		opacity: 0.6;
		cursor: default;
	}
</style>
