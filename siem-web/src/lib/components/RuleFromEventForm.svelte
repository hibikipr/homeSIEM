<script lang="ts">
	import { RULE_TEMPLATES, parseGroupBy, type RuleShape } from '$lib/ruleTemplates';
	import type { AlertSeverity } from '$lib/severity';

	let {
		defaultName,
		defaultLogql,
		onClose
	}: {
		defaultName: string;
		defaultLogql: string;
		onClose: () => void;
	} = $props();

	const BLANK_SHAPE: RuleShape = 'threshold';
	const BLANK_WINDOW_SEC = 60;
	const BLANK_THRESHOLD = 5;
	const BLANK_GROUP_BY = '';
	const BLANK_SEVERITY: AlertSeverity = 'warning';

	let name = $state(defaultName);
	let logql = $state(defaultLogql);
	let shape = $state<RuleShape>(BLANK_SHAPE);
	let windowSec = $state(BLANK_WINDOW_SEC);
	let threshold = $state(BLANK_THRESHOLD);
	let groupBy = $state(BLANK_GROUP_BY);
	let severity = $state<AlertSeverity>(BLANK_SEVERITY);
	let submitting = $state(false);
	let error = $state<string | null>(null);

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
			const response = await fetch('/api/search/rules', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({
					name,
					shape,
					logql,
					window_sec: windowSec,
					threshold,
					group_by: parseGroupBy(groupBy),
					severity,
					destinations: ['inapp'],
					cooldown_sec: 3600,
					interval_sec: 60,
					enabled: true
				})
			});
			if (!response.ok) {
				error =
					response.status === 403
						? "You don't have permission to create rules."
						: 'Failed to create rule.';
				return;
			}
			onClose();
		} catch {
			error = 'Network error — check your connection and try again.';
		} finally {
			submitting = false;
		}
	}
</script>

<div class="overlay">
	<form class="rule-form" onsubmit={submit}>
		<h2>Create rule</h2>
		<label>
			Template
			<select onchange={applyTemplate}>
				<option value="-1">Blank / custom</option>
				{#each RULE_TEMPLATES as template, index (template.name)}
					<option value={index}>{template.label}</option>
				{/each}
			</select>
		</label>
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
		{#if error}
			<p class="error">{error}</p>
		{/if}
		<div class="actions">
			<button type="button" onclick={onClose}>Cancel</button>
			<button type="submit" disabled={submitting}>
				{submitting ? 'Creating…' : 'Create rule'}
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
