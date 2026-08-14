import { describe, it, expect, vi } from 'vitest';
import { PUT } from './+server';
import * as siemApiClientModule from '$lib/server/siemApiClient';

vi.mock('$env/dynamic/private', () => ({ env: { API_URL: 'http://siem-api:8080' } }));

vi.mock('$lib/server/siemApiClient', async (importOriginal) => {
	const actual = await importOriginal<typeof import('$lib/server/siemApiClient')>();
	return { ...actual, SiemApiClient: vi.fn() };
});

describe('PUT /api/insights/[id]/mute', () => {
	it('calls muteInsight with the session token and returns 204', async () => {
		const muteInsightMock = vi.fn().mockResolvedValue(undefined);
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return { muteInsight: muteInsightMock };
		});

		const response = await PUT({
			params: { id: '42' },
			locals: { sessionToken: 'token-123' }
		} as never);

		expect(muteInsightMock).toHaveBeenCalledWith('token-123', 42);
		expect(response.status).toBe(204);
	});

	it('propagates a SiemApiError status code as a JSON error response', async () => {
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return {
				muteInsight: vi.fn().mockRejectedValue(new siemApiClientModule.SiemApiError(404, 'not found'))
			};
		});

		const response = await PUT({
			params: { id: '999' },
			locals: { sessionToken: 'token-123' }
		} as never);

		expect(response.status).toBe(404);
	});
});
