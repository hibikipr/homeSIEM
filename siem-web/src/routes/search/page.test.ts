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

function run(search: string, fetch: typeof globalThis.fetch = vi.fn()) {
	return load({
		data: { entries, logql: '{job="siem"}' },
		url: new URL(`https://siem.townsville.cc/search${search}`),
		fetch
	} as never) as Promise<{
		previewIndex: number | null;
		selectedEntry: (typeof entries)[number] | null;
		contextSummary: Promise<{ count: number } | null> | null;
		logql: string;
	}>;
}

describe('Search universal load (row selection)', () => {
	it('passes server data through and selects nothing without ?preview=', async () => {
		const fetch = vi.fn();
		const result = await run('', fetch);

		expect(result.logql).toBe('{job="siem"}');
		expect(result.previewIndex).toBeNull();
		expect(result.selectedEntry).toBeNull();
		expect(result.contextSummary).toBeNull();
		expect(fetch).not.toHaveBeenCalled();
	});

	it('selects the entry and skips the context lookup when it has no src_ip', async () => {
		const fetch = vi.fn();
		const result = await run('?preview=0', fetch);

		expect(result.selectedEntry?.Line).toBe('{}');
		expect(result.contextSummary).toBeNull();
		expect(fetch).not.toHaveBeenCalled();
	});

	it('streams a context count from the BFF endpoint when the entry has a src_ip', async () => {
		const fetch = vi.fn().mockResolvedValue(new Response(JSON.stringify({ count: 4 })));
		const result = await run('?preview=1', fetch);

		expect(result.selectedEntry?.Line).toBe('{"src_ip":"10.0.0.5"}');
		await expect(result.contextSummary).resolves.toEqual({ count: 4 });
		expect(fetch).toHaveBeenCalledWith('/api/search/context-count?ip=10.0.0.5');
	});

	it('degrades contextSummary to null when the lookup fails', async () => {
		const fetch = vi.fn().mockResolvedValue(new Response('nope', { status: 502 }));
		const result = await run('?preview=1', fetch);

		await expect(result.contextSummary).resolves.toBeNull();
	});

	it('resolves previewIndex to null when ?preview= is non-numeric or out of range', async () => {
		expect((await run('?preview=abc')).previewIndex).toBeNull();
		const outOfRange = await run('?preview=9');
		expect(outOfRange.previewIndex).toBe(9);
		expect(outOfRange.selectedEntry).toBeNull();
	});
});
