import { describe, it, expect, vi } from 'vitest';
import { SiemApiClient } from './siemApiClient';

function fakeFetch(body: unknown, status = 200) {
	return vi.fn(async (_url: string | URL | Request, _init?: RequestInit) => {
		return new Response(body === null ? null : JSON.stringify(body), { status });
	});
}

describe('SiemApiClient', () => {
	it('getEventsStats attaches Authorization and parses the response', async () => {
		const fetchFn = fakeFetch({ event_count_24h: 1240000, heat_grid: [] });
		const client = new SiemApiClient({ baseUrl: 'http://siem-api:8080' }, fetchFn);

		const result = await client.getEventsStats('token-123');

		expect(result.event_count_24h).toBe(1240000);
		const [url, init] = fetchFn.mock.calls[0];
		expect(url).toBe('http://siem-api:8080/events/stats');
		expect((init?.headers as Record<string, string>).Authorization).toBe('Bearer token-123');
	});

	it('getAlerts appends the state query param when given', async () => {
		const fetchFn = fakeFetch([]);
		const client = new SiemApiClient({ baseUrl: 'http://siem-api:8080' }, fetchFn);

		await client.getAlerts('token-123', 'open');

		const [url] = fetchFn.mock.calls[0];
		expect(url).toBe('http://siem-api:8080/alerts?state=open');
	});

	it('getAlerts omits the query string when no state is given', async () => {
		const fetchFn = fakeFetch([]);
		const client = new SiemApiClient({ baseUrl: 'http://siem-api:8080' }, fetchFn);

		await client.getAlerts('token-123');

		const [url] = fetchFn.mock.calls[0];
		expect(url).toBe('http://siem-api:8080/alerts');
	});

	it('establishSession POSTs JSON with no Authorization header', async () => {
		const fetchFn = fakeFetch({ user_id: 7, role: 'analyst', display_name: 'Alice' });
		const client = new SiemApiClient({ baseUrl: 'http://siem-api:8080' }, fetchFn);

		const result = await client.establishSession({
			subject: 'sub-1',
			email: 'alice@townsville.cc',
			display_name: 'Alice',
			groups: ['siem-analysts']
		});

		expect(result.user_id).toBe(7);
		const [url, init] = fetchFn.mock.calls[0];
		expect(url).toBe('http://siem-api:8080/auth/session');
		expect(init?.method).toBe('POST');
		expect((init?.headers as Record<string, string>).Authorization).toBeUndefined();
		expect(JSON.parse(init?.body as string)).toEqual({
			subject: 'sub-1',
			email: 'alice@townsville.cc',
			display_name: 'Alice',
			groups: ['siem-analysts']
		});
	});

	it('throws SiemApiError with the status code on a non-OK response', async () => {
		const fetchFn = fakeFetch({ error: 'denied' }, 403);
		const client = new SiemApiClient({ baseUrl: 'http://siem-api:8080' }, fetchFn);

		await expect(client.getEventsStats('token-123')).rejects.toMatchObject({
			name: 'SiemApiError',
			status: 403
		});
	});

	it('ackAlert POSTs with Authorization and no body', async () => {
		const fetchFn = vi.fn<typeof fetch>(async () => new Response(null, { status: 204 }));
		const client = new SiemApiClient({ baseUrl: 'http://siem-api:8080' }, fetchFn);

		await client.ackAlert('token-123', 42);

		const [url, init] = fetchFn.mock.calls[0];
		expect(url).toBe('http://siem-api:8080/alerts/42/ack');
		expect(init?.method).toBe('POST');
		expect((init?.headers as Record<string, string>).Authorization).toBe('Bearer token-123');
	});

	it('muteAlert POSTs with Authorization and no body', async () => {
		const fetchFn = vi.fn<typeof fetch>(async () => new Response(null, { status: 204 }));
		const client = new SiemApiClient({ baseUrl: 'http://siem-api:8080' }, fetchFn);

		await client.muteAlert('token-123', 42);

		const [url, init] = fetchFn.mock.calls[0];
		expect(url).toBe('http://siem-api:8080/alerts/42/mute');
		expect(init?.method).toBe('POST');
		expect((init?.headers as Record<string, string>).Authorization).toBe('Bearer token-123');
	});

	it('getAlertSamples attaches Authorization and parses the response', async () => {
		const fetchFn = fakeFetch([
			{ id: 1, ts: '2026-08-02T00:00:00Z', line: '{"src_ip":"10.0.0.5"}' }
		]);
		const client = new SiemApiClient({ baseUrl: 'http://siem-api:8080' }, fetchFn);

		const result = await client.getAlertSamples('token-123', 42);

		expect(result).toHaveLength(1);
		const [url, init] = fetchFn.mock.calls[0];
		expect(url).toBe('http://siem-api:8080/alerts/42/samples');
		expect((init?.headers as Record<string, string>).Authorization).toBe('Bearer token-123');
	});

	it('getRules attaches Authorization and parses the response', async () => {
		const fetchFn = fakeFetch([
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
		const client = new SiemApiClient({ baseUrl: 'http://siem-api:8080' }, fetchFn);

		const result = await client.getRules('token-123');

		expect(result).toHaveLength(1);
		const [url] = fetchFn.mock.calls[0];
		expect(url).toBe('http://siem-api:8080/rules');
	});

	it('getSources attaches Authorization and parses the response', async () => {
		const fetchFn = fakeFetch([
			{
				id: 1,
				name: 'udm-ultra',
				address: '10.0.0.1',
				transport: 'udp/514',
				parser: 'unifi-os',
				claimed: true,
				heartbeat_sec: 900,
				status: 'healthy',
				events_per_min: 12
			}
		]);
		const client = new SiemApiClient({ baseUrl: 'http://siem-api:8080' }, fetchFn);

		const result = await client.getSources('token-123');

		expect(result).toHaveLength(1);
		expect(result[0].status).toBe('healthy');
		const [url, init] = fetchFn.mock.calls[0];
		expect(url).toBe('http://siem-api:8080/sources');
		expect((init?.headers as Record<string, string>).Authorization).toBe('Bearer token-123');
	});

	it('getIngestHealth attaches Authorization and parses the response', async () => {
		const fetchFn = fakeFetch({
			received_events_per_source: { unifi: 1234 },
			loki_sent_events_total: 1290,
			degraded: false
		});
		const client = new SiemApiClient({ baseUrl: 'http://siem-api:8080' }, fetchFn);

		const result = await client.getIngestHealth('token-123');

		expect(result.loki_sent_events_total).toBe(1290);
		const [url] = fetchFn.mock.calls[0];
		expect(url).toBe('http://siem-api:8080/sources/ingest-health');
	});

	it('claimSource POSTs to the claim endpoint with Authorization', async () => {
		const fetchFn = fakeFetch(null, 204);
		const client = new SiemApiClient({ baseUrl: 'http://siem-api:8080' }, fetchFn);

		await client.claimSource('token-123', 7);

		const [url, init] = fetchFn.mock.calls[0];
		expect(url).toBe('http://siem-api:8080/sources/7/claim');
		expect(init?.method).toBe('POST');
		expect((init?.headers as Record<string, string>).Authorization).toBe('Bearer token-123');
	});

	it('search parses the volume field from the response', async () => {
		const fetchFn = fakeFetch({
			logql: '{job="siem"}',
			count: 1,
			entries: [],
			volume: [{ bucket_start: '2026-08-05T00:00:00Z', count: 3 }]
		});
		const client = new SiemApiClient({ baseUrl: 'http://siem-api:8080' }, fetchFn);

		const result = await client.search('token-123', {});

		expect(result.volume).toEqual([{ bucket_start: '2026-08-05T00:00:00Z', count: 3 }]);
	});

	it('createRule POSTs to /rules with Authorization and parses the response', async () => {
		const fetchFn = fakeFetch(
			{
				id: 9,
				name: 'search-alert',
				shape: 'threshold',
				logql: '{job="siem"}',
				window_sec: 60,
				group_by: [],
				severity: 'warning',
				destinations: ['inapp'],
				cooldown_sec: 3600,
				interval_sec: 60,
				enabled: true
			},
			201
		);
		const client = new SiemApiClient({ baseUrl: 'http://siem-api:8080' }, fetchFn);

		const result = await client.createRule('token-123', {
			name: 'search-alert',
			shape: 'threshold',
			logql: '{job="siem"}',
			window_sec: 60,
			group_by: [],
			severity: 'warning',
			destinations: ['inapp'],
			cooldown_sec: 3600,
			interval_sec: 60,
			enabled: true
		});

		expect(result.id).toBe(9);
		const [url, init] = fetchFn.mock.calls[0];
		expect(url).toBe('http://siem-api:8080/rules');
		expect(init?.method).toBe('POST');
		expect((init?.headers as Record<string, string>).Authorization).toBe('Bearer token-123');
	});

	it('getAuthSettings attaches Authorization and parses the response', async () => {
		const fetchFn = fakeFetch({
			oidc_issuer: 'https://pocketid.townsville.cc',
			oidc_client_id: 'homeSIEM',
			oidc_groups_scope: 'groups',
			role_mappings: [{ id: 1, group_claim: 'admins', role: 'admin', priority: 10 }]
		});
		const client = new SiemApiClient({ baseUrl: 'http://siem-api:8080' }, fetchFn);

		const result = await client.getAuthSettings('token-123');

		expect(result.role_mappings).toHaveLength(1);
		expect(result.role_mappings?.[0].group_claim).toBe('admins');
		const [url, init] = fetchFn.mock.calls[0];
		expect(url).toBe('http://siem-api:8080/settings/auth');
		expect((init?.headers as Record<string, string>).Authorization).toBe('Bearer token-123');
	});

	it('updateRoleMappings PUTs to /settings/auth with Authorization and a JSON body', async () => {
		const fetchFn = fakeFetch(null, 204);
		const client = new SiemApiClient({ baseUrl: 'http://siem-api:8080' }, fetchFn);

		await client.updateRoleMappings('token-123', {
			role_mappings: [{ group_claim: 'homelab', role: 'viewer', priority: 100 }]
		});

		const [url, init] = fetchFn.mock.calls[0];
		expect(url).toBe('http://siem-api:8080/settings/auth');
		expect(init?.method).toBe('PUT');
		expect((init?.headers as Record<string, string>).Authorization).toBe('Bearer token-123');
		expect(JSON.parse(init?.body as string)).toEqual({
			role_mappings: [{ group_claim: 'homelab', role: 'viewer', priority: 100 }]
		});
	});
});
