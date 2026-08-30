import { describe, it, expect, vi } from 'vitest';
import { handle } from './hooks.server';
import { mintSessionToken, SESSION_COOKIE_NAME } from '$lib/server/session';

vi.mock('$env/dynamic/private', () => ({
	env: { SESSION_SECRET: Buffer.from('0123456789abcdef0123456789abcdef').toString('base64') }
}));

const secret = new TextEncoder().encode('0123456789abcdef0123456789abcdef');

function fakeEvent(pathname: string, cookieValue: string | undefined) {
	const locals: Record<string, unknown> = {};
	return {
		event: {
			url: new URL(`https://siem.townsville.cc${pathname}`),
			cookies: {
				get: () => cookieValue,
				delete: vi.fn()
			},
			locals
		},
		locals
	};
}

describe('handle', () => {
	it('passes through /auth/login without requiring a session', async () => {
		const { event } = fakeEvent('/auth/login', undefined);
		const resolve = vi.fn().mockResolvedValue(new Response('ok'));

		await handle({ event: event as never, resolve });

		expect(resolve).toHaveBeenCalled();
	});

	it('passes through /auth/local-login without requiring a session', async () => {
		const { event } = fakeEvent('/auth/local-login', undefined);
		const resolve = vi.fn().mockResolvedValue(new Response('ok'));

		await handle({ event: event as never, resolve });

		expect(resolve).toHaveBeenCalled();
	});

	it('passes through /healthz without requiring a session', async () => {
		const { event } = fakeEvent('/healthz', undefined);
		const resolve = vi.fn().mockResolvedValue(new Response('ok'));

		await handle({ event: event as never, resolve });

		expect(resolve).toHaveBeenCalled();
	});

	it('redirects to /auth/login when no session cookie is present', async () => {
		const { event } = fakeEvent('/', undefined);
		const resolve = vi.fn();

		await expect(handle({ event: event as never, resolve })).rejects.toMatchObject({
			status: 302,
			location: '/auth/login'
		});
		expect(resolve).not.toHaveBeenCalled();
	});

	it('redirects to /auth/login and clears the cookie when the session token is invalid', async () => {
		const { event } = fakeEvent('/', 'not-a-valid-jwt');
		const resolve = vi.fn();

		await expect(handle({ event: event as never, resolve })).rejects.toMatchObject({ status: 302 });
		expect(event.cookies.delete).toHaveBeenCalledWith(SESSION_COOKIE_NAME, { path: '/' });
	});

	it('attaches locals.user and locals.sessionToken and resolves when the session is valid', async () => {
		const token = await mintSessionToken(
			{
				sub: 'oidc-sub-1',
				userId: 42,
				email: 'alice@townsville.cc',
				displayName: 'Alice',
				groups: ['siem-analysts'],
				role: 'analyst',
				picture: 'https://pocketid.townsville.cc/api/users/oidc-sub-1/profile-picture.png'
			},
			secret
		);
		const { event, locals } = fakeEvent('/', token);
		const resolve = vi.fn().mockResolvedValue(new Response('ok'));

		await handle({ event: event as never, resolve });

		expect(resolve).toHaveBeenCalled();
		expect(locals.user).toMatchObject({
			userId: 42,
			displayName: 'Alice',
			role: 'analyst',
			picture: 'https://pocketid.townsville.cc/api/users/oidc-sub-1/profile-picture.png'
		});
		expect(locals.sessionToken).toBe(token);
	});
});
