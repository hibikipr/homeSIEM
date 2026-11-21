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
			<a href={resolve('auth/login')}>sign in with SSO</a>
			instead.
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
		border: 1px solid var(--color-line);
		border-radius: var(--radius-lg);
		box-shadow: var(--shadow-overlay);
		padding: var(--space-6);
		width: 320px;
		display: flex;
		flex-direction: column;
		gap: var(--space-3);
	}
	h1 {
		margin: 0;
		font-family: var(--font-display);
		font-size: var(--text-page-title);
		font-weight: 400;
		letter-spacing: 0.06em;
		color: var(--color-gold);
		text-shadow: var(--glow-gold-text);
	}
	.hint {
		margin: 0;
		font-size: var(--text-label);
		color: var(--color-muted);
	}
	.hint a {
		color: var(--saber-blue);
		transition: color 0.2s;
	}
	.hint a:hover {
		color: var(--color-gold);
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
		border: 1px solid var(--color-line-2);
		border-radius: var(--radius-sm);
		color: var(--color-text);
		padding: var(--space-2);
		font-size: var(--text-table);
		font-family: inherit;
	}
	.error {
		color: var(--saber-red);
		font-size: var(--text-label);
		margin: 0;
	}
	button {
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
		padding: var(--space-2) var(--space-3);
		font-size: var(--text-table);
		margin-top: var(--space-2);
	}
	button:hover:not(:disabled) {
		background: var(--color-btn-primary-hover);
		border-color: var(--color-btn-primary-hover);
		box-shadow: var(--glow-primary-hover);
	}
	button:disabled {
		opacity: 0.6;
		cursor: default;
	}
</style>
