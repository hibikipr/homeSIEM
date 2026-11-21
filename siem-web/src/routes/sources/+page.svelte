<script lang="ts">
	import SourcesTable from '#lib/components/SourcesTable.svelte';
	import ParserPreview from '#lib/components/ParserPreview.svelte';
	import IngestHealthPanel from '#lib/components/IngestHealthPanel.svelte';
	import UnclaimedSenders from '#lib/components/UnclaimedSenders.svelte';
	import Skeleton from '#lib/components/Skeleton.svelte';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();
</script>

<div class="sources-screen">
	<div class="main">
		{#await data.registry}
			<div class="loading" aria-busy="true">
				<Skeleton height="1.4rem" />
				<Skeleton height="1.4rem" />
				<Skeleton height="1.4rem" />
				<Skeleton height="1.4rem" />
			</div>
		{:then registry}
			{#if registry}
				<SourcesTable
					sources={registry.sources}
					selectedName={registry.previewName}
					canRename={data.userRole === 'admin'}
				/>
				{#await data.previewSamples}
					<ParserPreview sourceName={registry.previewName} samples={[]} />
				{:then samples}
					<ParserPreview sourceName={registry.previewName} {samples} />
				{/await}
			{:else}
				<p class="load-error">Couldn't load sources.</p>
			{/if}
		{/await}
	</div>
	<aside class="rail">
		<section class="panel">
			<h2>Point a device here</h2>
			<p class="body">
				UniFi: <strong>Settings → System → Advanced → Remote Logging</strong>. Point it at:
			</p>
			<dl class="mono">
				<div class="row">
					<dt>Syslog (UDP)</dt>
					<dd>514</dd>
				</div>
				<div class="row">
					<dt>Syslog (TCP)</dt>
					<dd>601</dd>
				</div>
				<div class="row">
					<dt>Syslog (TLS)</dt>
					<dd>6514</dd>
				</div>
			</dl>
		</section>
		{#await data.health}
			<section class="panel"><Skeleton height="4rem" /></section>
		{:then health}
			<IngestHealthPanel {health} />
		{/await}
		{#await data.registry}
			<section class="panel"><Skeleton height="3rem" /></section>
		{:then registry}
			{#if registry}
				<UnclaimedSenders
					sources={registry.unclaimedSources}
					canClaim={data.userRole === 'admin'}
				/>
			{/if}
		{/await}
	</aside>
</div>

<style>
	.sources-screen {
		display: flex;
		gap: var(--space-6);
		padding: var(--space-5) var(--space-6);
		align-items: flex-start;
	}
	.main {
		flex: 1 1 auto;
		min-width: 0;
	}
	.rail {
		flex: 0 0 260px;
		display: flex;
		flex-direction: column;
		gap: var(--space-4);
	}

	@media (max-width: 768px) {
		.sources-screen {
			flex-direction: column;
			padding: var(--space-5);
		}
		.rail {
			flex: none;
			width: 100%;
		}
	}
	.loading {
		display: flex;
		flex-direction: column;
		gap: var(--space-3);
	}
	.load-error {
		margin: 0;
		color: var(--color-muted);
	}
	.panel {
		background: var(--color-surface);
		border: 1px solid var(--color-line);
		border-radius: var(--radius-default);
		padding: var(--space-4);
	}
	h2 {
		font-size: var(--text-section-head);
		font-weight: 600;
		letter-spacing: 0.02em;
		color: var(--color-muted);
		margin: 0 0 var(--space-3);
	}
	.body {
		font-size: var(--text-table);
		color: var(--color-text-3);
		margin: 0 0 var(--space-3);
	}
	dl.mono {
		margin: 0;
	}
	.row {
		display: flex;
		justify-content: space-between;
		font-family: var(--font-mono);
		font-size: var(--text-table);
		padding: var(--space-1) 0;
		color: var(--color-text-2);
	}
	.row dt {
		color: var(--color-muted);
	}
</style>
