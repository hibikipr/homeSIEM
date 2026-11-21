import { API_URL } from '$app/env/private';
import type { PageServerLoad } from './$types';
import { SiemApiClient, type LogEntry, type SourceResponse } from '#lib/server/siemApiClient.ts';
import { fetchOpenAlertsOrBail } from '#lib/server/sessionProbe.ts';
import { splitClaimedUnclaimed } from '#lib/sources.ts';
import { DEP_SOURCES } from '#lib/invalidation.ts';

const PREVIEW_SAMPLE_LIMIT = 10;

export interface SourcesRegistry {
	sources: SourceResponse[];
	claimedSources: SourceResponse[];
	unclaimedSources: SourceResponse[];
	previewName: string | null;
}

export const load: PageServerLoad = async ({ locals, url, depends }) => {
	depends(DEP_SOURCES);
	const client = new SiemApiClient({ baseUrl: API_URL as string });
	const token = locals.sessionToken as string;

	// Everything Loki-backed streams so the page shell (the "Point a device
	// here" rail, panel frames) renders immediately. GET /sources carries a
	// Loki-derived events/min column, so the registry is slow too and is
	// streamed rather than awaited. None of these promises reject: a failure
	// resolves to null / a safe default and the page shows it inline.

	const registry: Promise<SourcesRegistry | null> = client
		.getSources(token)
		.then((sources) => {
			const { claimed, unclaimed } = splitClaimedUnclaimed(sources);
			return {
				sources,
				claimedSources: claimed,
				unclaimedSources: unclaimed,
				previewName: url.searchParams.get('preview') ?? sources[0]?.name ?? null
			};
		})
		.catch((err) => {
			console.error('sources: registry lookup failed', err);
			return null;
		});

	// Ingest-health is supplementary (per design spec) — a Vector/Loki hiccup
	// here shouldn't take down the whole Sources screen.
	const health = client.getIngestHealth(token).catch((err) => {
		console.error('sources: ingest health lookup failed', err);
		return {
			received_events_per_source: {},
			loki_sent_events_total: 0,
			blank_messages_filtered_total: 0,
			degraded: true
		};
	});

	// PREVIEW_SAMPLE_LIMIT: was 1 (just the single most recent line) - the
	// parser preview is meant to let you confirm a source's messages are
	// actually parsing the way you expect, and one sample can easily be an
	// outlier (a startup banner, a one-off warning) that isn't
	// representative of what this source normally sends.
	// Parser preview is supplementary — a Loki hiccup degrades to no samples.
	const previewSamples: Promise<LogEntry[]> = registry.then(async (r) => {
		if (!r?.previewName) return [];
		try {
			const result = await client.search(token, {
				source: r.previewName,
				limit: String(PREVIEW_SAMPLE_LIMIT)
			});
			return result.entries;
		} catch (err) {
			console.error('sources: preview samples lookup failed', err);
			return [];
		}
	});

	// A stream can't redirect: the awaited, SQLite-fast call below catches a
	// stale/demoted session (401/403 -> /auth/logout, other failure -> 502)
	// before any HTML is sent. The streamed calls above run concurrently.
	await fetchOpenAlertsOrBail(client, token);

	return {
		registry,
		previewSamples,
		health,
		userRole: locals.user?.role
	};
};
