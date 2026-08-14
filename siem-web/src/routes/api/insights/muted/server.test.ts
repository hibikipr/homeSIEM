import { describe, it, expect, vi } from 'vitest';
import { GET } from './+server';
import * as siemApiClientModule from '$lib/server/siemApiClient';

vi.mock('$env/dynamic/private', () => ({ env: { API_URL: 'http://siem-api:8080' } }));

vi.mock('$lib/server/siemApiClient', async (importOriginal) => {
	const actual = await importOriginal<typeof import('$lib/server/siemApiClient')>();
	return { ...actual, SiemApiClient: vi.fn() };
});

describe('GET /api/insights/muted', () => {
	it('returns the muted fingerprints from listMutedInsights', async () => {
		const listMutedInsightsMock = vi.fn().mockResolvedValue([
			{ fingerprint: 'abc123', category: 'operational', programs: 'UI-poller', muted_at: '2026-08-14T00:00:00Z' }
		]);
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return { listMutedInsights: listMutedInsightsMock };
		});

		const response = await GET({ locals: { sessionToken: 'token-123' } } as never);
		const body = await response.json();

		expect(listMutedInsightsMock).toHaveBeenCalledWith('token-123');
		expect(body).toHaveLength(1);
		expect(body[0].fingerprint).toBe('abc123');
	});

	it('propagates a SiemApiError status code as a JSON error response', async () => {
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return {
				listMutedInsights: vi.fn().mockRejectedValue(new siemApiClientModule.SiemApiError(403, 'denied'))
			};
		});

		const response = await GET({ locals: { sessionToken: 'token-123' } } as never);

		expect(response.status).toBe(403);
	});
});
