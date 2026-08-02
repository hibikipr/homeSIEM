import { redirect } from '@sveltejs/kit';
import { env } from '$env/dynamic/private';
import type { RequestHandler } from './$types';
import { buildLoginRedirect, PKCE_COOKIE_NAME } from '$lib/server/oidc';

export const GET: RequestHandler = async ({ cookies }) => {
  const { url, codeVerifier } = await buildLoginRedirect({
    issuer: env.OIDC_ISSUER!,
    clientId: env.OIDC_CLIENT_ID!,
    redirectUri: `${env.APP_URL!}/auth/callback`
  });

  cookies.set(PKCE_COOKIE_NAME, codeVerifier, {
    path: '/',
    httpOnly: true,
    secure: true,
    sameSite: 'lax',
    maxAge: 600
  });

  redirect(302, url);
};
