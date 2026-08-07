<script lang="ts">
	import { roleCapabilityLabel } from '$lib/settings';
	import type { RoleMappingResponse } from '$lib/server/siemApiClient';

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
		color: var(--color-muted);
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
		border-radius: 999px;
	}
	.pill.accent {
		background: var(--color-accent-tint);
		color: var(--color-accent-lighter);
	}
	.edit-cell {
		text-align: right;
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
		padding: 2px 7px;
	}
</style>
