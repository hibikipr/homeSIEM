<script lang="ts">
	let { url }: { url: string } = $props();

	// theme=dark keeps the embed close to the Wall's own dark palette (Grafana
	// still renders its own fonts/chrome inside the frame - this doesn't make
	// it disappear, just keeps it from being jarringly light). Public
	// dashboards already omit Grafana's top nav/sidebar by default, so no
	// separate kiosk param is needed.
	let src = $derived(`${url}${url.includes('?') ? '&' : '?'}theme=dark`);
</script>

<div class="panel">
	<div class="panel-head">
		<span class="title">Host health</span>
		<span class="badge">Grafana</span>
	</div>
	<div class="frame-wrap">
		<iframe {src} title="Host CPU, RAM, and disk utilization" loading="lazy"></iframe>
	</div>
	<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- external Grafana URL, not a SvelteKit route -->
	<a class="open-link" href={url} target="_blank" rel="noopener noreferrer">Open in Grafana ↗</a>
</div>

<style>
	.panel {
		background: var(--color-surface-2);
		border-radius: var(--radius-default);
		box-shadow: var(--shadow-flat);
		padding: var(--space-4);
		display: flex;
		flex-direction: column;
		gap: var(--space-3);
	}
	.panel-head {
		display: flex;
		justify-content: space-between;
		align-items: baseline;
	}
	.title {
		font-size: var(--text-eyebrow);
		text-transform: uppercase;
		letter-spacing: 0.1em;
		color: var(--color-muted-2);
	}
	.badge {
		font-size: 9.5px;
		text-transform: uppercase;
		letter-spacing: 0.04em;
		color: var(--color-muted-2);
		border: 1px solid var(--color-line-2);
		border-radius: var(--radius-sm);
		padding: 1px 6px;
	}
	.frame-wrap {
		border-radius: var(--radius-sm);
		overflow: hidden;
		border: 1px solid var(--color-line);
		background: var(--color-surface-3);
	}
	.frame-wrap iframe {
		display: block;
		width: 100%;
		/* Grafana's public-dashboard page is a full-viewport flex column
		   with its "Powered by Grafana" footer pushed to the very bottom
		   via margin-top:auto (or similar) - it does NOT sit right after
		   the content. Any iframe height taller than what's actually needed
		   creates a visible gap, since the footer chases the iframe's
		   bottom edge, not the content above it. Measured precisely via
		   getBoundingClientRect() on the live embed: last data row bottom
		   at 389px, footer top at 671px in a 697px-tall viewport - i.e. the
		   footer was floating ~280px below the real content. 430px got the
		   footer right under the content but clipped the last data row by
		   a few px in the actual iframe context (vs. the standalone tab
		   used to measure) - 450px gives a small buffer. */
		height: 450px;
		border: 0;
	}
	.open-link {
		align-self: flex-end;
		font-size: 10.5px;
		color: var(--color-accent-light);
		text-decoration: none;
	}
</style>
