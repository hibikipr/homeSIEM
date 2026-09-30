import { error, redirect } from '@sveltejs/kit';
import { env } from '$env/dynamic/private';
import type { PageServerLoad } from './$types';
import { SiemApiClient, SiemApiError } from '$lib/server/siemApiClient';
import { parseFiltersFromURL, filtersToSearchParams, rangeToSeconds } from '$lib/search';

// Deliberately never reads `?preview=` (the selected row) - SvelteKit
// tracks which search params a load reads and only reruns it when one of
// those changes, so selecting a row (see +page.ts) no longer re-runs this
// whole search (entries + count + volume + four facet scans) against
// Loki. That rerun also re-anchored the time window to a new "now", so on
// a busy system `preview=N` could land on a different event than the one
// clicked; with no rerun, the entries - and the index - stay put.
export const load: PageServerLoad = async ({ locals, url }) => {
	const client = new SiemApiClient({ baseUrl: env.API_URL as string });
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
	const end = new Date();
	const start = new Date(end.getTime() - rangeToSeconds(filters.range) * 1000);

	let result;
	try {
		result = await client.search(token, {
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
		});
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
		filters,
		logql: result.logql,
		count: result.count,
		entries: result.entries,
		volume: result.volume,
		facets: result.facets,
		claimedSources
	};
};
