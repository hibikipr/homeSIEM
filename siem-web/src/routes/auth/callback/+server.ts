import { redirect, error } from '@sveltejs/kit';

import { OIDC_ISSUER, OIDC_CLIENT_ID, APP_URL, API_URL, SESSION_SECRET } from '$app/env/private';

import type { RequestHandler } from './$types';
import { completeLogin, PKCE_COOKIE_NAME, STATE_COOKIE_NAME } from '#lib/server/oidc.ts';
import {
	mintSessionToken,
	SESSION_COOKIE_NAME,
	SESSION_COOKIE_OPTIONS
} from '#lib/server/session.ts';
import { SiemApiClient, SiemApiError } from '#lib/server/siemApiClient.ts';

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
			issuer: OIDC_ISSUER!,
			clientId: OIDC_CLIENT_ID!,
			redirectUri: `${APP_URL!}/auth/callback`
		},
		url,
		codeVerifier,
		expectedState
	);

	const apiClient = new SiemApiClient({ baseUrl: API_URL! });
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

	const secret = Buffer.from(SESSION_SECRET!, 'base64');
	const token = await mintSessionToken(
		{
			sub: claims.sub,
			userId: session.user_id,
			email: claims.email,
			displayName: session.display_name,
			groups: claims.groups,
			role: session.role,
			picture: claims.picture
		},
		secret
	);

	cookies.set(SESSION_COOKIE_NAME, token, SESSION_COOKIE_OPTIONS);
	redirect(302, '/');
};
