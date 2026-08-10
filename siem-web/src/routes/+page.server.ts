import { redirect, error } from '@sveltejs/kit';
import { env } from '$env/dynamic/private';
import type { PageServerLoad } from './$types';
import { SiemApiClient, SiemApiError, type Insight } from '$lib/server/siemApiClient';
import { topTriageAlerts, deriveCountryBreakdown } from '$lib/wall';

export const load: PageServerLoad = async ({ locals }) => {
	const client = new SiemApiClient({ baseUrl: env.API_URL as string });
	const token = locals.sessionToken as string;

	let stats, openAlerts, sample;
	try {
		[stats, openAlerts, sample] = await Promise.all([
			client.getEventsStats(token),
			client.getAlerts(token, 'open'),
			client.search(token, { limit: '1000' })
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

	// The Insights panel is supplementary, not gated content - unlike the
	// stats/alerts/sample fetch above, a failure here (including "insights
	// isn't configured at all") must never break the Wall or force a
	// redirect. Same degrade-gracefully posture as +layout.server.ts's nav
	// summary.
	let insights: Insight[] = [];
	try {
		insights = await client.getInsights(token);
	} catch (err) {
		console.error('wall: insights lookup failed', err);
	}

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
