import { redirect } from '@sveltejs/kit';
import { OIDC_LOGOUT_URL } from '$app/env/private';
import type { RequestHandler } from './$types';
import { SESSION_COOKIE_NAME } from '#lib/server/session.ts';

export const GET: RequestHandler = async ({ cookies }) => {
	cookies.delete(SESSION_COOKIE_NAME, { path: '/' });
	redirect(302, OIDC_LOGOUT_URL!, { external: true });
};
