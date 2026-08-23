<script lang="ts">
	import { RULE_TEMPLATES, parseGroupBy, type RuleShape } from '$lib/ruleTemplates';
	import type { AlertSeverity } from '$lib/severity';
	import type { RuleResponse } from '$lib/server/siemApiClient';
	import MinutesPicker from '$lib/components/MinutesPicker.svelte';

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

	// Minute presets shown in each dropdown - a rule whose existing value
	// (edit mode) doesn't match one of these falls through to "Custom"
	// automatically, see MinutesPicker.
	const WINDOW_PRESETS_MIN = [1, 5, 15, 30, 60, 360, 1440];
	const COOLDOWN_PRESETS_MIN = [5, 15, 30, 60, 360, 1440];
	const INTERVAL_PRESETS_MIN = [1, 5, 15, 30, 60];

	const SHAPE_DESCRIPTIONS: Record<RuleShape, string> = {
		threshold:
			'Alerts once a matching event happens at least this many times within the time window.',
		absence:
			"Alerts when a source that's normally sending logs stops - no matching events for a full heartbeat period.",
		first_seen:
			'Alerts the first time a value (e.g. a new source or host) shows up that was never seen before.',
		insight:
			"Alerts when Ollama's Insights pass finds something new at or above the severity below - lets you opt in to a phone notification for insights instead of only seeing them on the Insights tab. Only fires for a finding's first occurrence, never for a recurrence of one already surfaced."
	};

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
			<p class="hint">Starts you off with a common rule shape - every field stays editable.</p>
		{/if}
		<label>
			Name
			<input bind:value={name} required />
		</label>
		<label>
			Rule type
			<select bind:value={shape}>
				<option value="threshold">Threshold — repeated events</option>
				<option value="absence">Absence — a source goes quiet</option>
				<option value="first_seen">First seen — a new value appears</option>
				<option value="insight">Insight — Ollama finds something</option>
			</select>
		</label>
		<p class="hint">{SHAPE_DESCRIPTIONS[shape]}</p>
		{#if shape !== 'absence' && shape !== 'insight'}
			<label>
				LogQL query
				<textarea bind:value={logql} required></textarea>
			</label>
			<p class="hint">The Loki query this rule watches - same syntax as Search's query bar.</p>
		{/if}
		{#if shape === 'absence'}
			<MinutesPicker
				bind:seconds={windowSec}
				presetsMinutes={WINDOW_PRESETS_MIN}
				label="Correlation window"
				description="If multiple sources go quiet within this long of each other, raise one combined alert instead of one per source - multiple unrelated devices going silent together is usually one shared cause (the collector, a network segment, a router), not independent coincidences."
			/>
		{:else if shape !== 'insight'}
			<MinutesPicker
				bind:seconds={windowSec}
				presetsMinutes={WINDOW_PRESETS_MIN}
				label="Time window"
				description="How far back to look for matching events each time this rule runs."
			/>
		{/if}
		{#if shape === 'threshold'}
			<label>
				Threshold
				<input type="number" bind:value={threshold} min="1" />
			</label>
			<p class="hint">Alert once this many matching events happen within the time window above.</p>
		{/if}
		{#if shape !== 'absence' && shape !== 'insight'}
			<label>
				Group by (comma-separated)
				<input bind:value={groupBy} placeholder="source, host" />
			</label>
			<p class="hint">
				{shape === 'first_seen'
					? 'Which field to watch for brand-new values in - e.g. "source" catches a sender that has never logged before.'
					: 'Count matches separately per value of these fields instead of all together. Leave blank to treat every match as one group.'}
			</p>
		{/if}
		<label>
			Severity
			<select bind:value={severity}>
				<option value="critical">Critical</option>
				<option value="warning">Warning</option>
				<option value="info">Info</option>
			</select>
		</label>
		<p class="hint">
			{shape === 'insight'
				? "The minimum severity Ollama assigned an insight for it to raise an alert here - the alert itself still shows the insight's own severity, this is only a floor."
				: 'How urgent the resulting alert is.'}
		</p>
		<MinutesPicker
			bind:seconds={cooldownSec}
			presetsMinutes={COOLDOWN_PRESETS_MIN}
			label="Cooldown"
			description="Minimum time to wait before raising another alert for the same match, so you don't get repeat notifications for the same ongoing issue."
		/>
		<MinutesPicker
			bind:seconds={intervalSec}
			presetsMinutes={INTERVAL_PRESETS_MIN}
			label="Check every"
			description="How often this rule re-checks for matches. Shorter catches problems sooner but adds load - most rules don't need less than a minute."
		/>
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
		width: 380px;
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
	.hint {
		margin: calc(-1 * var(--space-2)) 0 0;
		font-size: 11px;
		color: var(--color-muted-2);
		line-height: 1.4;
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
