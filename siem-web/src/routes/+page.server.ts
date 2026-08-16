import { redirect, error } from '@sveltejs/kit';
import { env } from '$env/dynamic/private';
import type { PageServerLoad } from './$types';
import { SiemApiClient, SiemApiError, type Insight } from '$lib/server/siemApiClient';
import { topTriageAlerts, deriveCountryBreakdown, buildSourceLabels } from '$lib/wall';

export const load: PageServerLoad = async ({ locals }) => {
	const client = new SiemApiClient({ baseUrl: env.API_URL as string });
	const token = locals.sessionToken as string;

	// The Insights panel is supplementary, not gated content - a failure here
	// (including "insights isn't configured at all") must never break the
	// Wall or force a redirect, same degrade-gracefully posture as
	// +layout.server.ts's nav summary. Started here (not awaited until after
	// the gating fetches below) so its round trip overlaps with theirs
	// instead of adding to the Wall's load time on top of them.
	const insightsPromise: Promise<Insight[]> = client.getInsights(token).catch((err) => {
		console.error('wall: insights lookup failed', err);
		return [];
	});

	// Same supplementary posture, for the same reason as search/+page.server.ts's
	// claimedSourcesPromise: HeatGrid's rows only ever carry the raw source
	// name (siem-api's /events/stats has no notion of display_name), so an
	// operator-set rename needs this separate lookup to show up at all.
	const sourceLabelsPromise = client
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

	let stats, openAlerts, sample;
	try {
		[stats, openAlerts, sample] = await Promise.all([
			client.getEventsStats(token),
			client.getAlerts(token, 'open'),
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
			client.search(token, { limit: '200', volume: 'false', geoip: 'true' })
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

	const insights = await insightsPromise;
	const sourceLabels = await sourceLabelsPromise;

	return {
		eventCount24h: stats.event_count_24h,
		heatGrid: stats.heat_grid,
		sourceLabels,
		hourlyTotals: stats.hourly_totals ?? [],
		openAlertCount: openAlerts.length,
		triageAlerts: topTriageAlerts(openAlerts),
		countryBreakdown: deriveCountryBreakdown(sample.entries),
		insights
	};
};
