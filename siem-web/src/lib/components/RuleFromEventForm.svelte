<script lang="ts">
	let {
		defaultName,
		defaultLogql,
		onClose
	}: {
		defaultName: string;
		defaultLogql: string;
		onClose: () => void;
	} = $props();

	let name = $state(defaultName);
	let logql = $state(defaultLogql);
	let windowSec = $state(60);
	let threshold = $state(5);
	let severity = $state('warning');
	let submitting = $state(false);
	let error = $state<string | null>(null);

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
					shape: 'threshold',
					logql,
					window_sec: windowSec,
					threshold,
					group_by: [],
					severity,
					destinations: ['inapp'],
					cooldown_sec: 3600,
					interval_sec: 60,
					enabled: true
				})
			});
			if (!response.ok) {
				error = 'Failed to create rule.';
				return;
			}
			onClose();
		} finally {
			submitting = false;
		}
	}
</script>

<div class="overlay">
	<form class="rule-form" onsubmit={submit}>
		<h2>Create rule</h2>
		<label>
			Name
			<input bind:value={name} required />
		</label>
		<label>
			LogQL
			<textarea bind:value={logql} required></textarea>
		</label>
		<label>
			Window (seconds)
			<input type="number" bind:value={windowSec} min="1" />
		</label>
		<label>
			Threshold
			<input type="number" bind:value={threshold} min="1" />
		</label>
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
