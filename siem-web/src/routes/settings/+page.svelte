<script lang="ts">
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

	const roleMappings = [
		{ group: 'homesiem-admins', role: 'admin', can: 'read/write/manage', members: '12' },
		{ group: 'homesiem-analysts', role: 'analyst', can: 'read/search/triage', members: '24' },
		{ group: 'homesiem-viewers', role: 'viewer', can: 'read only', members: '8' }
	];

	let oidc = $state({
		issuerUrl: 'https://id.home.arpa',
		clientId: 'homesiem',
		clientSecret: 'passkeysecret',
		scopes: 'openid profile email groups',
		redirectUri: 'https://siem.home.arpa/auth/callback'
	});

	function selectSection(key: SectionKey) {
		selectedSection = key;
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
					<span class="panel-title">OIDC provider</span>
					<span class="pill accent">PocketID</span>
					<span class="status">
						<span class="dot"></span>
						discovery OK · verified 2m ago
					</span>
				</div>

				<div class="form-grid">
					<label class="field">
						<span>Issuer URL</span>
						<input bind:value={oidc.issuerUrl} type="text" />
					</label>
					<label class="field">
						<span>Client ID</span>
						<input bind:value={oidc.clientId} type="text" />
					</label>
					<label class="field">
						<span>Client secret</span>
						<input bind:value={oidc.clientSecret} type="password" />
					</label>
					<label class="field">
						<span>Scopes</span>
						<input bind:value={oidc.scopes} type="text" />
					</label>
					<label class="field full-width">
						<span>Redirect URI — paste this into PocketID</span>
						<input bind:value={oidc.redirectUri} type="text" />
					</label>
				</div>

				<div class="actions">
					<button class="btn primary" type="button">Test connection</button>
					<button class="btn secondary" type="button">Save</button>
					<span>Sign-in screen shows a single <em>Continue with PocketID</em> button.</span>
				</div>
			</div>

			<div class="panel">
				<div class="panel-head">
					<span class="panel-title">Group → role mapping</span>
					<span class="muted">first match wins; users in no listed group are denied</span>
					<button class="btn ghost" type="button">+ Add mapping</button>
				</div>

				<table class="table">
					<thead>
						<tr>
							<th>OIDC group claim</th>
							<th>Role</th>
							<th>Can</th>
							<th>Members</th>
						</tr>
					</thead>
					<tbody>
						{#each roleMappings as mapping (mapping.group)}
							<tr>
								<td class="mono">{mapping.group}</td>
								<td><span class="pill accent">{mapping.role}</span></td>
								<td>{mapping.can}</td>
								<td class="mono">{mapping.members}</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>

			<div class="panel">
				<div class="panel-head">
					<span class="panel-title">Session &amp; break-glass</span>
				</div>
				<div class="cards">
					<div class="card">
						<div class="card-label">Session lifetime</div>
						<div class="card-value">12 hours</div>
					</div>
					<div class="card">
						<div class="card-label">Local admin</div>
						<div class="card-value">enabled · LAN only</div>
					</div>
					<div class="card">
						<div class="card-label">Audit log</div>
						<div class="card-value">on · 365d</div>
					</div>
				</div>
			</div>
		{:else}
			<div class="hero">
				<h1>{sections.find((section) => section.key === selectedSection)?.label}</h1>
				<p>This section is ready for the next set of settings content.</p>
			</div>
		{/if}
	</section>
</main>

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

	.status,
	.muted {
		font-size: var(--text-label);
		color: var(--color-muted);
	}

	.dot {
		display: inline-block;
		width: 6px;
		height: 6px;
		border-radius: 50%;
		background: var(--color-severity-ok);
		margin-right: 4px;
	}

	.pill {
		display: inline-flex;
		align-items: center;
		gap: 4px;
		font-size: var(--text-eyebrow);
		padding: 2px 6px;
		border-radius: 999px;
	}

	.pill.accent {
		background: var(--color-accent-tint);
		color: var(--color-accent-lighter);
	}

	.form-grid {
		display: grid;
		grid-template-columns: repeat(2, minmax(0, 1fr));
		gap: var(--space-4);
	}

	.field {
		display: flex;
		flex-direction: column;
		gap: 4px;
		font-size: var(--text-label);
		color: var(--color-muted);
	}

	.field.full-width {
		grid-column: 1 / -1;
	}

	.field input {
		background: var(--color-surface);
		border: 1px solid var(--color-line-2);
		border-radius: var(--radius-sm);
		padding: 8px 10px;
		color: var(--color-text);
		font-size: var(--text-table);
		font-family: var(--font-mono);
	}

	.actions {
		display: flex;
		align-items: center;
		gap: var(--space-3);
		flex-wrap: wrap;
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

	.btn.primary {
		background: var(--color-accent);
		color: var(--color-bg);
	}

	.btn.secondary {
		background: var(--color-surface);
		color: var(--color-text);
		border-color: var(--color-line-2);
	}

	.btn.ghost {
		background: transparent;
		color: var(--color-accent-light);
		border-color: transparent;
		margin-left: auto;
		padding: 2px 7px;
	}

	.table {
		width: 100%;
		border-collapse: collapse;
		font-size: var(--text-table);
	}

	.table th,
	.table td {
		text-align: left;
		padding: 8px 0;
		border-bottom: 1px solid var(--color-line);
	}

	.table th {
		color: var(--color-muted);
		font-weight: 500;
	}

	.mono {
		font-family: var(--font-mono);
	}

	.cards {
		display: grid;
		grid-template-columns: repeat(3, minmax(0, 1fr));
		gap: var(--space-4);
	}

	.card {
		display: flex;
		flex-direction: column;
		gap: 2.8px;
		padding: var(--space-4);
		border-radius: var(--radius-default);
		background: var(--color-surface);
		border: 1px solid var(--color-line-2);
	}

	.card-label {
		font-size: var(--text-label);
		color: var(--color-muted);
	}

	.card-value {
		font-size: 15px;
		color: var(--color-text);
	}
</style>
