<script lang="ts">
	import RoleMappingTable from '$lib/components/RoleMappingTable.svelte';
	import RoleMappingForm from '$lib/components/RoleMappingForm.svelte';
	import type { RoleMappingResponse } from '$lib/server/siemApiClient';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	type SectionKey =
		'authentication' | 'retention' | 'notifications' | 'parsers' | 'backups' | 'about';

	let selectedSection = $state<SectionKey>('authentication');

	const sections: { key: SectionKey; label: string }[] = [
		{ key: 'authentication', label: 'Authentication' },
		{ key: 'retention', label: 'Retention & storage' },
		{ key: 'notifications', label: 'Notifications' },
		{ key: 'parsers', label: 'Parsers' },
		{ key: 'backups', label: 'Backups' },
		{ key: 'about', label: 'About' }
	];

	function selectSection(key: SectionKey) {
		selectedSection = key;
	}

	let formMode = $state<'add' | 'edit' | null>(null);
	let formSeed = $state<RoleMappingResponse | null>(null);

	function openAddForm() {
		formSeed = null;
		formMode = 'add';
	}

	function openEditForm(mapping: RoleMappingResponse) {
		formSeed = mapping;
		formMode = 'edit';
	}

	function closeForm() {
		formMode = null;
		formSeed = null;
	}

	let minSeverity = $state<'info' | 'warning' | 'critical'>(
		(data.notificationSettings.min_severity as 'info' | 'warning' | 'critical') ?? 'info'
	);
	let savingSeverity = $state(false);
	let severitySaveError = $state<string | null>(null);
	let testSending = $state(false);
	let testResult = $state<'success' | 'error' | null>(null);

	async function saveMinSeverity() {
		savingSeverity = true;
		severitySaveError = null;
		try {
			const res = await fetch('/api/settings/notifications', {
				method: 'PUT',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ min_severity: minSeverity })
			});
			if (!res.ok) throw new Error('save failed');
		} catch {
			severitySaveError = 'Could not save — try again.';
		} finally {
			savingSeverity = false;
		}
	}

	async function sendTestNotification() {
		testSending = true;
		testResult = null;
		try {
			const res = await fetch('/api/settings/notifications/test', { method: 'POST' });
			testResult = res.ok ? 'success' : 'error';
		} catch {
			testResult = 'error';
		} finally {
			testSending = false;
		}
	}
</script>

<main class="settings-shell">
	<nav class="sidebar" aria-label="Settings sections">
		{#each sections as section (section.key)}
			<button
				type="button"
				class:selected={selectedSection === section.key}
				onclick={() => selectSection(section.key)}
			>
				{section.label}
			</button>
		{/each}
	</nav>

	<section class="content">
		{#if selectedSection === 'authentication'}
			<div class="hero">
				<h1>Authentication</h1>
				<p>
					homeSIEM delegates identity to your OIDC provider. Local accounts stay available as a
					break-glass path only.
				</p>
			</div>

			<div class="panel">
				<div class="panel-head">
					<span class="panel-title">Group → role mapping</span>
					<span class="muted">first match wins; users in no listed group are denied</span>
					<button class="btn ghost" type="button" onclick={openAddForm}>+ Add mapping</button>
				</div>

				<RoleMappingTable mappings={data.roleMappings} onEdit={openEditForm} />
			</div>
		{:else if selectedSection === 'notifications'}
			<div class="hero">
				<h1>Notifications</h1>
				<p>
					homeSIEM pushes new alerts through ntfy. The server URL and topic are set at deploy time;
					this page controls how loud it is.
				</p>
			</div>

			<div class="panel">
				<div class="panel-head">
					<span class="panel-title">ntfy status</span>
				</div>
				<p class="status-line">
					{#if data.notificationSettings.ntfy_configured}
						<span class="ok">Configured</span> — NTFY_URL and NTFY_TOPIC are set.
					{:else}
						<span class="warn">Not configured</span> — set NTFY_URL and NTFY_TOPIC to enable notifications.
					{/if}
				</p>
				<button
					class="btn ghost"
					type="button"
					disabled={!data.notificationSettings.ntfy_configured || testSending}
					onclick={sendTestNotification}
				>
					{testSending ? 'Sending…' : 'Send test notification'}
				</button>
				{#if testResult === 'success'}
					<p class="status-line ok">Test notification sent.</p>
				{:else if testResult === 'error'}
					<p class="status-line warn">Could not send the test notification.</p>
				{/if}
			</div>

			<div class="panel">
				<div class="panel-head">
					<span class="panel-title">Minimum severity to notify</span>
					<span class="muted">alerts below this severity still appear in-app, just don't push</span>
				</div>
				<select bind:value={minSeverity} onchange={saveMinSeverity}>
					<option value="info">info — notify on everything</option>
					<option value="warning">warning — skip info-level alerts</option>
					<option value="critical">critical — only the most severe</option>
				</select>
				{#if savingSeverity}
					<span class="muted">Saving…</span>
				{:else if severitySaveError}
					<span class="status-line warn">{severitySaveError}</span>
				{/if}
			</div>
		{:else}
			<div class="hero">
				<h1>{sections.find((section) => section.key === selectedSection)?.label}</h1>
				<p>This section is ready for the next set of settings content.</p>
			</div>
		{/if}
	</section>
</main>

{#if formMode}
	<RoleMappingForm
		mode={formMode}
		initial={formSeed}
		existingMappings={data.roleMappings}
		onClose={closeForm}
	/>
{/if}

<style>
	.settings-shell {
		display: flex;
		gap: var(--space-6);
		padding: var(--space-5) var(--space-6);
		min-height: 0;
	}

	.sidebar {
		width: 168px;
		flex: none;
		display: flex;
		flex-direction: column;
		gap: 2.8px;
	}

	.sidebar button {
		border: 0;
		background: transparent;
		color: var(--color-muted);
		text-align: left;
		padding: 6px 8.4px;
		border-radius: var(--radius-sm);
		cursor: pointer;
		font-size: var(--text-table);
	}

	.sidebar button.selected {
		background: var(--color-surface);
		color: var(--color-text);
		box-shadow: inset 2px 0 0 var(--color-accent);
	}

	.content {
		flex: 1;
		min-width: 0;
		max-width: 840px;
		display: flex;
		flex-direction: column;
		gap: var(--space-6);
	}

	.hero h1 {
		font-size: var(--text-page-title);
		margin: 0 0 4px;
	}

	.hero p {
		margin: 0;
		font-size: var(--text-body);
		color: var(--color-text-3);
		max-width: 70ch;
		line-height: 1.6;
	}

	.panel {
		display: flex;
		flex-direction: column;
		gap: var(--space-4);
		padding: var(--space-4);
		border-radius: var(--radius-default);
		background: var(--color-surface-2);
		box-shadow: var(--shadow-flat);
	}

	.panel-head {
		display: flex;
		align-items: center;
		gap: var(--space-3);
		flex-wrap: wrap;
	}

	.panel-title {
		font-size: var(--text-section-head);
		font-weight: 500;
	}

	.muted {
		font-size: var(--text-label);
		color: var(--color-muted);
	}

	.btn {
		border: 1px solid transparent;
		border-radius: var(--radius-sm);
		padding: 5px 11px;
		font-size: var(--text-table);
		cursor: pointer;
	}

	.btn.ghost {
		background: transparent;
		color: var(--color-accent-light);
		border-color: transparent;
		margin-left: auto;
		padding: 2px 7px;
	}

	.status-line {
		font-size: var(--text-table);
		margin: 0;
	}
	.status-line .ok,
	.status-line.ok {
		color: var(--color-severity-healthy);
	}
	.status-line .warn,
	.status-line.warn {
		color: var(--color-severity-warning);
	}
	select {
		background: var(--color-surface-3);
		color: var(--color-text);
		border: 1px solid var(--color-line-2);
		border-radius: var(--radius-sm);
		padding: 4px 8px;
		font-size: var(--text-table);
	}
</style>
