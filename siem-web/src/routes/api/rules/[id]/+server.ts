import { json } from '@sveltejs/kit';
import { env } from '$env/dynamic/private';
import type { RequestHandler } from './$types';
import { SiemApiClient, SiemApiError } from '$lib/server/siemApiClient';
import type { CreateRuleRequest } from '$lib/server/siemApiClient';

export const PUT: RequestHandler = async ({ request, params, locals }) => {
	const client = new SiemApiClient({ baseUrl: env.API_URL as string });
	const token = locals.sessionToken as string;
	const id = Number(params.id);
	const body = (await request.json()) as CreateRuleRequest;

	try {
		const rule = await client.updateRule(token, id, body);
		return json(rule);
	} catch (err) {
		if (err instanceof SiemApiError) {
			return json({ error: err.message }, { status: err.status });
		}
		throw err;
	}
};
