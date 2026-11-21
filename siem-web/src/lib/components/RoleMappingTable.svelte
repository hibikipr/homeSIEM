<script lang="ts">
	import { roleCapabilityLabel } from '#lib/settings.ts';
	import type { RoleMappingResponse } from '#lib/server/siemApiClient.ts';

	let {
		mappings,
		onEdit
	}: {
		mappings: RoleMappingResponse[];
		onEdit: (mapping: RoleMappingResponse) => void;
	} = $props();
</script>

<table class="table">
	<thead>
		<tr>
			<th>OIDC group claim</th>
			<th>Role</th>
			<th>Can</th>
			<th></th>
		</tr>
	</thead>
	<tbody>
		{#each mappings as mapping (mapping.id)}
			<tr>
				<td class="mono">{mapping.group_claim}</td>
				<td><span class="pill accent">{mapping.role}</span></td>
				<td>{roleCapabilityLabel(mapping.role)}</td>
				<td class="edit-cell">
					<button class="btn ghost" type="button" onclick={() => onEdit(mapping)}>Edit</button>
				</td>
			</tr>
		{/each}
	</tbody>
</table>

<style>
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
		font-family: var(--font-mono);
		font-size: 0.86em;
		text-transform: uppercase;
		letter-spacing: 0.1em;
		color: var(--color-muted-2);
		font-weight: 500;
	}
	.mono {
		font-family: var(--font-mono);
	}
	.pill {
		display: inline-flex;
		align-items: center;
		gap: 4px;
		font-size: var(--text-eyebrow);
		padding: 2px 6px;
		border-radius: var(--radius-pill);
	}
	.pill.accent {
		background: var(--row-selected-bg);
		color: var(--color-severity-info-text);
	}
	.edit-cell {
		text-align: right;
	}
	.btn {
		border: 1px solid transparent;
		border-radius: var(--radius-default);
		padding: 5px 11px;
		font-family: var(--font-ui);
		font-size: var(--text-table);
		cursor: pointer;
		transition:
			color 0.2s,
			border-color 0.2s;
	}
	.btn.ghost {
		background: transparent;
		color: var(--color-text-2);
		border-color: var(--color-line-2);
		padding: 2px 7px;
	}
	.btn.ghost:hover {
		border-color: var(--saber-blue);
		color: var(--color-text);
	}
</style>
