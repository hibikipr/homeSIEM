import { describe, it, expect, vi, beforeEach } from 'vitest';
import { GET } from './+server';
import * as oidc from '$lib/server/oidc';
import * as siemApiClientModule from '$lib/server/siemApiClient';

vi.mock('$env/dynamic/private', () => ({
	env: {
		OIDC_ISSUER: 'https://pocketid.townsville.cc',
		OIDC_CLIENT_ID: 'homeSIEM',
		APP_URL: 'https://siem.townsville.cc',
		API_URL: 'http://siem-api:8080',
		SESSION_SECRET: Buffer.from('0123456789abcdef0123456789abcdef').toString('base64')
	}
}));

vi.mock('$lib/server/oidc', async (importOriginal) => {
	const actual = await importOriginal<typeof import('$lib/server/oidc')>();
	return { ...actual, completeLogin: vi.fn() };
});

vi.mock('$lib/server/siemApiClient', async (importOriginal) => {
	const actual = await importOriginal<typeof import('$lib/server/siemApiClient')>();
	return { ...actual, SiemApiClient: vi.fn() };
});

function fakeEvent(pkceCookie: string | undefined, stateCookie: string | undefined) {
	const cookieStore = new Map<string, string>();
	if (pkceCookie) cookieStore.set(oidc.PKCE_COOKIE_NAME, pkceCookie);
	if (stateCookie) cookieStore.set(oidc.STATE_COOKIE_NAME, stateCookie);
	return {
		url: new URL('https://siem.townsville.cc/auth/callback?code=abc&state=xyz'),
		cookies: {
			get: (name: string) => cookieStore.get(name),
			delete: vi.fn((name: string) => cookieStore.delete(name)),
			set: vi.fn((name: string, value: string) => cookieStore.set(name, value))
		}
	};
}

describe('GET /auth/callback', () => {
	beforeEach(() => {
		vi.mocked(oidc.completeLogin).mockResolvedValue({
			sub: 'oidc-sub-1',
			email: 'alice@townsville.cc',
			displayName: 'Alice',
			groups: ['siem-analysts']
		});
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return {
				establishSession: vi
					.fn()
					.mockResolvedValue({ user_id: 7, role: 'analyst', display_name: 'Alice' })
			} as never;
		});
	});

	it('errors with 400 when the PKCE cookie is missing', async () => {
		const event = fakeEvent(undefined, 'state-xyz');
		await expect(GET(event as never)).rejects.toMatchObject({ status: 400 });
	});

	it('errors with 400 when the state cookie is missing', async () => {
		const event = fakeEvent('verifier-abc', undefined);
		await expect(GET(event as never)).rejects.toMatchObject({ status: 400 });
	});

	it('sets the session cookie, clears the PKCE/state cookies, and redirects to / on success', async () => {
		const event = fakeEvent('verifier-abc', 'state-xyz');

		await expect(GET(event as never)).rejects.toMatchObject({ status: 302, location: '/' });

		expect(event.cookies.set).toHaveBeenCalledWith(
			'siem_session',
			expect.any(String),
			expect.objectContaining({ httpOnly: true })
		);
		expect(event.cookies.delete).toHaveBeenCalledWith(oidc.PKCE_COOKIE_NAME, { path: '/' });
		expect(event.cookies.delete).toHaveBeenCalledWith(oidc.STATE_COOKIE_NAME, { path: '/' });
		expect(oidc.completeLogin).toHaveBeenCalledWith(
			expect.anything(),
			expect.anything(),
			'verifier-abc',
			'state-xyz'
		);
	});

	it('propagates an error when siem-api denies session establishment', async () => {
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return {
				establishSession: vi
					.fn()
					.mockRejectedValue(new siemApiClientModule.SiemApiError(403, 'denied'))
			} as never;
		});
		const event = fakeEvent('verifier-abc', 'state-xyz');

		await expect(GET(event as never)).rejects.toMatchObject({ status: 403 });
	});
});
