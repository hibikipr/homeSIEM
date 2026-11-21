import { describe, it, expect, vi, afterEach } from 'vitest';
import { load } from './+page.server';
import * as siemApiClientModule from '#lib/server/siemApiClient.ts';
import { SiemApiError } from '#lib/server/siemApiClient.ts';

vi.mock('$app/env/private', () => ({ API_URL: 'http://siem-api:8080' }));

vi.mock('#lib/server/siemApiClient.ts', async (importOriginal) => {
	const actual = await importOriginal<typeof import('#lib/server/siemApiClient.ts')>();
	return { ...actual, SiemApiClient: vi.fn() };
});

function fakeSearchResult(overrides: Record<string, unknown> = {}) {
	return {
		logql: '{job="siem"}',
		count: 1,
		entries: [{ Timestamp: '2026-08-05T00:00:00Z', Labels: { severity: 'info' }, Line: '{}' }],
		volume: [],
		...overrides
	};
}

function fakeGetAlerts() {
	return vi.fn().mockResolvedValue([]);
}

afterEach(() => {
	vi.useRealTimers();
});

function fakeGetSources() {
	return vi.fn().mockResolvedValue([
		{ id: 1, name: 'udm-ultra', display_name: '', claimed: true },
		{ id: 2, name: 'unclaimed-host', display_name: '', claimed: false }
	]);
}

describe('Search load', () => {
	it("fetches with limit=1000 (matching siem-api's own default) and returns the search result", async () => {
		const searchMock = vi.fn().mockResolvedValue(fakeSearchResult());
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return { search: searchMock, getSources: fakeGetSources(), getAlerts: fakeGetAlerts() };
		});

		const result = (await load({
			depends: () => {},
			locals: { sessionToken: 'token-123' },
			url: new URL('https://siem.townsville.cc/search')
		} as never)) as Exclude<Awaited<ReturnType<typeof load>>, void>;

		const results = await result.results;
		expect(results?.count).toBe(1);
		expect(searchMock).toHaveBeenCalledWith(
			'token-123',
			expect.objectContaining({ limit: '1000' })
		);
		// fakeGetSources() returns one claimed and one unclaimed source -
		// only the claimed one should survive into page data. claimedSources
		// is streamed (not awaited before load() returns - see the
		// module-level comment on it), hence the extra await here.
		await expect(result.claimedSources).resolves.toEqual([{ name: 'udm-ultra', displayName: '' }]);
	});

	it('carries display_name through as displayName for the Source facet to use', async () => {
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return {
				search: vi.fn().mockResolvedValue(fakeSearchResult()),
				getAlerts: fakeGetAlerts(),
				getSources: vi
					.fn()
					.mockResolvedValue([
						{ id: 7, name: '192.168.3.223', display_name: 'Home Assistant', claimed: true }
					])
			};
		});

		const result = (await load({
			depends: () => {},
			locals: { sessionToken: 'token-123' },
			url: new URL('https://siem.townsville.cc/search')
		} as never)) as Exclude<Awaited<ReturnType<typeof load>>, void>;

		await expect(result.claimedSources).resolves.toEqual([
			{ name: '192.168.3.223', displayName: 'Home Assistant' }
		]);
	});

	it('degrades to an empty claimedSources list when the sources lookup fails', async () => {
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return {
				search: vi.fn().mockResolvedValue(fakeSearchResult()),
				getAlerts: fakeGetAlerts(),
				getSources: vi.fn().mockRejectedValue(new Error('boom'))
			};
		});

		const result = (await load({
			depends: () => {},
			locals: { sessionToken: 'token-123' },
			url: new URL('https://siem.townsville.cc/search')
		} as never)) as Exclude<Awaited<ReturnType<typeof load>>, void>;

		await expect(result.claimedSources).resolves.toEqual([]);
	});

	it('never reads ?preview=, so selecting a row does not rerun the Loki search', async () => {
		const searchMock = vi.fn().mockResolvedValue(fakeSearchResult());
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return { search: searchMock, getSources: fakeGetSources(), getAlerts: fakeGetAlerts() };
		});
		const url = new URL('https://siem.townsville.cc/search?preview=0');
		const getSpy = vi.spyOn(url.searchParams, 'get');

		const result = (await load({
			depends: () => {},
			locals: { sessionToken: 'token-123' },
			url
		} as never)) as Exclude<Awaited<ReturnType<typeof load>>, void>;

		expect(getSpy).not.toHaveBeenCalledWith('preview');
		expect(result).not.toHaveProperty('selectedEntry');
		expect(result).not.toHaveProperty('contextSummary');
		expect(searchMock).toHaveBeenCalledTimes(1);
	});

	it('redirects to /auth/logout on a 401/403 from the auth-gating call', async () => {
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return {
				search: vi.fn().mockRejectedValue(new SiemApiError(401, 'invalid session')),
				getSources: fakeGetSources(),
				getAlerts: vi.fn().mockRejectedValue(new SiemApiError(401, 'invalid session'))
			};
		});

		await expect(
			load({
				depends: () => {},
				locals: { sessionToken: 'stale-token' },
				url: new URL('https://siem.townsville.cc/search')
			} as never)
		).rejects.toMatchObject({ status: 302, location: '/auth/logout' });
	});

	it('surfaces a 502 when the auth-gating call fails for a reason other than auth', async () => {
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return {
				search: vi.fn().mockResolvedValue(fakeSearchResult()),
				getSources: fakeGetSources(),
				getAlerts: vi.fn().mockRejectedValue(new SiemApiError(500, 'boom'))
			};
		});

		await expect(
			load({
				depends: () => {},
				locals: { sessionToken: 'token-123' },
				url: new URL('https://siem.townsville.cc/search')
			} as never)
		).rejects.toMatchObject({ status: 502 });
	});

	it('does not throw from load when the slow search fails; results resolves to null', async () => {
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return {
				search: vi.fn().mockRejectedValue(new SiemApiError(500, 'loki timeout')),
				getSources: fakeGetSources(),
				getAlerts: fakeGetAlerts()
			};
		});

		const result = (await load({
			depends: () => {},
			locals: { sessionToken: 'token-123' },
			url: new URL('https://siem.townsville.cc/search')
		} as never)) as Exclude<Awaited<ReturnType<typeof load>>, void>;

		expect(result.filters).toBeDefined();
		await expect(result.results).resolves.toBeNull();
	});

	it('sends identical start/end for two loads within the same 30s window', async () => {
		const searchMock = vi.fn().mockResolvedValue(fakeSearchResult());
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return { search: searchMock, getSources: fakeGetSources(), getAlerts: fakeGetAlerts() };
		});
		const run = () =>
			load({
				depends: () => {},
				locals: { sessionToken: 'token-123' },
				url: new URL('https://siem.townsville.cc/search')
			} as never);

		vi.useFakeTimers();
		vi.setSystemTime(new Date('2026-11-21T12:00:01.234Z'));
		await run();
		vi.setSystemTime(new Date('2026-11-21T12:00:24.987Z'));
		await run();

		const [first, second] = searchMock.mock.calls.map((c) => c[1]);
		expect(first.start).toBe(second.start);
		expect(first.end).toBe(second.end);
		// end is rounded UP to the next 30s boundary, start = end - range (24h).
		expect(first.end).toBe('2026-11-21T12:00:30.000Z');
		expect(first.start).toBe('2026-11-20T12:00:30.000Z');
	});
});
