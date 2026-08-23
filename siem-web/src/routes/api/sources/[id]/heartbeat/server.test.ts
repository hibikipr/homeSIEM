import { describe, it, expect, vi } from 'vitest';
import { PUT } from './+server';
import * as siemApiClientModule from '$lib/server/siemApiClient';

vi.mock('$env/dynamic/private', () => ({ env: { API_URL: 'http://siem-api:8080' } }));

vi.mock('$lib/server/siemApiClient', async (importOriginal) => {
	const actual = await importOriginal<typeof import('$lib/server/siemApiClient')>();
	return { ...actual, SiemApiClient: vi.fn() };
});

function fakeRequest(body: unknown) {
	return { json: () => Promise.resolve(body) } as Request;
}

describe('PUT /api/sources/[id]/heartbeat', () => {
	it('calls setSourceHeartbeat with the session token, id, and heartbeat_sec, returns 204', async () => {
		const setHeartbeatMock = vi.fn().mockResolvedValue(undefined);
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return { setSourceHeartbeat: setHeartbeatMock };
		});

		const response = await PUT({
			params: { id: '7' },
			request: fakeRequest({ heartbeat_sec: 3600 }),
			locals: { sessionToken: 'token-123' }
		} as never);

		expect(setHeartbeatMock).toHaveBeenCalledWith('token-123', 7, 3600);
		expect(response.status).toBe(204);
	});

	it('defaults to 0 when the body is missing or malformed, letting siem-api reject it', async () => {
		const setHeartbeatMock = vi.fn().mockResolvedValue(undefined);
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return { setSourceHeartbeat: setHeartbeatMock };
		});

		await PUT({
			params: { id: '7' },
			request: fakeRequest({}),
			locals: { sessionToken: 'token-123' }
		} as never);

		expect(setHeartbeatMock).toHaveBeenCalledWith('token-123', 7, 0);
	});

	it('propagates a SiemApiError status code as a JSON error response', async () => {
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return {
				setSourceHeartbeat: vi
					.fn()
					.mockRejectedValue(new siemApiClientModule.SiemApiError(404, 'not found'))
			};
		});

		const response = await PUT({
			params: { id: '999' },
			request: fakeRequest({ heartbeat_sec: 3600 }),
			locals: { sessionToken: 'token-123' }
		} as never);

		expect(response.status).toBe(404);
	});
});
