import { error, redirect } from '@sveltejs/kit';
import { API_URL } from '$app/env/private';
import type { PageServerLoad } from './$types';
import { SiemApiClient, SiemApiError } from '#lib/server/siemApiClient.ts';

export const load: PageServerLoad = async ({ locals }) => {
	const client = new SiemApiClient({ baseUrl: API_URL as string });
	const token = locals.sessionToken as string;

	let settings;
	let notificationSettings;
	let ollamaSettings;
	try {
		settings = await client.getAuthSettings(token);
		notificationSettings = await client.getNotificationSettings(token);
		ollamaSettings = await client.getOllamaSettings(token);
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

	return { roleMappings: settings.role_mappings ?? [], notificationSettings, ollamaSettings };
};
