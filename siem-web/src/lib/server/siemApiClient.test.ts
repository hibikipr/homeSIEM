import { describe, it, expect, vi } from 'vitest';
import { SiemApiClient, SiemApiError } from './siemApiClient';

function fakeFetch(body: unknown, status = 200) {
	return vi.fn(async (_url: string | URL | Request, _init?: RequestInit) => {
		return new Response(JSON.stringify(body), { status });
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
});
