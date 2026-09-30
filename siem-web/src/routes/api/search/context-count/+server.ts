import { json } from '@sveltejs/kit';
import { env } from '$env/dynamic/private';
import type { RequestHandler } from './$types';
import { SiemApiClient, SiemApiError } from '$lib/server/siemApiClient';

// Backs Search's "N events from this IP in the last 24h" inspector
// callout (see search/+page.ts). entries/volume/facets=false: the callout
// only reads the Loki-side total count, so every other scan is skipped.
export const GET: RequestHandler = async ({ url, locals }) => {
	const ip = url.searchParams.get('ip') ?? '';
	if (!ip) {
		return json({ error: 'ip is required' }, { status: 400 });
	}

	const client = new SiemApiClient({ baseUrl: env.API_URL as string });
	const end = new Date();
	try {
		const result = await client.search(locals.sessionToken as string, {
			q: ip,
			start: new Date(end.getTime() - 24 * 60 * 60 * 1000).toISOString(),
			end: end.toISOString(),
			entries: 'false',
			volume: 'false',
			facets: 'false'
		});
		return json({ count: result.count });
	} catch (err) {
		if (err instanceof SiemApiError) {
			return json({ error: err.message }, { status: err.status });
		}
		throw err;
	}
};
