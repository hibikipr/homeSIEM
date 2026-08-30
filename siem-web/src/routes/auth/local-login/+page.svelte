<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';

	let username = $state('');
	let password = $state('');
	let submitting = $state(false);
	let error = $state<string | null>(null);

	async function submit(event: SubmitEvent) {
		event.preventDefault();
		submitting = true;
		error = null;
		try {
			const response = await fetch('/auth/local-login', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ username, password })
			});
			if (!response.ok) {
				const body = await response.json().catch(() => ({}));
				error = body.error ?? 'Login failed.';
				return;
			}
			await goto(resolve('/'));
		} catch {
			error = 'Network error — check your connection and try again.';
		} finally {
			submitting = false;
		}
	}
</script>

<svelte:head>
	<title>Local admin sign-in — homeSIEM</title>
</svelte:head>

<div class="wrap">
	<form class="card" onsubmit={submit}>
		<h1>Local admin sign-in</h1>
		<p class="hint">
			Break-glass access for when the OIDC provider is unreachable. If you're here by mistake,
			<a href={resolve('/auth/login')}>sign in with SSO</a> instead.
		</p>
		<label>
			Username
			<input bind:value={username} autocomplete="username" required />
		</label>
		<label>
			Password
			<input type="password" bind:value={password} autocomplete="current-password" required />
		</label>
		{#if error}
			<p class="error">{error}</p>
		{/if}
		<button type="submit" disabled={submitting}>
			{submitting ? 'Signing in…' : 'Sign in'}
		</button>
	</form>
</div>

<style>
	.wrap {
		min-height: 80vh;
		display: flex;
		align-items: center;
		justify-content: center;
		padding: var(--space-6);
	}
	.card {
		background: var(--color-surface);
		border-radius: var(--radius-lg);
		box-shadow: var(--shadow-raised);
		padding: var(--space-6);
		width: 320px;
		display: flex;
		flex-direction: column;
		gap: var(--space-3);
	}
	h1 {
		margin: 0;
		font-size: var(--text-page-title);
		font-weight: 500;
	}
	.hint {
		margin: 0;
		font-size: var(--text-label);
		color: var(--color-muted);
	}
	.hint a {
		color: var(--color-accent-light);
	}
	label {
		display: flex;
		flex-direction: column;
		gap: var(--space-1);
		font-size: var(--text-label);
		color: var(--color-muted);
	}
	input {
		background: var(--color-surface-2);
		border: none;
		border-radius: var(--radius-sm);
		color: var(--color-text);
		padding: var(--space-2);
		font-size: var(--text-table);
		font-family: inherit;
	}
	.error {
		color: var(--color-severity-critical);
		font-size: var(--text-label);
		margin: 0;
	}
	button {
		border: none;
		border-radius: var(--radius-sm);
		padding: var(--space-2) var(--space-3);
		font-size: var(--text-table);
		cursor: pointer;
		background: var(--color-accent-tint-2);
		color: var(--color-accent-lighter);
		margin-top: var(--space-2);
	}
	button:disabled {
		opacity: 0.6;
		cursor: default;
	}
</style>
