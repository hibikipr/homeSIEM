import { redirect, error } from '@sveltejs/kit';
import { env } from '$env/dynamic/private';
import type { RequestHandler } from './$types';
import { completeLogin, PKCE_COOKIE_NAME, STATE_COOKIE_NAME } from '$lib/server/oidc';
import { mintSessionToken, SESSION_COOKIE_NAME, SESSION_COOKIE_OPTIONS } from '$lib/server/session';
import { SiemApiClient, SiemApiError } from '$lib/server/siemApiClient';

export const GET: RequestHandler = async ({ url, cookies }) => {
	const codeVerifier = cookies.get(PKCE_COOKIE_NAME);
	if (!codeVerifier) {
		error(400, 'missing PKCE verifier cookie');
	}
	cookies.delete(PKCE_COOKIE_NAME, { path: '/' });

	const expectedState = cookies.get(STATE_COOKIE_NAME);
	if (!expectedState) {
		error(400, 'missing state cookie');
	}
	cookies.delete(STATE_COOKIE_NAME, { path: '/' });

	const claims = await completeLogin(
		{
			issuer: env.OIDC_ISSUER!,
			clientId: env.OIDC_CLIENT_ID!,
			redirectUri: `${env.APP_URL!}/auth/callback`
		},
		url,
		codeVerifier,
		expectedState
	);

	const apiClient = new SiemApiClient({ baseUrl: env.API_URL! });
	let session;
	try {
		session = await apiClient.establishSession({
			subject: claims.sub,
			email: claims.email,
			display_name: claims.displayName,
			groups: claims.groups
		});
	} catch (err) {
		if (err instanceof SiemApiError) {
			error(err.status, err.message);
		}
		throw err;
	}

	const secret = Buffer.from(env.SESSION_SECRET!, 'base64');
	const token = await mintSessionToken(
		{
			sub: claims.sub,
			userId: session.user_id,
			email: claims.email,
			displayName: session.display_name,
			groups: claims.groups,
			role: session.role
		},
		secret
	);

	cookies.set(SESSION_COOKIE_NAME, token, SESSION_COOKIE_OPTIONS);
	redirect(302, '/');
};
