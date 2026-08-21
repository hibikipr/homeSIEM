import { describe, it, expect, vi } from 'vitest';
import { DELETE } from './+server';
import * as siemApiClientModule from '$lib/server/siemApiClient';

vi.mock('$env/dynamic/private', () => ({ env: { API_URL: 'http://siem-api:8080' } }));

vi.mock('$lib/server/siemApiClient', async (importOriginal) => {
	const actual = await importOriginal<typeof import('$lib/server/siemApiClient')>();
	return { ...actual, SiemApiClient: vi.fn() };
});

describe('DELETE /api/sources/[id]', () => {
	it('calls deleteSource with the session token and id, returns 204', async () => {
		const deleteSourceMock = vi.fn().mockResolvedValue(undefined);
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return { deleteSource: deleteSourceMock };
		});

		const response = await DELETE({
			params: { id: '7' },
			locals: { sessionToken: 'token-123' }
		} as never);

		expect(deleteSourceMock).toHaveBeenCalledWith('token-123', 7);
		expect(response.status).toBe(204);
	});

	it('propagates a SiemApiError status code as a JSON error response', async () => {
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return {
				deleteSource: vi
					.fn()
					.mockRejectedValue(new siemApiClientModule.SiemApiError(404, 'not found'))
			};
		});

		const response = await DELETE({
			params: { id: '999' },
			locals: { sessionToken: 'token-123' }
		} as never);

		expect(response.status).toBe(404);
	});
});
