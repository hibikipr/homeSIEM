import { env } from '$env/dynamic/private';
import type { PageServerLoad } from './$types';
import { SiemApiClient } from '$lib/server/siemApiClient';
import { topTriageAlerts, deriveCountryBreakdown } from '$lib/wall';

export const load: PageServerLoad = async ({ locals }) => {
	const client = new SiemApiClient({ baseUrl: env.API_URL as string });
	const token = locals.sessionToken as string;

	const [stats, openAlerts, sample] = await Promise.all([
		client.getEventsStats(token),
		client.getAlerts(token, 'open'),
		client.search(token, {})
	]);

	return {
		eventCount24h: stats.event_count_24h,
		heatGrid: stats.heat_grid,
		openAlertCount: openAlerts.length,
		triageAlerts: topTriageAlerts(openAlerts),
		countryBreakdown: deriveCountryBreakdown(sample.entries)
	};
};
