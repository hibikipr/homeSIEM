import { json } from '@sveltejs/kit';
import { env } from '$env/dynamic/private';
import type { RequestHandler } from './$types';
import { SiemApiClient, SiemApiError } from '$lib/server/siemApiClient';

export const PUT: RequestHandler = async ({ params, request, locals }) => {
	const client = new SiemApiClient({ baseUrl: env.API_URL as string });
	const token = locals.sessionToken as string;
	const id = Number(params.id);
	const body = await request.json().catch(() => ({}));
	const displayName = typeof body.display_name === 'string' ? body.display_name : '';

	try {
		await client.renameSource(token, id, displayName);
	} catch (err) {
		if (err instanceof SiemApiError) {
			return json({ error: err.message }, { status: err.status });
		}
		throw err;
	}

	return new Response(null, { status: 204 });
};
