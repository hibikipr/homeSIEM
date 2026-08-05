<script lang="ts">
	import SourcesTable from '$lib/components/SourcesTable.svelte';
	import ParserPreview from '$lib/components/ParserPreview.svelte';
	import IngestHealthPanel from '$lib/components/IngestHealthPanel.svelte';
	import UnclaimedSenders from '$lib/components/UnclaimedSenders.svelte';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();
</script>

<div class="sources-screen">
	<div class="main">
		<SourcesTable sources={data.sources} selectedName={data.previewName} />
		<ParserPreview sourceName={data.previewName} sample={data.previewSample} />
	</div>
	<aside class="rail">
		<section class="panel">
			<h2>Point a device here</h2>
			<p class="body">
				UniFi: <strong>Settings → System → Advanced → Remote Logging</strong>. Point it at:
			</p>
			<dl class="mono">
				<div class="row"><dt>Syslog (UDP)</dt><dd>514</dd></div>
				<div class="row"><dt>Syslog (TCP)</dt><dd>601</dd></div>
				<div class="row"><dt>Syslog (TLS)</dt><dd>6514</dd></div>
			</dl>
		</section>
		<IngestHealthPanel health={data.health} />
		<UnclaimedSenders sources={data.unclaimedSources} canClaim={data.userRole === 'admin'} />
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
	.panel {
		background: var(--color-surface-2);
		border-radius: var(--radius-default);
		padding: var(--space-4);
	}
	h2 {
		font-size: var(--text-section-head);
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
