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
			entries: [],
			facets: { country: [{ value: 'US', count: 1 }] }
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
				search: searchMock,
				getSources: vi.fn().mockResolvedValue([]),
				getInsights: vi.fn().mockResolvedValue([
					{
						id: 1,
						created_at: '2026-08-10T00:00:00Z',
						title: 'Bambuddy errors look mistagged',
						detail: 'd',
						severity: 'warning',
						category: 'severity-misclassification',
						evidence: [],
						dismissed: false
					}
				])
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
		// countryBreakdown, insights, and sourceLabels are streamed - not
		// awaited before the load function returns, so the page can render
		// the above (stats/alerts-derived) content immediately instead of
		// waiting on these supplementary lookups too. See wall/+page.svelte's
		// {#await} blocks for the client side of this.
		await expect(result.countryBreakdown).resolves.toEqual([{ country: 'US', count: 1 }]);
		expect(searchMock).toHaveBeenCalledWith('token-123', {
			entries: 'false',
			volume: 'false',
			facets: 'true'
		});
		const insights = await result.insights;
		expect(insights).toHaveLength(1);
		expect(insights[0].title).toBe('Bambuddy errors look mistagged');
		await expect(result.sourceLabels).resolves.toEqual({});
	});

	it('builds sourceLabels from claimed sources so HeatGrid can show a rename', async () => {
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return {
				getEventsStats: vi.fn().mockResolvedValue({
					event_count_24h: 0,
					heat_grid: [{ source: '192.168.3.223', hours: ['none'] }]
				}),
				getAlerts: vi.fn().mockResolvedValue([]),
				search: vi.fn().mockResolvedValue({ logql: '', count: 0, entries: [] }),
				getSources: vi.fn().mockResolvedValue([
					{ id: 7, name: '192.168.3.223', display_name: 'Home Assistant', claimed: true },
					{ id: 8, name: 'unclaimed-host', display_name: '', claimed: false }
				]),
				getInsights: vi.fn().mockResolvedValue([])
			};
		});

		const result = (await load({ locals: { sessionToken: 'token-123' } } as never)) as Exclude<
			Awaited<ReturnType<typeof load>>,
			void
		>;

		// Only the claimed source contributes a label; an unclaimed one
		// (not yet vetted by an admin) is deliberately left out, same as
		// the Search page's claimedSources.
		await expect(result.sourceLabels).resolves.toEqual({ '192.168.3.223': 'Home Assistant' });
	});

	it('degrades to an empty insights array without throwing when the insights lookup fails', async () => {
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return {
				getEventsStats: vi.fn().mockResolvedValue({ event_count_24h: 0, heat_grid: [] }),
				getAlerts: vi.fn().mockResolvedValue([]),
				search: vi.fn().mockResolvedValue({ logql: '', count: 0, entries: [] }),
				getSources: vi.fn().mockResolvedValue([]),
				getInsights: vi.fn().mockRejectedValue(new SiemApiError(400, 'insights is not configured'))
			};
		});

		const result = (await load({ locals: { sessionToken: 'token-123' } } as never)) as Exclude<
			Awaited<ReturnType<typeof load>>,
			void
		>;

		await expect(result.insights).resolves.toEqual([]);
	});

	it('degrades to an empty countryBreakdown without throwing when the search lookup fails', async () => {
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return {
				getEventsStats: vi.fn().mockResolvedValue({ event_count_24h: 0, heat_grid: [] }),
				getAlerts: vi.fn().mockResolvedValue([]),
				search: vi.fn().mockRejectedValue(new SiemApiError(500, 'loki unavailable')),
				getSources: vi.fn().mockResolvedValue([]),
				getInsights: vi.fn().mockResolvedValue([])
			};
		});

		const result = (await load({ locals: { sessionToken: 'token-123' } } as never)) as Exclude<
			Awaited<ReturnType<typeof load>>,
			void
		>;

		await expect(result.countryBreakdown).resolves.toEqual([]);
	});

	it('redirects to /auth/logout when siem-api rejects the session with 401', async () => {
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return {
				getEventsStats: vi.fn().mockRejectedValue(new SiemApiError(401, 'invalid session')),
				getAlerts: vi.fn().mockResolvedValue([]),
				search: vi.fn().mockResolvedValue({ logql: '', count: 0, entries: [] }),
				getSources: vi.fn().mockResolvedValue([]),
				getInsights: vi.fn().mockResolvedValue([])
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
				search: vi.fn().mockResolvedValue({ logql: '', count: 0, entries: [] }),
				getSources: vi.fn().mockResolvedValue([]),
				getInsights: vi.fn().mockResolvedValue([])
			};
		});

		await expect(
			load({ locals: { sessionToken: 'demoted-token' } } as never)
		).rejects.toMatchObject({ status: 302, location: '/auth/logout' });
	});

	it('surfaces a 502 when siem-api fails for a reason other than auth', async () => {
		// getEventsStats/getAlerts are the only calls still on the blocking
		// path (see the streamed-data comment above) - search/getSources/
		// getInsights failures degrade gracefully instead of surfacing here,
		// covered by their own "degrades to empty ... " tests.
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return {
				getEventsStats: vi.fn().mockResolvedValue({ event_count_24h: 0, heat_grid: [] }),
				getAlerts: vi.fn().mockRejectedValue(new SiemApiError(500, 'boom')),
				search: vi.fn().mockResolvedValue({ logql: '', count: 0, entries: [] }),
				getSources: vi.fn().mockResolvedValue([]),
				getInsights: vi.fn().mockResolvedValue([])
			};
		});

		await expect(load({ locals: { sessionToken: 'token-123' } } as never)).rejects.toMatchObject({
			status: 502
		});
	});
});
