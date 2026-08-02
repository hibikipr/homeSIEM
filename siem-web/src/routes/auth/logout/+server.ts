import { redirect } from '@sveltejs/kit';
import { env } from '$env/dynamic/private';
import type { RequestHandler } from './$types';
import { SESSION_COOKIE_NAME } from '$lib/server/session';

export const GET: RequestHandler = async ({ cookies }) => {
  cookies.delete(SESSION_COOKIE_NAME, { path: '/' });
  redirect(302, env.OIDC_LOGOUT_URL!);
};
