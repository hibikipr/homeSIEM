import { describe, it, expect, vi } from 'vitest';
import { GET } from './+server';
import * as oidc from '$lib/server/oidc';

vi.mock('$env/dynamic/private', () => ({
	env: {
		OIDC_ISSUER: 'https://pocketid.townsville.cc',
		OIDC_CLIENT_ID: 'homeSIEM',
		APP_URL: 'https://siem.townsville.cc'
	}
}));

vi.mock('$lib/server/oidc', async (importOriginal) => {
	const actual = await importOriginal<typeof import('$lib/server/oidc')>();
	return { ...actual, buildLoginRedirect: vi.fn() };
});

describe('GET /auth/login', () => {
	it('redirects to the OIDC authorization URL and sets the PKCE cookie', async () => {
		vi.mocked(oidc.buildLoginRedirect).mockResolvedValue({
			url: 'https://pocketid.townsville.cc/authorize?state=abc',
			codeVerifier: 'verifier-abc'
		});
		const setCookie = vi.fn();

		await expect(GET({ cookies: { set: setCookie } } as never)).rejects.toMatchObject({
			status: 302,
			location: 'https://pocketid.townsville.cc/authorize?state=abc'
		});

		expect(setCookie).toHaveBeenCalledWith(
			oidc.PKCE_COOKIE_NAME,
			'verifier-abc',
			expect.objectContaining({ httpOnly: true, maxAge: 600 })
		);
	});
});
