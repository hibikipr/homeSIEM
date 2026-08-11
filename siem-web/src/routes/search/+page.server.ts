import { error, redirect } from '@sveltejs/kit';
import { env } from '$env/dynamic/private';
import type { PageServerLoad } from './$types';
import { SiemApiClient, SiemApiError } from '$lib/server/siemApiClient';
import {
	parseFiltersFromURL,
	filtersToSearchParams,
	rangeToSeconds,
	extractSrcIp
} from '$lib/search';

export const load: PageServerLoad = async ({ locals, url }) => {
	const client = new SiemApiClient({ baseUrl: env.API_URL as string });
	const token = locals.sessionToken as string;

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

	const previewParam = url.searchParams.get('preview');
	const parsedPreview = previewParam !== null ? Number(previewParam) : null;
	const previewIndex =
		parsedPreview !== null && Number.isInteger(parsedPreview) ? parsedPreview : null;
	const selectedEntry =
		previewIndex !== null && previewIndex >= 0 && previewIndex < result.entries.length
			? result.entries[previewIndex]
			: null;

	let contextSummary: { count: number } | null = null;
	if (selectedEntry) {
		const srcIp = extractSrcIp(selectedEntry.Line);
		if (srcIp) {
			try {
				// entries=false/volume=false: this callout only ever shows
				// contextResult.count - fetching up to 5000 full log entries
				// (and a volume histogram) just to read their length was
				// measurably slower than asking siem-api for a real
				// aggregate count directly.
				const contextResult = await client.search(token, {
					q: srcIp,
					start: new Date(end.getTime() - 24 * 60 * 60 * 1000).toISOString(),
					end: end.toISOString(),
					entries: 'false',
					volume: 'false'
				});
				contextSummary = { count: contextResult.count };
			} catch (err) {
				// Context callout is supplementary — a failure here shouldn't
				// take down the rest of the page.
				console.error('search: context summary lookup failed', err);
				contextSummary = null;
			}
		}
	}

	return {
		filters,
		logql: result.logql,
		count: result.count,
		entries: result.entries,
		volume: result.volume,
		previewIndex,
		selectedEntry,
		contextSummary
	};
};
