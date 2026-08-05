import { json } from '@sveltejs/kit';
import { env } from '$env/dynamic/private';
import type { RequestHandler } from './$types';
import { SiemApiClient, SiemApiError } from '$lib/server/siemApiClient';
import type { CreateRuleRequest } from '$lib/server/siemApiClient';

export const POST: RequestHandler = async ({ request, locals }) => {
	const client = new SiemApiClient({ baseUrl: env.API_URL as string });
	const token = locals.sessionToken as string;
	const body = (await request.json()) as CreateRuleRequest;

	try {
		const rule = await client.createRule(token, body);
		return json(rule, { status: 201 });
	} catch (err) {
		if (err instanceof SiemApiError) {
			return json({ error: err.message }, { status: err.status });
		}
		throw err;
	}
};
