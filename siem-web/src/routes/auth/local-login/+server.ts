import { json } from '@sveltejs/kit';
import { env } from '$env/dynamic/private';
import type { RequestHandler } from './$types';
import { mintSessionToken, SESSION_COOKIE_NAME, SESSION_COOKIE_OPTIONS } from '$lib/server/session';
import { SiemApiClient, SiemApiError } from '$lib/server/siemApiClient';

export const POST: RequestHandler = async ({ request, cookies }) => {
	const { username, password } = (await request.json()) as { username: string; password: string };

	const apiClient = new SiemApiClient({ baseUrl: env.API_URL as string });
	let session;
	try {
		session = await apiClient.localLogin({ username, password });
	} catch (err) {
		if (err instanceof SiemApiError) {
			return json({ error: 'Invalid username or password.' }, { status: err.status });
		}
		throw err;
	}

	const secret = Buffer.from(env.SESSION_SECRET as string, 'base64');
	const token = await mintSessionToken(
		{
			// No OIDC subject exists for a local admin (store.User.Subject is
			// NULL) - "local:<username>" gives mintSessionToken a stable,
			// non-empty value that can't collide with a real OIDC sub, even
			// though nothing downstream actually reads the sub claim back out
			// today (see hooks.server.ts).
			sub: `local:${username}`,
			userId: session.user_id,
			email: '',
			displayName: session.display_name,
			groups: [],
			role: session.role,
			picture: ''
		},
		secret
	);

	cookies.set(SESSION_COOKIE_NAME, token, SESSION_COOKIE_OPTIONS);
	return json({ ok: true });
};
