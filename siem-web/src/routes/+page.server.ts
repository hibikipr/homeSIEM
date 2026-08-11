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
			// The Wall only needs a representative sample to derive a country
			// breakdown from, not an exhaustive one, and doesn't use the
			// volume histogram /events/search also computes by default - 200
			// entries is still plenty for a stable "top countries" picture,
			// and volume=false skips a whole extra Loki query server-side
			// that would otherwise be fetched and immediately discarded.
			client.search(token, { limit: '200', volume: 'false' })
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
