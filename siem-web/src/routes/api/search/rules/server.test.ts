import { describe, it, expect, vi } from 'vitest';
import { POST } from './+server';
import * as siemApiClientModule from '$lib/server/siemApiClient';

vi.mock('$env/dynamic/private', () => ({ env: { API_URL: 'http://siem-api:8080' } }));

vi.mock('$lib/server/siemApiClient', async (importOriginal) => {
	const actual = await importOriginal<typeof import('$lib/server/siemApiClient')>();
	return { ...actual, SiemApiClient: vi.fn() };
});

function fakeRuleRequest() {
	return {
		name: 'search-alert',
		shape: 'threshold',
		logql: '{job="siem"}',
		window_sec: 60,
		threshold: 5,
		group_by: [],
		severity: 'warning',
		destinations: ['inapp'],
		cooldown_sec: 3600,
		interval_sec: 60,
		enabled: true
	};
}

describe('POST /api/search/rules', () => {
	it('calls createRule with the session token and returns 201', async () => {
		const createRuleMock = vi.fn().mockResolvedValue({ id: 9, name: 'search-alert' });
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return { createRule: createRuleMock };
		});

		const response = await POST({
			request: new Request('http://x/api/search/rules', {
				method: 'POST',
				body: JSON.stringify(fakeRuleRequest())
			}),
			locals: { sessionToken: 'token-123' }
		} as never);

		expect(createRuleMock).toHaveBeenCalledWith('token-123', fakeRuleRequest());
		expect(response.status).toBe(201);
	});

	it('propagates a SiemApiError status code as a JSON error response', async () => {
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return {
				createRule: vi.fn().mockRejectedValue(new siemApiClientModule.SiemApiError(403, 'denied'))
			};
		});

		const response = await POST({
			request: new Request('http://x/api/search/rules', {
				method: 'POST',
				body: JSON.stringify(fakeRuleRequest())
			}),
			locals: { sessionToken: 'token-123' }
		} as never);

		expect(response.status).toBe(403);
	});
});
