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

describe('Search load', () => {
	it('fetches with limit=10000 and returns the search result', async () => {
		const searchMock = vi.fn().mockResolvedValue(fakeSearchResult());
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return { search: searchMock };
		});

		const result = (await load({
			locals: { sessionToken: 'token-123' },
			url: new URL('https://siem.townsville.cc/search')
		} as never)) as Exclude<Awaited<ReturnType<typeof load>>, void>;

		expect(result.count).toBe(1);
		expect(searchMock).toHaveBeenCalledWith(
			'token-123',
			expect.objectContaining({ limit: '10000' })
		);
	});

	it('has no selected entry or context summary when ?preview= is absent', async () => {
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return { search: vi.fn().mockResolvedValue(fakeSearchResult()) };
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
			return { search: searchMock };
		});

		const result = (await load({
			locals: { sessionToken: 'token-123' },
			url: new URL('https://siem.townsville.cc/search?preview=0')
		} as never)) as Exclude<Awaited<ReturnType<typeof load>>, void>;

		expect(result.selectedEntry?.Line).toBe('{"src_ip":"10.0.0.5"}');
		expect(result.contextSummary).toEqual({ count: 4 });
		expect(searchMock).toHaveBeenCalledTimes(2);
	});

	it('redirects to /auth/logout on a 401/403 from the primary search', async () => {
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return { search: vi.fn().mockRejectedValue(new SiemApiError(401, 'invalid session')) };
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
			return { search: vi.fn().mockRejectedValue(new SiemApiError(500, 'boom')) };
		});

		await expect(
			load({
				locals: { sessionToken: 'token-123' },
				url: new URL('https://siem.townsville.cc/search')
			} as never)
		).rejects.toMatchObject({ status: 502 });
	});
});
