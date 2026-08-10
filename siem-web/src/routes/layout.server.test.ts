import { describe, it, expect, vi } from 'vitest';
import { load } from './+layout.server';
import * as siemApiClientModule from '$lib/server/siemApiClient';

vi.mock('$env/dynamic/private', () => ({
	env: { API_URL: 'http://siem-api:8080', TZ: 'America/New_York' }
}));

vi.mock('$lib/server/siemApiClient', async (importOriginal) => {
	const actual = await importOriginal<typeof import('$lib/server/siemApiClient')>();
	return { ...actual, SiemApiClient: vi.fn() };
});

const fakeUser = { displayName: 'Test', email: 't@t.com', role: 'viewer', picture: '' };

describe('Root layout load', () => {
	it('returns real ingest rate and open alert count for an authenticated session', async () => {
		const getNavSummaryMock = vi
			.fn()
			.mockResolvedValue({ events_per_min: 42, open_alert_count: 3 });
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return { getNavSummary: getNavSummaryMock };
		});

		const result = (await load({
			locals: { user: fakeUser, sessionToken: 'token-123' },
			url: new URL('https://siem.townsville.cc/')
		} as never)) as Exclude<Awaited<ReturnType<typeof load>>, void>;

		expect(result.ingestRate).toBe(42);
		expect(result.alertCount).toBe(3);
		expect(getNavSummaryMock).toHaveBeenCalledWith('token-123');
		expect(result.displayTimezone).toBe('America/New_York');
	});

	it('never calls the API for an unauthenticated session, returns zeros', async () => {
		const getNavSummaryMock = vi.fn();
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return { getNavSummary: getNavSummaryMock };
		});

		const result = (await load({
			locals: {},
			url: new URL('https://siem.townsville.cc/auth/login')
		} as never)) as Exclude<Awaited<ReturnType<typeof load>>, void>;

		expect(result.ingestRate).toBe(0);
		expect(result.alertCount).toBe(0);
		expect(getNavSummaryMock).not.toHaveBeenCalled();
	});

	it('falls back to zeros without throwing when the nav summary lookup fails', async () => {
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return { getNavSummary: vi.fn().mockRejectedValue(new Error('siem-api unavailable')) };
		});

		// Nav chrome is supplementary, not gated content - a failure here must
		// never break page navigation or force a redirect the way a real
		// page-level loader's auth gate does.
		const result = (await load({
			locals: { user: fakeUser, sessionToken: 'token-123' },
			url: new URL('https://siem.townsville.cc/')
		} as never)) as Exclude<Awaited<ReturnType<typeof load>>, void>;

		expect(result.ingestRate).toBe(0);
		expect(result.alertCount).toBe(0);
	});

	it('defaults displayTimezone to UTC when TZ is unset', async () => {
		vi.resetModules();
		vi.doMock('$env/dynamic/private', () => ({
			env: { API_URL: 'http://siem-api:8080' }
		}));
		const { load: loadWithNoTz } = await import('./+layout.server');

		const result = (await loadWithNoTz({
			locals: {},
			url: new URL('https://siem.townsville.cc/auth/login')
		} as never)) as Exclude<Awaited<ReturnType<typeof load>>, void>;

		expect(result.displayTimezone).toBe('UTC');
	});
});
