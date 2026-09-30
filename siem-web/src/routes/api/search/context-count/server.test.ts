import { describe, it, expect, vi } from 'vitest';
import { GET } from './+server';
import * as siemApiClientModule from '$lib/server/siemApiClient';
import { SiemApiError } from '$lib/server/siemApiClient';

vi.mock('$env/dynamic/private', () => ({ env: { API_URL: 'http://siem-api:8080' } }));

vi.mock('$lib/server/siemApiClient', async (importOriginal) => {
	const actual = await importOriginal<typeof import('$lib/server/siemApiClient')>();
	return { ...actual, SiemApiClient: vi.fn() };
});

function call(search: string) {
	return GET({
		url: new URL(`https://siem.townsville.cc/api/search/context-count${search}`),
		locals: { sessionToken: 'token-123' }
	} as never);
}

describe('GET /api/search/context-count', () => {
	it('returns only the Loki-side count, skipping entries/volume/facets', async () => {
		const searchMock = vi.fn().mockResolvedValue({ count: 4, entries: [] });
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return { search: searchMock };
		});

		const res = await call('?ip=10.0.0.5');

		expect(res.status).toBe(200);
		expect(await res.json()).toEqual({ count: 4 });
		expect(searchMock).toHaveBeenCalledWith(
			'token-123',
			expect.objectContaining({ q: '10.0.0.5', entries: 'false', volume: 'false', facets: 'false' })
		);
	});

	it('400s without an ip', async () => {
		expect((await call('')).status).toBe(400);
	});

	it('passes through siem-api error statuses', async () => {
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return { search: vi.fn().mockRejectedValue(new SiemApiError(502, 'loki down')) };
		});

		expect((await call('?ip=10.0.0.5')).status).toBe(502);
	});
});
