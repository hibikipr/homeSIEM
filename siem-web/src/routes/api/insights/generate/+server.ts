import { json } from '@sveltejs/kit';
import { API_URL } from '$app/env/private';
import type { RequestHandler } from './$types';
import { SiemApiClient, SiemApiError } from '#lib/server/siemApiClient.ts';

export const POST: RequestHandler = async ({ locals }) => {
	const client = new SiemApiClient({ baseUrl: API_URL as string });
	const token = locals.sessionToken as string;

	try {
		const result = await client.generateInsightsNow(token);
		return json(result);
	} catch (err) {
		if (err instanceof SiemApiError) {
			return json({ error: err.message }, { status: err.status });
		}
		throw err;
	}
};
