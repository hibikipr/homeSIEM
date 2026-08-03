import { error, redirect } from '@sveltejs/kit';
import { env } from '$env/dynamic/private';
import type { PageServerLoad } from './$types';
import { SiemApiClient, SiemApiError } from '$lib/server/siemApiClient';
import { deriveAlertStats } from '$lib/alerts';

export const load: PageServerLoad = async ({ locals, url }) => {
	const client = new SiemApiClient({ baseUrl: env.API_URL as string });
	const token = locals.sessionToken as string;

	const tabParam = url.searchParams.get('state');
	const tab: 'open' | 'acked' | 'rules' =
		tabParam === 'acked' || tabParam === 'rules' ? tabParam : 'open';
	const selectedId = url.searchParams.get('id');

	let alerts, rules;
	try {
		[alerts, rules] = await Promise.all([
			tab === 'rules' ? Promise.resolve([]) : client.getAlerts(token, tab),
			client.getRules(token)
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

	const selectedAlert =
		tab !== 'rules' && selectedId
			? (alerts.find((a) => a.id === Number(selectedId)) ?? null)
			: null;
	const selectedRule =
		tab === 'rules' && selectedId ? (rules.find((r) => r.id === Number(selectedId)) ?? null) : null;

	let selectedSamples;
	try {
		selectedSamples = selectedAlert ? await client.getAlertSamples(token, selectedAlert.id) : [];
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
		tab,
		alerts,
		rules,
		selectedAlert,
		selectedSamples,
		stats: selectedAlert ? deriveAlertStats(selectedSamples) : null,
		selectedRule
	};
};
