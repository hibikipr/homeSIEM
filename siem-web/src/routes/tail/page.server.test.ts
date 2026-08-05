import { describe, it, expect, vi } from 'vitest';
import { load } from './+page.server';
import * as siemApiClientModule from '$lib/server/siemApiClient';
import { SiemApiError } from '$lib/server/siemApiClient';

vi.mock('$env/dynamic/private', () => ({ env: { API_URL: 'http://siem-api:8080' } }));

vi.mock('$lib/server/siemApiClient', async (importOriginal) => {
	const actual = await importOriginal<typeof import('$lib/server/siemApiClient')>();
	return { ...actual, SiemApiClient: vi.fn() };
});

describe('Live tail load', () => {
	it('returns the source count', async () => {
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return {
				getSources: vi.fn().mockResolvedValue([{ id: 1 }, { id: 2 }, { id: 3 }])
			};
		});

		const result = (await load({
			locals: { sessionToken: 'token-123' }
		} as never)) as Exclude<Awaited<ReturnType<typeof load>>, void>;

		expect(result.sourceCount).toBe(3);
	});

	it('redirects to /auth/logout on a 401/403 from siem-api', async () => {
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return {
				getSources: vi.fn().mockRejectedValue(new SiemApiError(401, 'invalid session'))
			};
		});

		await expect(load({ locals: { sessionToken: 'stale-token' } } as never)).rejects.toMatchObject({
			status: 302,
			location: '/auth/logout'
		});
	});

	it('surfaces a 502 when siem-api fails for a reason other than auth', async () => {
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return {
				getSources: vi.fn().mockRejectedValue(new SiemApiError(500, 'boom'))
			};
		});

		await expect(load({ locals: { sessionToken: 'token-123' } } as never)).rejects.toMatchObject({
			status: 502
		});
	});
});
