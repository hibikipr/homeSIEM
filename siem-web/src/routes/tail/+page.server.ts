import { error, redirect } from '@sveltejs/kit';
import { API_URL } from '$app/env/private';
import type { PageServerLoad } from './$types';
import { SiemApiClient, SiemApiError } from '#lib/server/siemApiClient.ts';

export const load: PageServerLoad = async ({ locals }) => {
	const client = new SiemApiClient({ baseUrl: API_URL as string });
	const token = locals.sessionToken as string;

	let sources;
	try {
		sources = await client.getSources(token);
	} catch (err) {
		if (err instanceof SiemApiError) {
			if (err.status === 401 || err.status === 403) {
				redirect(302, '/auth/logout');
			}
			error(502, 'siem-api unavailable');
		}
		throw err;
	}

	return { sourceCount: sources.length };
};
