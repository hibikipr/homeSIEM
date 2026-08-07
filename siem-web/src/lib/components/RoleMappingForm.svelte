<script lang="ts">
	import { invalidateAll } from '$app/navigation';
	import type { RoleMappingResponse } from '$lib/server/siemApiClient';

	let {
		mode,
		initial,
		existingMappings,
		onClose
	}: {
		mode: 'add' | 'edit';
		initial: RoleMappingResponse | null;
		existingMappings: RoleMappingResponse[];
		onClose: () => void;
	} = $props();

	let groupClaim = $state(initial?.group_claim ?? '');
	let role = $state(initial?.role ?? 'viewer');
	let submitting = $state(false);
	let error = $state<string | null>(null);

	function nextPriority(): number {
		if (existingMappings.length === 0) return 1;
		return Math.max(...existingMappings.map((m) => m.priority)) + 1;
	}

	async function submit(event: SubmitEvent) {
		event.preventDefault();
		submitting = true;
		error = null;
		try {
			const priority = mode === 'edit' && initial ? initial.priority : nextPriority();
			const response = await fetch('/api/settings/auth', {
				method: 'PUT',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({
					role_mappings: [{ group_claim: groupClaim, role, priority }]
				})
			});
			if (!response.ok) {
				error = 'Failed to save role mapping.';
				return;
			}
			await invalidateAll();
			onClose();
		} finally {
			submitting = false;
		}
	}
</script>

<div class="overlay">
	<form class="mapping-form" onsubmit={submit}>
		<h2>{mode === 'add' ? 'Add mapping' : 'Edit mapping'}</h2>
		<label>
			OIDC group claim
			<input bind:value={groupClaim} required disabled={mode === 'edit'} />
		</label>
		<label>
			Role
			<select bind:value={role}>
				<option value="viewer">viewer</option>
				<option value="analyst">analyst</option>
				<option value="admin">admin</option>
			</select>
		</label>
		{#if error}
			<p class="error">{error}</p>
		{/if}
		<div class="actions">
			<button type="button" onclick={onClose}>Cancel</button>
			<button type="submit" disabled={submitting}>
				{submitting ? 'Saving…' : 'Save'}
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
	.mapping-form {
		background: var(--color-surface);
		border-radius: var(--radius-lg);
		box-shadow: var(--shadow-raised);
		padding: var(--space-6);
		width: 340px;
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
	select {
		background: var(--color-surface-2);
		border: none;
		border-radius: var(--radius-sm);
		color: var(--color-text);
		padding: var(--space-2);
		font-size: var(--text-table);
		font-family: inherit;
	}
	input:disabled {
		opacity: 0.6;
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
