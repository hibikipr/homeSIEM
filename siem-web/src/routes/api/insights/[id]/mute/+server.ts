import { json } from '@sveltejs/kit';
import { API_URL } from '$app/env/private';
import type { RequestHandler } from './$types';
import { SiemApiClient, SiemApiError } from '#lib/server/siemApiClient.ts';

export const PUT: RequestHandler = async ({ params, locals }) => {
	const client = new SiemApiClient({ baseUrl: API_URL as string });
	const token = locals.sessionToken as string;
	const id = Number(params.id);

	try {
		await client.muteInsight(token, id);
	} catch (err) {
		if (err instanceof SiemApiError) {
			return json({ error: err.message }, { status: err.status });
		}
		throw err;
	}

	return new Response(null, { status: 204 });
};
