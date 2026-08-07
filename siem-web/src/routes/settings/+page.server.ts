import { error, redirect } from '@sveltejs/kit';
import { env } from '$env/dynamic/private';
import type { PageServerLoad } from './$types';
import { SiemApiClient, SiemApiError } from '$lib/server/siemApiClient';

export const load: PageServerLoad = async ({ locals }) => {
	const client = new SiemApiClient({ baseUrl: env.API_URL as string });
	const token = locals.sessionToken as string;

	let settings;
	try {
		settings = await client.getAuthSettings(token);
	} catch (err) {
		if (err instanceof SiemApiError) {
			if (err.status === 401) {
				redirect(302, '/auth/logout');
			}
			if (err.status === 403) {
				error(403, 'Settings is only available to admins.');
			}
			error(502, 'siem-api unavailable');
		}
		throw err;
	}

	return { roleMappings: settings.role_mappings ?? [] };
};
