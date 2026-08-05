<script lang="ts">
	import { resolve } from '$app/paths';
	import type { SourceResponse } from '$lib/server/siemApiClient';
	import { formatEventsPerMin, formatLastSeen } from '$lib/sources';

	let { sources, selectedName }: { sources: SourceResponse[]; selectedName: string | null } =
		$props();
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
				<td
					><a
						class="name-link"
						href={resolve(`/sources?preview=${encodeURIComponent(source.name)}`)}>{source.name}</a
					></td
				>
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
