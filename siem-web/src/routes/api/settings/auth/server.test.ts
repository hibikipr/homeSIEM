import { describe, it, expect, vi } from 'vitest';
import { PUT } from './+server';
import * as siemApiClientModule from '$lib/server/siemApiClient';

vi.mock('$env/dynamic/private', () => ({ env: { API_URL: 'http://siem-api:8080' } }));

vi.mock('$lib/server/siemApiClient', async (importOriginal) => {
	const actual = await importOriginal<typeof import('$lib/server/siemApiClient')>();
	return { ...actual, SiemApiClient: vi.fn() };
});

function fakeUpdateRequest() {
	return { role_mappings: [{ group_claim: 'homelab', role: 'viewer', priority: 100 }] };
}

describe('PUT /api/settings/auth', () => {
	it('calls updateRoleMappings with the session token and returns 204', async () => {
		const updateRoleMappingsMock = vi.fn().mockResolvedValue(undefined);
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return { updateRoleMappings: updateRoleMappingsMock };
		});

		const response = await PUT({
			request: new Request('http://x/api/settings/auth', {
				method: 'PUT',
				body: JSON.stringify(fakeUpdateRequest())
			}),
			locals: { sessionToken: 'token-123' }
		} as never);

		expect(updateRoleMappingsMock).toHaveBeenCalledWith('token-123', fakeUpdateRequest());
		expect(response.status).toBe(204);
	});

	it('propagates a SiemApiError status code as a JSON error response', async () => {
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return {
				updateRoleMappings: vi
					.fn()
					.mockRejectedValue(new siemApiClientModule.SiemApiError(403, 'denied'))
			};
		});

		const response = await PUT({
			request: new Request('http://x/api/settings/auth', {
				method: 'PUT',
				body: JSON.stringify(fakeUpdateRequest())
			}),
			locals: { sessionToken: 'token-123' }
		} as never);

		expect(response.status).toBe(403);
	});
});
