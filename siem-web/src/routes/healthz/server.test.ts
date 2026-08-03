import { describe, it, expect } from 'vitest';
import { GET } from './+server';

describe('GET /healthz', () => {
	it('returns 200 ok', async () => {
		const response = await GET({} as never);

		expect(response.status).toBe(200);
		expect(await response.text()).toBe('ok');
	});
});
