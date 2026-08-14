import { json } from '@sveltejs/kit';
import { env } from '$env/dynamic/private';
import type { RequestHandler } from './$types';
import { SiemApiClient, SiemApiError } from '$lib/server/siemApiClient';

export const DELETE: RequestHandler = async ({ params, locals }) => {
	const client = new SiemApiClient({ baseUrl: env.API_URL as string });
	const token = locals.sessionToken as string;

	try {
		await client.unmuteInsight(token, params.fingerprint as string);
	} catch (err) {
		if (err instanceof SiemApiError) {
			return json({ error: err.message }, { status: err.status });
		}
		throw err;
	}

	return new Response(null, { status: 204 });
};
