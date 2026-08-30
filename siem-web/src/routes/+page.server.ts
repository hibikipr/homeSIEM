import { redirect, error } from '@sveltejs/kit';
import { env } from '$env/dynamic/private';
import type { PageServerLoad } from './$types';
import { SiemApiClient, SiemApiError, type Insight } from '$lib/server/siemApiClient';
import { topTriageAlerts, deriveCountryBreakdown, buildSourceLabels } from '$lib/wall';

export const load: PageServerLoad = async ({ locals }) => {
	const client = new SiemApiClient({ baseUrl: env.API_URL as string });
	const token = locals.sessionToken as string;

	// Streamed to the client rather than awaited here - none of these three
	// are needed to render the Wall's primary content (stats/alerts/heatmap
	// below), so blocking the whole page on them turned "first paint" into
	// "slowest of five network calls." SvelteKit sends the rest of the page
	// immediately and patches these in via +page.svelte's {#await} blocks
	// once each resolves. Each degrades to an empty result on failure rather
	// than ever redirecting/erroring - same posture as +layout.server.ts's
	// nav summary, just now also true of countryBreakdown (previously
	// blocking, see git history for why it moved here).

	// The Insights panel is supplementary, not gated content - a failure here
	// (including "insights isn't configured at all") must never break the
	// Wall.
	const insights: Promise<Insight[]> = client.getInsights(token).catch((err) => {
		console.error('wall: insights lookup failed', err);
		return [];
	});

	// HeatGrid's rows only ever carry the raw source name (siem-api's
	// /events/stats has no notion of display_name), so an operator-set
	// rename needs this separate lookup to show up at all - HeatGrid
	// defaults to an empty label map, so rows just show raw names until
	// this resolves.
	const sourceLabels = client
		.getSources(token)
		.then((sources) =>
			buildSourceLabels(
				sources.filter((s) => s.claimed).map((s) => ({ name: s.name, displayName: s.display_name }))
			)
		)
		.catch((err) => {
			console.error('wall: sources lookup failed', err);
			return {} as Record<string, string>;
		});

	const countryBreakdown = client
		.search(token, {
			limit: '200',
			volume: 'false',
			// geoip=true: found in production that geoip-bearing security
			// events (a UniFi threat/block with a public src or dst IP) are
			// a tiny fraction of this host's overall log volume - an
			// unfiltered recent-200 sample almost never contained one at
			// all, even when enrich_geo was enriching them correctly, so
			// the country breakdown was reliably empty. Filtering at the
			// LogQL level means every one of the 200 entries can actually
			// contribute a country. volume=false skips a whole extra Loki
			// query server-side that would otherwise be fetched and
			// immediately discarded.
			geoip: 'true'
		})
		.then((sample) => deriveCountryBreakdown(sample.entries))
		.catch((err) => {
			console.error('wall: country breakdown lookup failed', err);
			return [];
		});

	let stats, openAlerts;
	try {
		[stats, openAlerts] = await Promise.all([
			client.getEventsStats(token),
			client.getAlerts(token, 'open')
		]);
	} catch (err) {
		if (err instanceof SiemApiError) {
			if (err.status === 401 || err.status === 403) {
				redirect(302, '/auth/logout');
			}
			error(502, 'siem-api unavailable');
		}
		throw err;
	}

	return {
		eventCount24h: stats.event_count_24h,
		heatGrid: stats.heat_grid,
		sourceLabels,
		hourlyTotals: stats.hourly_totals ?? [],
		openAlertCount: openAlerts.length,
		triageAlerts: topTriageAlerts(openAlerts),
		countryBreakdown,
		insights,
		// Not every deployment has Grafana's public-dashboard sharing set up -
		// undefined here means the Wall omits the card entirely rather than
		// embedding a broken iframe.
		grafanaHostHealthUrl: (env.GRAFANA_HOST_HEALTH_URL as string) || undefined
	};
};
