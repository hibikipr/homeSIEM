import { env } from '$env/dynamic/private';
import { SiemApiClient } from '$lib/server/siemApiClient';
import type { LayoutServerLoad } from './$types';

export const load: LayoutServerLoad = async ({ locals, url }) => {
	// Nav chrome's "ingest live X/min" text and alert-count badge were
	// previously hardcoded to 0 - no data source at all. Fetched here
	// (rather than a page-level load) since Nav renders on every route.
	// Only attempted for an authenticated session, and never lets a
	// failure here break the page or force a redirect - this is
	// supplementary chrome, not gated content; the real page-level
	// loaders already handle actual auth gating.
	let ingestRate = 0;
	let alertCount = 0;
	if (locals.user && locals.sessionToken) {
		try {
			const client = new SiemApiClient({ baseUrl: env.API_URL as string });
			const summary = await client.getNavSummary(locals.sessionToken as string);
			ingestRate = summary.events_per_min;
			alertCount = summary.open_alert_count;
		} catch (err) {
			console.error('layout: nav summary lookup failed', err);
		}
	}

	return {
		user: locals.user,
		activeRoute: url.pathname,
		ingestRate,
		alertCount
	};
};
