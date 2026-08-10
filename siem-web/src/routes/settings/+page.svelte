<script lang="ts">
	import RoleMappingTable from '$lib/components/RoleMappingTable.svelte';
	import RoleMappingForm from '$lib/components/RoleMappingForm.svelte';
	import type { RoleMappingResponse } from '$lib/server/siemApiClient';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	type SectionKey = 'authentication' | 'notifications' | 'ollama';

	let selectedSection = $state<SectionKey>('authentication');

	const sections: { key: SectionKey; label: string }[] = [
		{ key: 'authentication', label: 'Authentication' },
		{ key: 'notifications', label: 'Notifications' },
		{ key: 'ollama', label: 'Ollama' }
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

	let systemPrompt = $state(data.ollamaSettings.system_prompt);
	let temperature = $state(data.ollamaSettings.temperature);
	let topP = $state(data.ollamaSettings.top_p);
	let numPredict = $state(data.ollamaSettings.num_predict);
	let numCtx = $state(data.ollamaSettings.num_ctx);
	let showDefaultPrompt = $state(false);
	let savingOllama = $state(false);
	let ollamaSaveError = $state<string | null>(null);
	let ollamaSaved = $state(false);

	function resetPromptToDefault() {
		systemPrompt = '';
	}

	async function saveOllamaSettings() {
		savingOllama = true;
		ollamaSaveError = null;
		ollamaSaved = false;
		try {
			const res = await fetch('/api/settings/ollama', {
				method: 'PUT',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({
					system_prompt: systemPrompt,
					temperature,
					top_p: topP,
					num_predict: numPredict,
					num_ctx: numCtx
				})
			});
			if (!res.ok) throw new Error('save failed');
			ollamaSaved = true;
		} catch {
			ollamaSaveError = 'Could not save — try again.';
		} finally {
			savingOllama = false;
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
		{:else if selectedSection === 'ollama'}
			<div class="hero">
				<h1>Ollama</h1>
				<p>
					siem-insights reviews recent activity with a locally-hosted LLM and surfaces suggestions
					on the Wall and at Insights. The connection itself (URL, model, timeout, schedule) is set
					at deploy time; this page controls how it prompts the model.
				</p>
			</div>

			<div class="panel">
				<div class="panel-head">
					<span class="panel-title">Status</span>
				</div>
				<p class="status-line">
					{#if data.ollamaSettings.configured}
						<span class="ok">Configured</span> — model <code>{data.ollamaSettings.model}</code>,
						{data.ollamaSettings.timeout_sec}s timeout, runs every
						{Math.round(data.ollamaSettings.interval_sec / 60)}min over the last
						{data.ollamaSettings.lookback_min}min.
					{:else}
						<span class="warn">Not configured</span> — set OLLAMA_URL to enable insights.
					{/if}
				</p>
			</div>

			<div class="panel">
				<div class="panel-head">
					<span class="panel-title">System prompt</span>
					<span class="muted">what the model is told before each pass</span>
					<button class="btn ghost" type="button" onclick={resetPromptToDefault}>
						Reset to default
					</button>
				</div>
				<textarea
					bind:value={systemPrompt}
					rows="10"
					placeholder={data.ollamaSettings.default_system_prompt}></textarea>
				<p class="muted">
					Empty uses the built-in default shown below. Whatever prompt runs must still ask for the
					same JSON array shape - siem-api can't parse suggestions back out of anything else.
				</p>
				<button
					class="btn ghost"
					type="button"
					onclick={() => (showDefaultPrompt = !showDefaultPrompt)}
				>
					{showDefaultPrompt ? 'Hide' : 'Show'} built-in default
				</button>
				{#if showDefaultPrompt}
					<pre class="default-prompt">{data.ollamaSettings.default_system_prompt}</pre>
				{/if}
			</div>

			<div class="panel">
				<div class="panel-head">
					<span class="panel-title">Generation parameters</span>
				</div>
				<div class="param-grid">
					<label>
						Temperature
						<input type="number" min="0" max="2" step="0.05" bind:value={temperature} />
						<span class="muted"
							>Lower is more consistent/grounded, higher is more creative. Recommended: 0.1-0.3 for
							this analytical task - it shouldn't be inventing anything.</span
						>
					</label>
					<label>
						Top-p
						<input type="number" min="0.01" max="1" step="0.05" bind:value={topP} />
						<span class="muted"
							>Nucleus sampling cutoff. 0.9 pairs well with a low temperature; rarely worth changing
							on its own.</span
						>
					</label>
					<label>
						Max response tokens (num_predict)
						<input type="number" min="1" max="8192" step="1" bind:value={numPredict} />
						<span class="muted"
							>Caps how long a response can run, independent of the HTTP timeout. 1024 comfortably
							fits a handful of suggestions in the expected JSON shape.</span
						>
					</label>
					<label>
						Context window (num_ctx)
						<input type="number" min="256" max="262144" step="256" bind:value={numCtx} />
						<span class="muted"
							>Ollama's own per-model default is often only 2048-4096, which can silently truncate
							the alerts/rollup/samples data in the prompt. 8192 is a safer floor; raise it if your
							model supports more and lookback/sample volume is large.</span
						>
					</label>
				</div>
			</div>

			<div class="panel">
				<button
					class="btn primary"
					type="button"
					disabled={savingOllama}
					onclick={saveOllamaSettings}
				>
					{savingOllama ? 'Saving…' : 'Save'}
				</button>
				{#if ollamaSaved}
					<span class="status-line ok">Saved.</span>
				{:else if ollamaSaveError}
					<span class="status-line warn">{ollamaSaveError}</span>
				{/if}
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

	.btn.primary {
		background: var(--color-accent);
		color: var(--color-on-accent, #fff);
		align-self: flex-start;
	}

	.btn.primary:disabled {
		opacity: 0.6;
		cursor: default;
	}

	textarea {
		background: var(--color-surface-3);
		color: var(--color-text);
		border: 1px solid var(--color-line-2);
		border-radius: var(--radius-sm);
		padding: 8px 10px;
		font-size: var(--text-table);
		font-family: inherit;
		resize: vertical;
	}

	.default-prompt {
		background: var(--color-surface-3);
		border: 1px solid var(--color-line-2);
		border-radius: var(--radius-sm);
		padding: 8px 10px;
		font-size: var(--text-label);
		color: var(--color-text-3);
		white-space: pre-wrap;
		margin: 0;
		max-height: 240px;
		overflow-y: auto;
	}

	.param-grid {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(240px, 1fr));
		gap: var(--space-4);
	}

	.param-grid label {
		display: flex;
		flex-direction: column;
		gap: 4px;
		font-size: var(--text-table);
	}

	.param-grid input[type='number'] {
		background: var(--color-surface-3);
		color: var(--color-text);
		border: 1px solid var(--color-line-2);
		border-radius: var(--radius-sm);
		padding: 4px 8px;
		font-size: var(--text-table);
		width: 100%;
		max-width: 160px;
	}

	.status-line code {
		background: var(--color-surface-3);
		border-radius: var(--radius-sm);
		padding: 1px 5px;
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
