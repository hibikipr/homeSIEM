import { redirect, type Handle } from '@sveltejs/kit';
import { env } from '$env/dynamic/private';
import { verifySessionToken, SESSION_COOKIE_NAME } from '$lib/server/session';

const PUBLIC_PREFIXES = ['/auth/login', '/auth/callback', '/auth/logout', '/healthz'];

export const handle: Handle = async ({ event, resolve }) => {
	if (
		PUBLIC_PREFIXES.some(
			(prefix) => event.url.pathname === prefix || event.url.pathname.startsWith(prefix + '/')
		)
	) {
		return resolve(event);
	}

	const token = event.cookies.get(SESSION_COOKIE_NAME);
	if (!token) {
		redirect(302, '/auth/login');
	}

	try {
		const secret = Buffer.from(env.SESSION_SECRET!, 'base64');
		const claims = await verifySessionToken(token, secret);
		event.locals.user = {
			userId: claims.userId,
			email: claims.email,
			displayName: claims.displayName,
			groups: claims.groups,
			role: claims.role
		};
		event.locals.sessionToken = token;
	} catch {
		event.cookies.delete(SESSION_COOKIE_NAME, { path: '/' });
		redirect(302, '/auth/login');
	}

	return resolve(event);
};
