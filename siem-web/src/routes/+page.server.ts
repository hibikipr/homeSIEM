import { redirect, error } from '@sveltejs/kit';
import { env } from '$env/dynamic/private';
import type { PageServerLoad } from './$types';
import { SiemApiClient, SiemApiError, type Insight } from '$lib/server/siemApiClient';
import { topTriageAlerts, deriveCountryBreakdown } from '$lib/wall';

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

	return {
		eventCount24h: stats.event_count_24h,
		heatGrid: stats.heat_grid,
		hourlyTotals: stats.hourly_totals ?? [],
		openAlertCount: openAlerts.length,
		triageAlerts: topTriageAlerts(openAlerts),
		countryBreakdown: deriveCountryBreakdown(sample.entries),
		insights
	};
};
