import { describe, it, expect, vi } from 'vitest';
import { POST } from './+server';
import * as siemApiClientModule from '$lib/server/siemApiClient';

vi.mock('$env/dynamic/private', () => ({ env: { API_URL: 'http://siem-api:8080' } }));

vi.mock('$lib/server/siemApiClient', async (importOriginal) => {
	const actual = await importOriginal<typeof import('$lib/server/siemApiClient')>();
	return { ...actual, SiemApiClient: vi.fn() };
});

describe('POST /api/sources/[id]/claim', () => {
	it('calls claimSource with the session token and returns 204', async () => {
		const claimSourceMock = vi.fn().mockResolvedValue(undefined);
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return { claimSource: claimSourceMock };
		});

		const response = await POST({
			params: { id: '7' },
			locals: { sessionToken: 'token-123' }
		} as never);

		expect(claimSourceMock).toHaveBeenCalledWith('token-123', 7);
		expect(response.status).toBe(204);
	});

	it('propagates a SiemApiError status code as a JSON error response', async () => {
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return {
				claimSource: vi.fn().mockRejectedValue(new siemApiClientModule.SiemApiError(403, 'denied'))
			};
		});

		const response = await POST({
			params: { id: '7' },
			locals: { sessionToken: 'token-123' }
		} as never);

		expect(response.status).toBe(403);
	});
});
