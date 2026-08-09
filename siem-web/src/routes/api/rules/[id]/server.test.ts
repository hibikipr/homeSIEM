import { describe, it, expect, vi } from 'vitest';
import { PUT } from './+server';
import * as siemApiClientModule from '$lib/server/siemApiClient';

vi.mock('$env/dynamic/private', () => ({ env: { API_URL: 'http://siem-api:8080' } }));

vi.mock('$lib/server/siemApiClient', async (importOriginal) => {
	const actual = await importOriginal<typeof import('$lib/server/siemApiClient')>();
	return { ...actual, SiemApiClient: vi.fn() };
});

function fakeRuleRequest(overrides: Record<string, unknown> = {}) {
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
		enabled: false,
		...overrides
	};
}

describe('PUT /api/rules/[id]', () => {
	it('calls updateRule with the session token, id, and body, returning the updated rule', async () => {
		const updateRuleMock = vi.fn().mockResolvedValue({ id: 9, ...fakeRuleRequest() });
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return { updateRule: updateRuleMock };
		});

		const response = await PUT({
			request: new Request('http://x/api/rules/9', {
				method: 'PUT',
				body: JSON.stringify(fakeRuleRequest())
			}),
			params: { id: '9' },
			locals: { sessionToken: 'token-123' }
		} as never);

		expect(updateRuleMock).toHaveBeenCalledWith('token-123', 9, fakeRuleRequest());
		expect(response.status).toBe(200);
		const body = await response.json();
		expect(body.enabled).toBe(false);
	});

	it('propagates a SiemApiError status code as a JSON error response', async () => {
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return {
				updateRule: vi.fn().mockRejectedValue(new siemApiClientModule.SiemApiError(403, 'denied'))
			};
		});

		const response = await PUT({
			request: new Request('http://x/api/rules/9', {
				method: 'PUT',
				body: JSON.stringify(fakeRuleRequest())
			}),
			params: { id: '9' },
			locals: { sessionToken: 'token-123' }
		} as never);

		expect(response.status).toBe(403);
	});
});
