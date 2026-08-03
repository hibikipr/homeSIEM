import { describe, it, expect, vi } from 'vitest';
import { load } from './+page.server';
import * as siemApiClientModule from '$lib/server/siemApiClient';
import { SiemApiError } from '$lib/server/siemApiClient';

vi.mock('$env/dynamic/private', () => ({ env: { API_URL: 'http://siem-api:8080' } }));

vi.mock('$lib/server/siemApiClient', async (importOriginal) => {
	const actual = await importOriginal<typeof import('$lib/server/siemApiClient')>();
	return { ...actual, SiemApiClient: vi.fn() };
});

describe('Wall load', () => {
	it('shapes siem-api responses into WallPageData', async () => {
		const searchMock = vi.fn().mockResolvedValue({
			logql: '{job="siem"}',
			count: 1,
			entries: [{ Timestamp: '2026-08-02T00:00:00Z', Labels: {}, Line: '{"geoip":{"cc":"US"}}' }]
		});
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return {
				getEventsStats: vi.fn().mockResolvedValue({
					event_count_24h: 1240000,
					heat_grid: [{ source: 'udm-ultra', hours: ['critical', 'none'] }]
				}),
				getAlerts: vi.fn().mockResolvedValue([
					{
						id: 1,
						rule_id: 1,
						group_key: 'a',
						severity: 'critical',
						title: 't',
						body: 'b',
						event_count: 1,
						state: 'open',
						first_seen_at: '2026-08-02T00:00:00Z',
						last_seen_at: '2026-08-02T00:00:00Z'
					}
				]),
				search: searchMock
			};
		});

		const result = (await load({ locals: { sessionToken: 'token-123' } } as never)) as Exclude<
			Awaited<ReturnType<typeof load>>,
			void
		>;

		expect(result.eventCount24h).toBe(1240000);
		expect(result.heatGrid).toEqual([{ source: 'udm-ultra', hours: ['critical', 'none'] }]);
		expect(result.openAlertCount).toBe(1);
		expect(result.triageAlerts).toHaveLength(1);
		expect(result.countryBreakdown).toEqual([{ country: 'US', count: 1 }]);
		expect(searchMock).toHaveBeenCalledWith('token-123', { limit: '1000' });
	});

	it('redirects to /auth/logout when siem-api rejects the session with 401', async () => {
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return {
				getEventsStats: vi.fn().mockRejectedValue(new SiemApiError(401, 'invalid session')),
				getAlerts: vi.fn().mockResolvedValue([]),
				search: vi.fn().mockResolvedValue({ logql: '', count: 0, entries: [] })
			};
		});

		await expect(load({ locals: { sessionToken: 'stale-token' } } as never)).rejects.toMatchObject({
			status: 302,
			location: '/auth/logout'
		});
	});

	it('redirects to /auth/logout when siem-api rejects the session with 403', async () => {
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return {
				getEventsStats: vi.fn().mockResolvedValue({ event_count_24h: 0, heat_grid: [] }),
				getAlerts: vi.fn().mockRejectedValue(new SiemApiError(403, 'role no longer valid')),
				search: vi.fn().mockResolvedValue({ logql: '', count: 0, entries: [] })
			};
		});

		await expect(
			load({ locals: { sessionToken: 'demoted-token' } } as never)
		).rejects.toMatchObject({ status: 302, location: '/auth/logout' });
	});

	it('surfaces a 502 when siem-api fails for a reason other than auth', async () => {
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return {
				getEventsStats: vi.fn().mockResolvedValue({ event_count_24h: 0, heat_grid: [] }),
				getAlerts: vi.fn().mockResolvedValue([]),
				search: vi.fn().mockRejectedValue(new SiemApiError(500, 'boom'))
			};
		});

		await expect(load({ locals: { sessionToken: 'token-123' } } as never)).rejects.toMatchObject({
			status: 502
		});
	});
});
