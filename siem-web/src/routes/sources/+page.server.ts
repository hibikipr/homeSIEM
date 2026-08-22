import { redirect, error } from '@sveltejs/kit';
import { env } from '$env/dynamic/private';
import type { PageServerLoad } from './$types';
import { SiemApiClient, SiemApiError } from '$lib/server/siemApiClient';
import { splitClaimedUnclaimed } from '$lib/sources';

export const load: PageServerLoad = async ({ locals, url }) => {
	const client = new SiemApiClient({ baseUrl: env.API_URL as string });
	const token = locals.sessionToken as string;

	let sources;
	try {
		sources = await client.getSources(token);
	} catch (err) {
		if (err instanceof SiemApiError) {
			if (err.status === 401 || err.status === 403) {
				redirect(302, '/auth/logout');
			}
			error(502, 'siem-api unavailable');
		}
		throw err;
	}

	let health;
	try {
		health = await client.getIngestHealth(token);
	} catch (err) {
		// Ingest-health is supplementary (per design spec) — a Vector/Loki hiccup
		// here shouldn't take down the whole Sources screen.
		if (err instanceof SiemApiError && (err.status === 401 || err.status === 403)) {
			redirect(302, '/auth/logout');
		}
		health = {
			received_events_per_source: {},
			loki_sent_events_total: 0,
			blank_messages_filtered_total: 0,
			degraded: true
		};
	}

	const previewName = url.searchParams.get('preview') ?? sources[0]?.name ?? null;

	let previewSample = null;
	if (previewName) {
		try {
			const result = await client.search(token, { source: previewName, limit: '1' });
			previewSample = result.entries[0] ?? null;
		} catch (err) {
			// Parser preview is supplementary (per design spec) — a Loki hiccup
			// here shouldn't take down the whole Sources screen.
			if (err instanceof SiemApiError && (err.status === 401 || err.status === 403)) {
				redirect(302, '/auth/logout');
			}
			previewSample = null;
		}
	}

	const { claimed, unclaimed } = splitClaimedUnclaimed(sources);

	return {
		sources,
		claimedSources: claimed,
		unclaimedSources: unclaimed,
		previewName,
		previewSample,
		health,
		userRole: locals.user?.role
	};
};
