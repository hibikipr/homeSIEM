import { redirect, error } from '@sveltejs/kit';
import { env } from '$env/dynamic/private';
import type { PageServerLoad } from './$types';
import { SiemApiClient, SiemApiError, type LogEntry } from '$lib/server/siemApiClient';
import { splitClaimedUnclaimed } from '$lib/sources';

const PREVIEW_SAMPLE_LIMIT = 10;

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

	// PREVIEW_SAMPLE_LIMIT: was 1 (just the single most recent line) - the
	// parser preview is meant to let you confirm a source's messages are
	// actually parsing the way you expect, and one sample can easily be an
	// outlier (a startup banner, a one-off warning) that isn't
	// representative of what this source normally sends.
	let previewSamples: LogEntry[] = [];
	if (previewName) {
		try {
			const result = await client.search(token, {
				source: previewName,
				limit: String(PREVIEW_SAMPLE_LIMIT)
			});
			previewSamples = result.entries;
		} catch (err) {
			// Parser preview is supplementary (per design spec) — a Loki hiccup
			// here shouldn't take down the whole Sources screen.
			if (err instanceof SiemApiError && (err.status === 401 || err.status === 403)) {
				redirect(302, '/auth/logout');
			}
			previewSamples = [];
		}
	}

	const { claimed, unclaimed } = splitClaimedUnclaimed(sources);

	return {
		sources,
		claimedSources: claimed,
		unclaimedSources: unclaimed,
		previewName,
		previewSamples,
		health,
		userRole: locals.user?.role
	};
};
