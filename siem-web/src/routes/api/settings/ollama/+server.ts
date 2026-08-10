import { json } from '@sveltejs/kit';
import { env } from '$env/dynamic/private';
import type { RequestHandler } from './$types';
import {
	SiemApiClient,
	SiemApiError,
	type UpdateOllamaSettingsRequest
} from '$lib/server/siemApiClient';

export const GET: RequestHandler = async ({ locals }) => {
	const client = new SiemApiClient({ baseUrl: env.API_URL as string });
	const token = locals.sessionToken as string;

	try {
		const settings = await client.getOllamaSettings(token);
		return json(settings);
	} catch (err) {
		if (err instanceof SiemApiError) {
			return json({ error: err.message }, { status: err.status });
		}
		throw err;
	}
};

export const PUT: RequestHandler = async ({ request, locals }) => {
	const client = new SiemApiClient({ baseUrl: env.API_URL as string });
	const token = locals.sessionToken as string;
	const body = (await request.json()) as UpdateOllamaSettingsRequest;

	try {
		await client.updateOllamaSettings(token, body);
	} catch (err) {
		if (err instanceof SiemApiError) {
			return json({ error: err.message }, { status: err.status });
		}
		throw err;
	}

	return new Response(null, { status: 204 });
};
