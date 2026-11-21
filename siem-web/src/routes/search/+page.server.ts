import { API_URL } from '$app/env/private';
import type { PageServerLoad } from './$types';
import {
	SiemApiClient,
	type LogEntry,
	type VolumeBucket,
	type ApiFacetCount
} from '#lib/server/siemApiClient.ts';
import { fetchOpenAlertsOrBail } from '#lib/server/sessionProbe.ts';
import { DEP_SOURCES } from '#lib/invalidation.ts';
import { parseFiltersFromURL, filtersToSearchParams, rangeToSeconds } from '#lib/search.ts';

const SEARCH_WINDOW_ALIGN_MS = 30_000;

export interface SearchResults {
	logql: string;
	count: number;
	entries: LogEntry[];
	volume: VolumeBucket[];
	facets?: Record<string, ApiFacetCount[]>;
}

// Deliberately never reads `?preview=` (the selected row) - SvelteKit
// tracks which search params a load reads and only reruns it when one of
// those changes, so selecting a row (see +page.ts) no longer re-runs this
// whole search (entries + count + volume + four facet scans) against
// Loki. That rerun also re-anchored the time window to a new "now", so on
// a busy system `preview=N` could land on a different event than the one
// clicked; with no rerun, the entries - and the index - stay put.
export const load: PageServerLoad = async ({ locals, url, depends }) => {
	// Only the claimed-sources facet list can change from an action.
	depends(DEP_SOURCES);
	const client = new SiemApiClient({ baseUrl: API_URL as string });
	const token = locals.sessionToken as string;

	// The Source facet needs the full list of claimed sources, not just
	// whatever happens to be in the current (filtered, capped) result set -
	// see mergeSourceFacet's own comment for why. Supplementary, not gated
	// content: streamed to the client (not awaited before load() returns -
	// see the return statement) rather than blocking the whole page on it,
	// and a failure here degrades to an empty list rather than breaking the
	// page. Carries display_name alongside name so the facet can show an
	// operator-set rename (e.g. "Home Assistant" instead of a bare IP)
	// without changing what onFacetClick actually filters on.
	const claimedSources = client
		.getSources(token)
		.then((sources) =>
			sources.filter((s) => s.claimed).map((s) => ({ name: s.name, displayName: s.display_name }))
		)
		.catch((err) => {
			console.error('search: sources lookup failed', err);
			return [] as { name: string; displayName: string }[];
		});

	const filters = parseFiltersFromURL(url);
	// siem-api caches /events/search for 30s keyed by the exact start/end, so
	// a millisecond-precise "now" never hits. Round the window's end UP to the
	// next 30s boundary (a slightly-future end is harmless to Loki and still
	// includes the newest events) so repeat loads within a window share a key.
	const end = new Date(Math.ceil(Date.now() / SEARCH_WINDOW_ALIGN_MS) * SEARCH_WINDOW_ALIGN_MS);
	const start = new Date(end.getTime() - rangeToSeconds(filters.range) * 1000);

	// The slow, Loki-backed part (entries + count + volume + facets) is
	// streamed so the query bar / facet rail frame render immediately. It
	// resolves to null on failure (the page shows an inline "couldn't load")
	// - never rejects, so there is no unhandled rejection. Started before the
	// auth probe below so the two run concurrently.
	const results: Promise<SearchResults | null> = client
		.search(token, {
			...filtersToSearchParams(filters),
			start: start.toISOString(),
			end: end.toISOString(),
			// siem-api's own default (when no limit is given) is 1000 — match it
			// explicitly rather than requesting more. An unfiltered, 24h-range
			// search asking for 10000 rows was intermittently exceeding Loki's
			// query timeout on real deployments (Loki returning that many raw
			// log lines is measurably more expensive than a filtered/narrower
			// query), turning the default Search view into a 502.
			limit: '1000'
		})
		.then((r) => ({
			logql: r.logql,
			count: r.count,
			entries: r.entries,
			volume: r.volume,
			facets: r.facets
		}))
		.catch((err) => {
			console.error('search: query failed', err);
			return null;
		});

	// A stream can't redirect, so a stale/demoted session is caught by this
	// awaited, SQLite-fast call instead (same `protect` middleware as the
	// search itself): 401/403 -> /auth/logout before any HTML is sent.
	await fetchOpenAlertsOrBail(client, token);

	return {
		filters,
		results,
		claimedSources
	};
};
