import { describe, it, expect, vi, beforeEach } from 'vitest';
import { POST } from './+server';
import * as siemApiClientModule from '$lib/server/siemApiClient';

vi.mock('$env/dynamic/private', () => ({
	env: {
		API_URL: 'http://siem-api:8080',
		SESSION_SECRET: Buffer.from('0123456789abcdef0123456789abcdef').toString('base64')
	}
}));

vi.mock('$lib/server/siemApiClient', async (importOriginal) => {
	const actual = await importOriginal<typeof import('$lib/server/siemApiClient')>();
	return { ...actual, SiemApiClient: vi.fn() };
});

function fakeEvent(body: unknown) {
	const cookieStore = new Map<string, string>();
	return {
		request: new Request('https://siem.townsville.cc/auth/local-login', {
			method: 'POST',
			body: JSON.stringify(body)
		}),
		cookies: {
			set: vi.fn((name: string, value: string) => cookieStore.set(name, value)),
			get: (name: string) => cookieStore.get(name)
		}
	};
}

describe('POST /auth/local-login', () => {
	beforeEach(() => {
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return {
				localLogin: vi
					.fn()
					.mockResolvedValue({ user_id: 1, role: 'admin', display_name: 'Local Admin' })
			} as never;
		});
	});

	it('sets a signed session cookie and returns ok on valid credentials', async () => {
		const event = fakeEvent({ username: 'admin', password: 'correct-horse' });

		const response = await POST(event as never);

		expect(response.status).toBe(200);
		expect(event.cookies.set).toHaveBeenCalledWith(
			'siem_session',
			expect.any(String),
			expect.objectContaining({ httpOnly: true })
		);
	});

	it('returns the SiemApiError status with a generic message on invalid credentials', async () => {
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return {
				localLogin: vi
					.fn()
					.mockRejectedValue(new siemApiClientModule.SiemApiError(401, 'invalid credentials'))
			} as never;
		});
		const event = fakeEvent({ username: 'admin', password: 'wrong' });

		const response = await POST(event as never);

		expect(response.status).toBe(401);
		expect(event.cookies.set).not.toHaveBeenCalled();
		const body = await response.json();
		expect(body.error).toBe('Invalid username or password.');
	});
});
