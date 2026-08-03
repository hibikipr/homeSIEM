import { describe, it, expect, vi } from 'vitest';
import { GET } from './+server';

vi.mock('$env/dynamic/private', () => ({ env: { API_URL: 'http://siem-api:8080' } }));

describe('GET /api/tail-proxy', () => {
	it('forwards the Authorization header to siem-api and streams the response', async () => {
		const fetchFn = vi.fn().mockResolvedValue(new Response(new ReadableStream(), { status: 200 }));

		const response = await GET({ locals: { sessionToken: 'token-123' }, fetch: fetchFn } as never);

		expect(fetchFn).toHaveBeenCalledWith(
			'http://siem-api:8080/events/tail',
			expect.objectContaining({ headers: { Authorization: 'Bearer token-123' } })
		);
		expect(response.headers.get('Content-Type')).toBe('text/event-stream');
		expect(response.status).toBe(200);
	});
});
