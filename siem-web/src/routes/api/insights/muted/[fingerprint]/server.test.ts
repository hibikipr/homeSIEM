import { describe, it, expect, vi } from 'vitest';
import { DELETE } from './+server';
import * as siemApiClientModule from '$lib/server/siemApiClient';

vi.mock('$env/dynamic/private', () => ({ env: { API_URL: 'http://siem-api:8080' } }));

vi.mock('$lib/server/siemApiClient', async (importOriginal) => {
	const actual = await importOriginal<typeof import('$lib/server/siemApiClient')>();
	return { ...actual, SiemApiClient: vi.fn() };
});

describe('DELETE /api/insights/muted/[fingerprint]', () => {
	it('calls unmuteInsight with the session token and returns 204', async () => {
		const unmuteInsightMock = vi.fn().mockResolvedValue(undefined);
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return { unmuteInsight: unmuteInsightMock };
		});

		const response = await DELETE({
			params: { fingerprint: 'abc123' },
			locals: { sessionToken: 'token-123' }
		} as never);

		expect(unmuteInsightMock).toHaveBeenCalledWith('token-123', 'abc123');
		expect(response.status).toBe(204);
	});

	it('propagates a SiemApiError status code as a JSON error response', async () => {
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return {
				unmuteInsight: vi.fn().mockRejectedValue(new siemApiClientModule.SiemApiError(404, 'not found'))
			};
		});

		const response = await DELETE({
			params: { fingerprint: 'does-not-exist' },
			locals: { sessionToken: 'token-123' }
		} as never);

		expect(response.status).toBe(404);
	});
});
