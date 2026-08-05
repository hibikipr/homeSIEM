import { describe, it, expect, vi } from 'vitest';
import { load } from './+page.server';
import * as siemApiClientModule from '$lib/server/siemApiClient';
import { SiemApiError } from '$lib/server/siemApiClient';

vi.mock('$env/dynamic/private', () => ({ env: { API_URL: 'http://siem-api:8080' } }));

vi.mock('$lib/server/siemApiClient', async (importOriginal) => {
	const actual = await importOriginal<typeof import('$lib/server/siemApiClient')>();
	return { ...actual, SiemApiClient: vi.fn() };
});

function fakeSource(overrides: Record<string, unknown> = {}) {
	return {
		id: 1,
		name: 'udm-ultra',
		address: '10.0.0.1',
		transport: 'udp/514',
		parser: 'unifi-os',
		claimed: true,
		heartbeat_sec: 900,
		status: 'healthy',
		events_per_min: 5,
		...overrides
	};
}

function fakeHealth() {
	return { received_events_per_source: {}, loki_sent_events_total: 0, degraded: false };
}

describe('Sources load', () => {
	it('loads sources, health, and a preview sample for the first source by default', async () => {
		const searchMock = vi.fn().mockResolvedValue({ logql: '', count: 1, entries: [{ Line: '{}' }] });
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return {
				getSources: vi.fn().mockResolvedValue([fakeSource({ name: 'udm-ultra' })]),
				getIngestHealth: vi.fn().mockResolvedValue(fakeHealth()),
				search: searchMock
			};
		});

		const result = (await load({
			locals: { sessionToken: 'token-123' },
			url: new URL('https://siem.townsville.cc/sources')
		} as never)) as Exclude<Awaited<ReturnType<typeof load>>, void>;

		expect(result.previewName).toBe('udm-ultra');
		expect(result.previewSample).toEqual({ Line: '{}' });
		expect(searchMock).toHaveBeenCalledWith('token-123', { source: 'udm-ultra', limit: '1' });
	});

	it('uses the ?preview= query param over the first source when given', async () => {
		const searchMock = vi.fn().mockResolvedValue({ logql: '', count: 0, entries: [] });
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return {
				getSources: vi.fn().mockResolvedValue([
					fakeSource({ id: 1, name: 'udm-ultra' }),
					fakeSource({ id: 2, name: 'host-1' })
				]),
				getIngestHealth: vi.fn().mockResolvedValue(fakeHealth()),
				search: searchMock
			};
		});

		const result = (await load({
			locals: { sessionToken: 'token-123' },
			url: new URL('https://siem.townsville.cc/sources?preview=host-1')
		} as never)) as Exclude<Awaited<ReturnType<typeof load>>, void>;

		expect(result.previewName).toBe('host-1');
		expect(searchMock).toHaveBeenCalledWith('token-123', { source: 'host-1', limit: '1' });
	});

	it('splits sources into claimed and unclaimed', async () => {
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return {
				getSources: vi
					.fn()
					.mockResolvedValue([
						fakeSource({ id: 1, name: 'a', claimed: true }),
						fakeSource({ id: 2, name: 'b', claimed: false })
					]),
				getIngestHealth: vi.fn().mockResolvedValue(fakeHealth()),
				search: vi.fn().mockResolvedValue({ logql: '', count: 0, entries: [] })
			};
		});

		const result = (await load({
			locals: { sessionToken: 'token-123' },
			url: new URL('https://siem.townsville.cc/sources')
		} as never)) as Exclude<Awaited<ReturnType<typeof load>>, void>;

		expect(result.claimedSources).toHaveLength(1);
		expect(result.unclaimedSources).toHaveLength(1);
	});

	it('degrades the preview sample to null instead of failing the page on a search error', async () => {
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return {
				getSources: vi.fn().mockResolvedValue([fakeSource()]),
				getIngestHealth: vi.fn().mockResolvedValue(fakeHealth()),
				search: vi.fn().mockRejectedValue(new SiemApiError(502, 'loki down'))
			};
		});

		const result = (await load({
			locals: { sessionToken: 'token-123' },
			url: new URL('https://siem.townsville.cc/sources')
		} as never)) as Exclude<Awaited<ReturnType<typeof load>>, void>;

		expect(result.previewSample).toBeNull();
		expect(result.sources).toHaveLength(1);
	});

	it('redirects to /auth/logout on a 401/403 from the primary sources fetch', async () => {
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return {
				getSources: vi.fn().mockRejectedValue(new SiemApiError(401, 'invalid session')),
				getIngestHealth: vi.fn().mockResolvedValue(fakeHealth()),
				search: vi.fn()
			};
		});

		await expect(
			load({
				locals: { sessionToken: 'stale-token' },
				url: new URL('https://siem.townsville.cc/sources')
			} as never)
		).rejects.toMatchObject({ status: 302, location: '/auth/logout' });
	});

	it('surfaces a 502 when siem-api fails for a reason other than auth', async () => {
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return {
				getSources: vi.fn().mockRejectedValue(new SiemApiError(500, 'boom')),
				getIngestHealth: vi.fn().mockResolvedValue(fakeHealth()),
				search: vi.fn()
			};
		});

		await expect(
			load({
				locals: { sessionToken: 'token-123' },
				url: new URL('https://siem.townsville.cc/sources')
			} as never)
		).rejects.toMatchObject({ status: 502 });
	});
});
