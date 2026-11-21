import { describe, it, expect, vi } from 'vitest';
import { load } from './+page';

const entries = [
	{ Timestamp: '2026-08-05T00:00:00Z', Labels: { severity: 'info' }, Line: '{}' },
	{
		Timestamp: '2026-08-05T00:00:01Z',
		Labels: { severity: 'critical' },
		Line: '{"src_ip":"10.0.0.5"}'
	}
];

function run(
	search: string,
	fetch: typeof globalThis.fetch = vi.fn(),
	results: { entries: typeof entries } | null | Promise<never> = { entries }
) {
	return load({
		depends: () => {},
		data: {
			results: results instanceof Promise ? results : Promise.resolve(results),
			logql: '{job="siem"}'
		},
		url: new URL(`https://siem.townsville.cc/search${search}`),
		fetch
	} as never) as Promise<{
		previewIndex: number | null;
		contextSummary: Promise<{ count: number } | null> | null;
		logql: string;
	}>;
}

describe('Search universal load (row selection)', () => {
	it('passes server data through and starts no lookup without ?preview=', async () => {
		const fetch = vi.fn();
		const result = await run('', fetch);

		expect(result.logql).toBe('{job="siem"}');
		expect(result.previewIndex).toBeNull();
		expect(result.contextSummary).toBeNull();
		expect(fetch).not.toHaveBeenCalled();
	});

	it('skips the context lookup when the selected entry has no src_ip', async () => {
		const fetch = vi.fn();
		const result = await run('?preview=0', fetch);

		expect(result.previewIndex).toBe(0);
		await expect(result.contextSummary).resolves.toBeNull();
		expect(fetch).not.toHaveBeenCalled();
	});

	it('streams a context count from the BFF endpoint when the entry has a src_ip', async () => {
		const fetch = vi.fn().mockResolvedValue(new Response(JSON.stringify({ count: 4 })));
		const result = await run('?preview=1', fetch);

		await expect(result.contextSummary).resolves.toEqual({ count: 4 });
		expect(fetch).toHaveBeenCalledWith('/api/search/context-count?ip=10.0.0.5');
	});

	it('degrades contextSummary to null when the lookup fails', async () => {
		const fetch = vi.fn().mockResolvedValue(new Response('nope', { status: 502 }));
		const result = await run('?preview=1', fetch);

		await expect(result.contextSummary).resolves.toBeNull();
	});

	it('does not throw and resolves contextSummary to null when the search itself failed', async () => {
		const fetch = vi.fn();
		const result = await run('?preview=1', fetch, null);

		await expect(result.contextSummary).resolves.toBeNull();
		expect(fetch).not.toHaveBeenCalled();
	});

	it('resolves previewIndex to null when ?preview= is non-numeric, and no lookup for out of range', async () => {
		expect((await run('?preview=abc')).previewIndex).toBeNull();
		const fetch = vi.fn();
		const outOfRange = await run('?preview=9', fetch);
		expect(outOfRange.previewIndex).toBe(9);
		await expect(outOfRange.contextSummary).resolves.toBeNull();
		expect(fetch).not.toHaveBeenCalled();
	});
});
