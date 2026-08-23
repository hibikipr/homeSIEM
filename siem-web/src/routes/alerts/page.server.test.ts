import { describe, it, expect, vi } from 'vitest';
import { load } from './+page.server';
import * as siemApiClientModule from '$lib/server/siemApiClient';
import { SiemApiError } from '$lib/server/siemApiClient';

vi.mock('$env/dynamic/private', () => ({ env: { API_URL: 'http://siem-api:8080' } }));

vi.mock('$lib/server/siemApiClient', async (importOriginal) => {
	const actual = await importOriginal<typeof import('$lib/server/siemApiClient')>();
	return { ...actual, SiemApiClient: vi.fn() };
});

function fakeAlert(overrides: Partial<{ id: number; state: string; rule_id: number }> = {}) {
	return {
		id: 1,
		rule_id: 1,
		group_key: 'a',
		severity: 'critical',
		title: 't',
		body: 'b',
		event_count: 1,
		state: 'open',
		first_seen_at: '2026-08-02T00:00:00Z',
		last_seen_at: '2026-08-02T00:00:00Z',
		...overrides
	};
}

describe('Alerts load', () => {
	it('defaults to the open tab and loads alerts plus rules for it', async () => {
		const getAlertsMock = vi.fn().mockResolvedValue([fakeAlert()]);
		const getRulesMock = vi.fn().mockResolvedValue([]);
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return {
				getAlerts: getAlertsMock,
				getRules: getRulesMock,
				getSources: vi.fn().mockResolvedValue([]),
				getAlertSamples: vi.fn()
			};
		});

		const result = (await load({
			locals: { sessionToken: 'token-123' },
			url: new URL('https://siem.townsville.cc/alerts')
		} as never)) as Exclude<Awaited<ReturnType<typeof load>>, void>;

		expect(result.tab).toBe('open');
		expect(result.alerts).toHaveLength(1);
		expect(getAlertsMock).toHaveBeenCalledWith('token-123', 'open');
		expect(getRulesMock).toHaveBeenCalledWith('token-123');
	});

	it('loads rules and no alerts when state=rules', async () => {
		const getAlertsMock = vi.fn();
		const getRulesMock = vi.fn().mockResolvedValue([
			{
				id: 1,
				name: 'wan-portscan',
				shape: 'threshold',
				logql: '{job="siem"}',
				window_sec: 60,
				group_by: [],
				severity: 'critical',
				destinations: ['inapp'],
				cooldown_sec: 3600,
				interval_sec: 60,
				enabled: true
			}
		]);
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return {
				getAlerts: getAlertsMock,
				getRules: getRulesMock,
				getSources: vi.fn().mockResolvedValue([]),
				getAlertSamples: vi.fn()
			};
		});

		const result = (await load({
			locals: { sessionToken: 'token-123' },
			url: new URL('https://siem.townsville.cc/alerts?state=rules')
		} as never)) as Exclude<Awaited<ReturnType<typeof load>>, void>;

		expect(result.tab).toBe('rules');
		expect(result.rules).toHaveLength(1);
		expect(result.alerts).toEqual([]);
		expect(getAlertsMock).not.toHaveBeenCalled();
	});

	it('resolves selectedRule from the rules list when state=rules and id is given', async () => {
		const rule = {
			id: 5,
			name: 'wan-portscan',
			shape: 'threshold',
			logql: '{job="siem"}',
			window_sec: 60,
			group_by: [],
			severity: 'critical',
			destinations: ['inapp'],
			cooldown_sec: 3600,
			interval_sec: 60,
			enabled: true
		};
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return {
				getAlerts: vi.fn(),
				getRules: vi.fn().mockResolvedValue([rule]),
				getSources: vi.fn().mockResolvedValue([]),
				getAlertSamples: vi.fn()
			};
		});

		const result = (await load({
			locals: { sessionToken: 'token-123' },
			url: new URL('https://siem.townsville.cc/alerts?state=rules&id=5')
		} as never)) as Exclude<Awaited<ReturnType<typeof load>>, void>;

		expect(result.selectedRule?.id).toBe(5);
		expect(result.selectedAlert).toBeNull();
	});

	it('loads samples and stats for the selected alert when id is given', async () => {
		const alert = fakeAlert({ id: 7 });
		const getAlertSamplesMock = vi
			.fn()
			.mockResolvedValue([
				{ id: 1, ts: '2026-08-02T00:00:00Z', line: '{"src_ip":"10.0.0.5","dst_port":443}' }
			]);
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return {
				getAlerts: vi.fn().mockResolvedValue([alert]),
				getRules: vi.fn().mockResolvedValue([]),
				getSources: vi.fn().mockResolvedValue([]),
				getAlertSamples: getAlertSamplesMock
			};
		});

		const result = (await load({
			locals: { sessionToken: 'token-123' },
			url: new URL('https://siem.townsville.cc/alerts?id=7')
		} as never)) as Exclude<Awaited<ReturnType<typeof load>>, void>;

		expect(result.selectedAlert?.id).toBe(7);
		expect(result.selectedSamples).toHaveLength(1);
		expect(result.stats?.distinctPorts).toEqual([443]);
		expect(getAlertSamplesMock).toHaveBeenCalledWith('token-123', 7);
	});

	it('redirects to /auth/logout on a 401/403 from siem-api', async () => {
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return {
				getAlerts: vi.fn().mockRejectedValue(new SiemApiError(401, 'invalid session')),
				getRules: vi.fn().mockResolvedValue([]),
				getSources: vi.fn().mockResolvedValue([]),
				getAlertSamples: vi.fn()
			};
		});

		await expect(
			load({
				locals: { sessionToken: 'stale-token' },
				url: new URL('https://siem.townsville.cc/alerts')
			} as never)
		).rejects.toMatchObject({ status: 302, location: '/auth/logout' });
	});

	it('surfaces a 502 when siem-api fails for a reason other than auth', async () => {
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return {
				getAlerts: vi.fn().mockRejectedValue(new SiemApiError(500, 'boom')),
				getRules: vi.fn().mockResolvedValue([]),
				getSources: vi.fn().mockResolvedValue([]),
				getAlertSamples: vi.fn()
			};
		});

		await expect(
			load({
				locals: { sessionToken: 'token-123' },
				url: new URL('https://siem.townsville.cc/alerts')
			} as never)
		).rejects.toMatchObject({ status: 502 });
	});

	it('returns a name lookup keyed by raw source name, from live Sources data', async () => {
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return {
				getAlerts: vi.fn().mockResolvedValue([fakeAlert()]),
				getRules: vi.fn().mockResolvedValue([]),
				getSources: vi.fn().mockResolvedValue([
					{ id: 1, name: '192.168.3.223', display_name: 'Home Assistant', claimed: true },
					{ id: 2, name: 'udm-ultra', display_name: '', claimed: true }
				]),
				getAlertSamples: vi.fn()
			};
		});

		const result = (await load({
			locals: { sessionToken: 'token-123' },
			url: new URL('https://siem.townsville.cc/alerts')
		} as never)) as Exclude<Awaited<ReturnType<typeof load>>, void>;

		// sourceDisplayNames/liveSourcesByName are streamed (not awaited
		// before load() returns - see the module-level comment on
		// sourcesByNamePromise), hence the extra await here.
		await expect(result.sourceDisplayNames).resolves.toEqual({
			'192.168.3.223': 'Home Assistant',
			'udm-ultra': ''
		});
	});

	it('also returns the full live Sources rows keyed by name, for AlertDetail context fallback', async () => {
		const homeAssistant = {
			id: 1,
			name: '192.168.3.223',
			display_name: 'Home Assistant',
			claimed: true
		};
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return {
				getAlerts: vi.fn().mockResolvedValue([fakeAlert()]),
				getRules: vi.fn().mockResolvedValue([]),
				getSources: vi.fn().mockResolvedValue([homeAssistant]),
				getAlertSamples: vi.fn()
			};
		});

		const result = (await load({
			locals: { sessionToken: 'token-123' },
			url: new URL('https://siem.townsville.cc/alerts')
		} as never)) as Exclude<Awaited<ReturnType<typeof load>>, void>;

		await expect(result.liveSourcesByName).resolves.toEqual({ '192.168.3.223': homeAssistant });
	});

	it('degrades to an empty name lookup if the Sources fetch fails, without failing the page', async () => {
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return {
				getAlerts: vi.fn().mockResolvedValue([fakeAlert()]),
				getRules: vi.fn().mockResolvedValue([]),
				getSources: vi.fn().mockRejectedValue(new Error('boom')),
				getAlertSamples: vi.fn()
			};
		});

		const result = (await load({
			locals: { sessionToken: 'token-123' },
			url: new URL('https://siem.townsville.cc/alerts')
		} as never)) as Exclude<Awaited<ReturnType<typeof load>>, void>;

		await expect(result.sourceDisplayNames).resolves.toEqual({});
		await expect(result.liveSourcesByName).resolves.toEqual({});
		expect(result.alerts).toHaveLength(1);
	});
});
