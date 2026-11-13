import { json } from '@sveltejs/kit';
import { API_URL } from '$app/env/private';
import type { RequestHandler } from './$types';
import { SiemApiClient, SiemApiError } from '#lib/server/siemApiClient.ts';

export const GET: RequestHandler = async ({ locals }) => {
	const client = new SiemApiClient({ baseUrl: API_URL as string });
	const token = locals.sessionToken as string;

	try {
		const muted = await client.listMutedInsights(token);
		return json(muted);
	} catch (err) {
		if (err instanceof SiemApiError) {
			return json({ error: err.message }, { status: err.status });
		}
		throw err;
	}
};
