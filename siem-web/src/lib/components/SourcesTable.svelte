<script lang="ts">
	import { invalidateAll } from '$app/navigation';
	import { resolve } from '$app/paths';
	import type { SourceResponse } from '$lib/server/siemApiClient';
	import { formatEventsPerMin, formatLastSeen } from '$lib/sources';

	let {
		sources,
		selectedName,
		canRename = false
	}: { sources: SourceResponse[]; selectedName: string | null; canRename?: boolean } = $props();

	let renamingId = $state<number | null>(null);
	let renameValue = $state('');
	let saving = $state(false);

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
				await invalidateAll();
			}
		} finally {
			saving = false;
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
							href={resolve(`/sources?preview=${encodeURIComponent(source.name)}`)}
							>{source.display_name || source.name}</a
						>
						{#if canRename}
							<button class="rename-trigger" onclick={() => startRename(source)}>Rename</button>
						{/if}
					{/if}
				</td>
				<td class="mono">{source.address}</td>
				<td class="mono">{source.transport}</td>
				<td><span class="tag">{source.parser}</span></td>
				<td class="num mono">{formatEventsPerMin(source.events_per_min)}</td>
				<td class="mono">{formatLastSeen(source.last_seen_at)}</td>
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
		font-size: var(--text-eyebrow);
		text-transform: uppercase;
		letter-spacing: 0.06em;
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
		font-weight: 500;
	}
	.name-link:hover {
		color: var(--color-accent-light);
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
		color: var(--color-accent-light);
	}
	.rename-row {
		display: flex;
		align-items: center;
		gap: var(--space-1);
	}
	.rename-input {
		font-size: var(--text-table);
		background: var(--color-surface-3);
		border: 1px solid var(--color-line);
		border-radius: var(--radius-sm);
		color: var(--color-text);
		padding: 2px var(--space-2);
		min-width: 0;
		flex: 1 1 auto;
	}
	.rename-action {
		background: var(--color-accent-tint-2);
		color: var(--color-accent-lighter);
		border: none;
		border-radius: var(--radius-sm);
		padding: 2px var(--space-2);
		font-size: var(--text-label);
		cursor: pointer;
		flex-shrink: 0;
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
		border-radius: var(--radius-sm);
		padding: 1px var(--space-2);
		color: var(--color-muted);
	}
	.health {
		display: inline-flex;
		align-items: center;
		gap: var(--space-1);
		text-transform: capitalize;
	}
	.dot {
		width: 7px;
		height: 7px;
		border-radius: 50%;
		background: currentColor;
	}
	.health-healthy {
		color: var(--color-severity-healthy);
	}
	.health-silent {
		color: var(--color-severity-warning);
	}
</style>
