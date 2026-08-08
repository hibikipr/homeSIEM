import { redirect, error } from '@sveltejs/kit';
import { env } from '$env/dynamic/private';
import type { PageServerLoad } from './$types';
import { SiemApiClient, SiemApiError } from '$lib/server/siemApiClient';
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

	return {
		eventCount24h: stats.event_count_24h,
		heatGrid: stats.heat_grid,
		hourlyTotals: stats.hourly_totals ?? [],
		openAlertCount: openAlerts.length,
		triageAlerts: topTriageAlerts(openAlerts),
		countryBreakdown: deriveCountryBreakdown(sample.entries)
	};
};
