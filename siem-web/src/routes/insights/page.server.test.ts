import { describe, it, expect, vi } from 'vitest';
import { load } from './+page.server';
import * as siemApiClientModule from '$lib/server/siemApiClient';
import { SiemApiError } from '$lib/server/siemApiClient';

vi.mock('$env/dynamic/private', () => ({ env: { API_URL: 'http://siem-api:8080' } }));

vi.mock('$lib/server/siemApiClient', async (importOriginal) => {
	const actual = await importOriginal<typeof import('$lib/server/siemApiClient')>();
	return { ...actual, SiemApiClient: vi.fn() };
});

describe('Insights load', () => {
	it('loads both insights and muted fingerprints', async () => {
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return {
				getInsights: vi.fn().mockResolvedValue([{ id: 1, title: 't', dismissed: false }]),
				listMutedInsights: vi
					.fn()
					.mockResolvedValue([
						{
							fingerprint: 'abc123',
							category: 'operational',
							programs: 'UI-poller',
							example_title: 'UI-poller repeated errors',
							muted_at: 'x'
						}
					])
			};
		});

		const result = (await load({
			locals: { sessionToken: 'token-123' }
		} as never)) as Exclude<Awaited<ReturnType<typeof load>>, void>;

		expect(result.insights).toHaveLength(1);
		expect(result.mutedInsights).toHaveLength(1);
		expect(result.mutedInsights[0].fingerprint).toBe('abc123');
	});

	it('redirects to /auth/logout on a 401/403 from the insights fetch', async () => {
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return {
				getInsights: vi.fn().mockRejectedValue(new SiemApiError(401, 'invalid session')),
				listMutedInsights: vi.fn().mockResolvedValue([])
			};
		});

		await expect(
			load({ locals: { sessionToken: 'stale-token' } } as never)
		).rejects.toMatchObject({ status: 302, location: '/auth/logout' });
	});

	it('surfaces a 502 when siem-api fails for a reason other than auth', async () => {
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return {
				getInsights: vi.fn().mockRejectedValue(new SiemApiError(500, 'boom')),
				listMutedInsights: vi.fn().mockResolvedValue([])
			};
		});

		await expect(load({ locals: { sessionToken: 'token-123' } } as never)).rejects.toMatchObject({
			status: 502
		});
	});
});
