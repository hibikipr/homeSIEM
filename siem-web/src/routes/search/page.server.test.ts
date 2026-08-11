import { describe, it, expect, vi } from 'vitest';
import { load } from './+page.server';
import * as siemApiClientModule from '$lib/server/siemApiClient';
import { SiemApiError } from '$lib/server/siemApiClient';

vi.mock('$env/dynamic/private', () => ({ env: { API_URL: 'http://siem-api:8080' } }));

vi.mock('$lib/server/siemApiClient', async (importOriginal) => {
	const actual = await importOriginal<typeof import('$lib/server/siemApiClient')>();
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

function fakeGetSources() {
	return vi.fn().mockResolvedValue([
		{ id: 1, name: 'udm-ultra', claimed: true },
		{ id: 2, name: 'unclaimed-host', claimed: false }
	]);
}

describe('Search load', () => {
	it("fetches with limit=1000 (matching siem-api's own default) and returns the search result", async () => {
		const searchMock = vi.fn().mockResolvedValue(fakeSearchResult());
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return { search: searchMock, getSources: fakeGetSources() };
		});

		const result = (await load({
			locals: { sessionToken: 'token-123' },
			url: new URL('https://siem.townsville.cc/search')
		} as never)) as Exclude<Awaited<ReturnType<typeof load>>, void>;

		expect(result.count).toBe(1);
		expect(searchMock).toHaveBeenCalledWith(
			'token-123',
			expect.objectContaining({ limit: '1000' })
		);
		// fakeGetSources() returns one claimed and one unclaimed source -
		// only the claimed one's name should survive into page data.
		expect(result.claimedSourceNames).toEqual(['udm-ultra']);
	});

	it('degrades to an empty claimedSourceNames list when the sources lookup fails', async () => {
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return {
				search: vi.fn().mockResolvedValue(fakeSearchResult()),
				getSources: vi.fn().mockRejectedValue(new Error('boom'))
			};
		});

		const result = (await load({
			locals: { sessionToken: 'token-123' },
			url: new URL('https://siem.townsville.cc/search')
		} as never)) as Exclude<Awaited<ReturnType<typeof load>>, void>;

		expect(result.claimedSourceNames).toEqual([]);
	});

	it('has no selected entry or context summary when ?preview= is absent', async () => {
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return {
				search: vi.fn().mockResolvedValue(fakeSearchResult()),
				getSources: fakeGetSources()
			};
		});

		const result = (await load({
			locals: { sessionToken: 'token-123' },
			url: new URL('https://siem.townsville.cc/search')
		} as never)) as Exclude<Awaited<ReturnType<typeof load>>, void>;

		expect(result.selectedEntry).toBeNull();
		expect(result.contextSummary).toBeNull();
	});

	it('resolves the selected entry from ?preview= and fetches a context summary when src_ip is present', async () => {
		const searchMock = vi
			.fn()
			.mockResolvedValueOnce(
				fakeSearchResult({
					entries: [
						{
							Timestamp: '2026-08-05T00:00:00Z',
							Labels: { severity: 'critical' },
							Line: '{"src_ip":"10.0.0.5"}'
						}
					]
				})
			)
			.mockResolvedValueOnce(fakeSearchResult({ count: 4, entries: [] }));
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return { search: searchMock, getSources: fakeGetSources() };
		});

		const result = (await load({
			locals: { sessionToken: 'token-123' },
			url: new URL('https://siem.townsville.cc/search?preview=0')
		} as never)) as Exclude<Awaited<ReturnType<typeof load>>, void>;

		expect(result.selectedEntry?.Line).toBe('{"src_ip":"10.0.0.5"}');
		expect(result.contextSummary).toEqual({ count: 4 });
		expect(searchMock).toHaveBeenCalledTimes(2);
		expect(searchMock).toHaveBeenNthCalledWith(
			2,
			'token-123',
			expect.objectContaining({ entries: 'false', volume: 'false' })
		);
	});

	it('resolves previewIndex to null when ?preview= is non-numeric', async () => {
		const searchMock = vi.fn().mockResolvedValue(fakeSearchResult());
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return { search: searchMock, getSources: fakeGetSources() };
		});

		const result = (await load({
			locals: { sessionToken: 'token-123' },
			url: new URL('https://siem.townsville.cc/search?preview=abc')
		} as never)) as Exclude<Awaited<ReturnType<typeof load>>, void>;

		expect(result.previewIndex).toBeNull();
		expect(result.selectedEntry).toBeNull();
		expect(result.contextSummary).toBeNull();
		expect(searchMock).toHaveBeenCalledTimes(1);
	});

	it('redirects to /auth/logout on a 401/403 from the primary search', async () => {
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return {
				search: vi.fn().mockRejectedValue(new SiemApiError(401, 'invalid session')),
				getSources: fakeGetSources()
			};
		});

		await expect(
			load({
				locals: { sessionToken: 'stale-token' },
				url: new URL('https://siem.townsville.cc/search')
			} as never)
		).rejects.toMatchObject({ status: 302, location: '/auth/logout' });
	});

	it('surfaces a 502 when siem-api fails for a reason other than auth', async () => {
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return {
				search: vi.fn().mockRejectedValue(new SiemApiError(500, 'boom')),
				getSources: fakeGetSources()
			};
		});

		await expect(
			load({
				locals: { sessionToken: 'token-123' },
				url: new URL('https://siem.townsville.cc/search')
			} as never)
		).rejects.toMatchObject({ status: 502 });
	});
});
