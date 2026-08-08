import { describe, it, expect, vi } from 'vitest';
import { load } from './+page.server';
import * as siemApiClientModule from '$lib/server/siemApiClient';
import { SiemApiError } from '$lib/server/siemApiClient';

vi.mock('$env/dynamic/private', () => ({ env: { API_URL: 'http://siem-api:8080' } }));

vi.mock('$lib/server/siemApiClient', async (importOriginal) => {
	const actual = await importOriginal<typeof import('$lib/server/siemApiClient')>();
	return { ...actual, SiemApiClient: vi.fn() };
});

describe('Settings load', () => {
	it('returns the real role mappings from siem-api', async () => {
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return {
				getAuthSettings: vi.fn().mockResolvedValue({
					oidc_issuer: 'https://pocketid.townsville.cc',
					oidc_client_id: 'homeSIEM',
					oidc_groups_scope: 'groups',
					role_mappings: [{ id: 1, group_claim: 'admins', role: 'admin', priority: 10 }]
				}),
				getNotificationSettings: vi
					.fn()
					.mockResolvedValue({ ntfy_configured: false, min_severity: 'info' })
			};
		});

		const result = (await load({
			locals: { sessionToken: 'token-123' }
		} as never)) as Exclude<Awaited<ReturnType<typeof load>>, void>;

		expect(result.roleMappings).toEqual([
			{ id: 1, group_claim: 'admins', role: 'admin', priority: 10 }
		]);
		expect(result.notificationSettings).toEqual({ ntfy_configured: false, min_severity: 'info' });
	});

	it('returns an empty array when siem-api sends role_mappings: null', async () => {
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return {
				getAuthSettings: vi.fn().mockResolvedValue({
					oidc_issuer: 'https://pocketid.townsville.cc',
					oidc_client_id: 'homeSIEM',
					oidc_groups_scope: 'groups',
					role_mappings: null
				}),
				getNotificationSettings: vi
					.fn()
					.mockResolvedValue({ ntfy_configured: false, min_severity: 'info' })
			};
		});

		const result = (await load({
			locals: { sessionToken: 'token-123' }
		} as never)) as Exclude<Awaited<ReturnType<typeof load>>, void>;

		expect(result.roleMappings).toEqual([]);
	});

	it('redirects to /auth/logout on a 401 from siem-api', async () => {
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return {
				getAuthSettings: vi.fn().mockRejectedValue(new SiemApiError(401, 'invalid session'))
			};
		});

		await expect(load({ locals: { sessionToken: 'stale-token' } } as never)).rejects.toMatchObject({
			status: 302,
			location: '/auth/logout'
		});
	});

	it('rejects with 403 when siem-api reports the session is not an admin', async () => {
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return {
				getAuthSettings: vi.fn().mockRejectedValue(new SiemApiError(403, 'not an admin'))
			};
		});

		await expect(load({ locals: { sessionToken: 'viewer-token' } } as never)).rejects.toMatchObject(
			{
				status: 403
			}
		);
	});

	it('surfaces a 502 when siem-api fails for a reason other than auth', async () => {
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return {
				getAuthSettings: vi.fn().mockRejectedValue(new SiemApiError(500, 'boom'))
			};
		});

		await expect(load({ locals: { sessionToken: 'token-123' } } as never)).rejects.toMatchObject({
			status: 502
		});
	});
});
