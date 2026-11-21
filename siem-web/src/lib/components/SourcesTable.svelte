<script lang="ts">
	import { invalidate } from '$app/navigation';
	import { DEP_SOURCES } from '#lib/invalidation.ts';
	import { resolve } from '$app/paths';
	import type { SourceResponse } from '#lib/server/siemApiClient.ts';
	import { formatEventsPerMin, formatLastSeen } from '#lib/sources.ts';
	import { formatSecondsAsMinutes } from '#lib/minutePresets.ts';

	let {
		sources,
		selectedName,
		canRename = false
	}: { sources: SourceResponse[]; selectedName: string | null; canRename?: boolean } = $props();

	let renamingId = $state<number | null>(null);
	let renameValue = $state('');
	let saving = $state(false);
	let deletingId = $state<number | null>(null);

	let editingHeartbeatId = $state<number | null>(null);
	let heartbeatMinutesValue = $state(1);
	let savingHeartbeat = $state(false);

	function startRename(source: SourceResponse) {
		renamingId = source.id;
		renameValue = source.display_name;
	}

	function cancelRename() {
		renamingId = null;
	}

	async function saveRename(id: number) {
		saving = true;
		try {
			const response = await fetch(`/api/sources/${id}/rename`, {
				method: 'PUT',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ display_name: renameValue.trim() })
			});
			if (response.ok) {
				renamingId = null;
				await invalidate(DEP_SOURCES);
			}
		} finally {
			saving = false;
		}
	}

	function startEditHeartbeat(source: SourceResponse) {
		editingHeartbeatId = source.id;
		heartbeatMinutesValue = source.heartbeat_sec / 60;
	}

	function cancelEditHeartbeat() {
		editingHeartbeatId = null;
	}

	async function saveHeartbeat(id: number) {
		savingHeartbeat = true;
		try {
			const response = await fetch(`/api/sources/${id}/heartbeat`, {
				method: 'PUT',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ heartbeat_sec: Math.round(heartbeatMinutesValue * 60) })
			});
			if (response.ok) {
				editingHeartbeatId = null;
				await invalidate(DEP_SOURCES);
			}
		} finally {
			savingHeartbeat = false;
		}
	}

	// A source that's still actively sending logs re-creates itself,
	// unclaimed, on its next heartbeat - forgetting it is a cleanup action
	// for silent/decommissioned devices, not a permanent ban. Warn when
	// deleting one that's currently healthy, since it'll likely come right
	// back.
	async function forgetSource(source: SourceResponse) {
		const label = source.display_name || source.name;
		const message =
			source.status === 'healthy'
				? `"${label}" is still actively sending logs - it'll likely reappear (unclaimed, unrenamed) the next time it logs. Forget it anyway?`
				: `Forget "${label}"? This removes it from the list; if it starts logging again, it'll come back as a new, unclaimed source.`;
		if (!confirm(message)) return;

		deletingId = source.id;
		try {
			const response = await fetch(`/api/sources/${source.id}`, { method: 'DELETE' });
			if (response.ok) {
				await invalidate(DEP_SOURCES);
			}
		} finally {
			deletingId = null;
		}
	}
</script>

<table class="sources">
	<thead>
		<tr>
			<th>Source</th>
			<th>Address</th>
			<th>Transport</th>
			<th>Parser</th>
			<th class="num">Events/min</th>
			<th>Last seen</th>
			<th>Heartbeat</th>
			<th>Health</th>
		</tr>
	</thead>
	<tbody>
		{#each sources as source (source.id)}
			<tr class:selected={source.name === selectedName}>
				<td>
					{#if renamingId === source.id}
						<div class="rename-row">
							<input
								class="rename-input"
								type="text"
								bind:value={renameValue}
								placeholder={source.name}
								disabled={saving}
								onkeydown={(e) => {
									if (e.key === 'Enter') saveRename(source.id);
									if (e.key === 'Escape') cancelRename();
								}}
							/>
							<button class="rename-action" onclick={() => saveRename(source.id)} disabled={saving}>
								{saving ? '…' : 'Save'}
							</button>
							<button class="rename-action" onclick={cancelRename} disabled={saving}>Cancel</button>
						</div>
					{:else}
						<a
							class="name-link"
							href={resolve(`sources?preview=${encodeURIComponent(source.name)}`)}
						>
							{source.display_name || source.name}
						</a>

						{#if canRename}
							<button class="rename-trigger" onclick={() => startRename(source)}>Rename</button>

							<button
								class="rename-trigger"
								onclick={() => forgetSource(source)}
								disabled={deletingId === source.id}
							>
								{deletingId === source.id ? '…' : 'Forget'}
							</button>
						{/if}
					{/if}
				</td>
				<td class="mono">{source.address}</td>
				<td class="mono">{source.transport}</td>
				<td><span class="tag">{source.parser}</span></td>
				<td class="num mono">{formatEventsPerMin(source.events_per_min)}</td>
				<td class="mono">{formatLastSeen(source.last_seen_at)}</td>
				<td>
					{#if editingHeartbeatId === source.id}
						<div class="rename-row">
							<input
								class="rename-input heartbeat-input"
								type="number"
								min="1"
								step="1"
								bind:value={heartbeatMinutesValue}
								disabled={savingHeartbeat}
								onkeydown={(e) => {
									if (e.key === 'Enter') saveHeartbeat(source.id);
									if (e.key === 'Escape') cancelEditHeartbeat();
								}}
							/>
							<span class="unit">min</span>
							<button
								class="rename-action"
								onclick={() => saveHeartbeat(source.id)}
								disabled={savingHeartbeat}
							>
								{savingHeartbeat ? '…' : 'Save'}
							</button>
							<button
								class="rename-action"
								onclick={cancelEditHeartbeat}
								disabled={savingHeartbeat}
							>
								Cancel
							</button>
						</div>
					{:else}
						<span class="mono">{formatSecondsAsMinutes(source.heartbeat_sec)}</span>
						{#if canRename}
							<button class="rename-trigger" onclick={() => startEditHeartbeat(source)}>
								Edit
							</button>
						{/if}
					{/if}
				</td>
				<td>
					<span class="health health-{source.status}">
						<span class="dot"></span>{source.status}
					</span>
				</td>
			</tr>
		{/each}
	</tbody>
</table>

<style>
	.sources {
		width: 100%;
		border-collapse: collapse;
		font-size: var(--text-table);
		line-height: var(--line-height-dense-table);
	}
	thead th {
		text-align: left;
		font-family: var(--font-mono);
		font-size: 0.86em;
		text-transform: uppercase;
		letter-spacing: 0.1em;
		color: var(--color-muted-2);
		padding: var(--space-2) var(--space-3);
		border-bottom: 1px solid var(--color-line);
	}
	tbody td {
		padding: var(--space-2) var(--space-3);
		border-bottom: 1px solid var(--color-line);
	}
	tbody tr.selected {
		background: var(--row-selected-bg);
	}
	tbody tr:hover {
		background: var(--row-hover-bg);
	}
	.num {
		text-align: right;
	}
	.mono {
		font-family: var(--font-mono);
		color: var(--color-text-3);
	}
	.name-link {
		color: var(--color-text);
		text-decoration: none;
		font-weight: 600;
		letter-spacing: 0.02em;
		transition: color 0.2s;
	}
	.name-link:hover {
		color: var(--color-gold);
	}
	.rename-trigger {
		background: none;
		border: none;
		color: var(--color-muted-2);
		font-size: var(--text-label);
		cursor: pointer;
		padding: 0;
		margin-left: var(--space-2);
	}
	.rename-trigger:hover {
		color: var(--color-gold);
	}
	.rename-row {
		display: flex;
		align-items: center;
		gap: var(--space-1);
	}
	.rename-input {
		font-size: var(--text-table);
		background: var(--color-surface-2);
		border: 1px solid var(--color-line-2);
		border-radius: var(--radius-sm);
		color: var(--color-text);
		padding: 2px var(--space-2);
		min-width: 0;
		flex: 1 1 auto;
	}
	.heartbeat-input {
		flex: 0 0 64px;
	}
	.unit {
		font-size: var(--text-label);
		color: var(--color-muted-2);
	}
	.rename-action {
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
		padding: 2px var(--space-2);
		font-size: var(--text-label);
		flex-shrink: 0;
	}
	.rename-action:hover:not(:disabled) {
		background: var(--color-btn-primary-hover);
		border-color: var(--color-btn-primary-hover);
		box-shadow: var(--glow-primary-hover);
	}

	.rename-action:disabled {
		opacity: 0.6;
		cursor: default;
	}
	.tag {
		display: inline-block;
		font-family: var(--font-mono);
		font-size: var(--text-label);
		background: var(--color-surface-3);
		border-radius: var(--radius-pill);
		padding: 1px var(--space-3);
		color: var(--color-muted);
	}
	.health {
		display: inline-flex;
		align-items: center;
		gap: var(--space-2);
		text-transform: capitalize;
		border-radius: var(--radius-pill);
		padding: 1px var(--space-3);
	}
	.dot {
		width: 7px;
		height: 7px;
		border-radius: 50%;
		background: currentColor;
	}
	.health-healthy {
		background: var(--color-status-success-bg);
		color: var(--saber-green);
	}
	.health-silent {
		background: var(--color-status-warn-bg);
		color: var(--color-gold);
	}
</style>
