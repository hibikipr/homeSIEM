import { redirect } from '@sveltejs/kit';
import { env } from '$env/dynamic/private';
import type { RequestHandler } from './$types';
import { buildLoginRedirect, PKCE_COOKIE_NAME, STATE_COOKIE_NAME } from '$lib/server/oidc';

const SHORT_LIVED_COOKIE_OPTIONS = {
	path: '/',
	httpOnly: true,
	secure: true,
	sameSite: 'lax' as const,
	maxAge: 600
};

export const GET: RequestHandler = async ({ cookies }) => {
	const { url, codeVerifier, state } = await buildLoginRedirect({
		issuer: env.OIDC_ISSUER!,
		clientId: env.OIDC_CLIENT_ID!,
		redirectUri: `${env.APP_URL!}/auth/callback`
	});

	cookies.set(PKCE_COOKIE_NAME, codeVerifier, SHORT_LIVED_COOKIE_OPTIONS);
	cookies.set(STATE_COOKIE_NAME, state, SHORT_LIVED_COOKIE_OPTIONS);

	redirect(302, url);
};
