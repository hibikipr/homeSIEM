import { describe, it, expect, vi } from 'vitest';
import { load } from './+page.server';
import * as siemApiClientModule from '#lib/server/siemApiClient.ts';
import { SiemApiError } from '#lib/server/siemApiClient.ts';

vi.mock('$app/env/private', () => ({ API_URL: 'http://siem-api:8080' }));

vi.mock('#lib/server/siemApiClient.ts', async (importOriginal) => {
	const actual = await importOriginal<typeof import('#lib/server/siemApiClient.ts')>();
	return { ...actual, SiemApiClient: vi.fn() };
});

function fakeSource(overrides: Record<string, unknown> = {}) {
	return {
		id: 1,
		name: 'udm-ultra',
		address: '10.0.0.1',
		transport: 'udp/514',
		parser: 'unifi-os',
		claimed: true,
		heartbeat_sec: 900,
		status: 'healthy',
		events_per_min: 5,
		...overrides
	};
}

function fakeHealth() {
	return {
		received_events_per_source: {},
		loki_sent_events_total: 0,
		blank_messages_filtered_total: 0,
		degraded: false
	};
}

type LoadResult = Exclude<Awaited<ReturnType<typeof load>>, void>;

function mockClient(overrides: Record<string, unknown> = {}) {
	const client = {
		getSources: vi.fn().mockResolvedValue([fakeSource()]),
		getIngestHealth: vi.fn().mockResolvedValue(fakeHealth()),
		search: vi.fn().mockResolvedValue({ logql: '', count: 0, entries: [] }),
		getAlerts: vi.fn().mockResolvedValue([]),
		...overrides
	};
	vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
		return client;
	});
	return client;
}

async function run(search = '', token = 'token-123') {
	return (await load({
		depends: () => {},
		locals: { sessionToken: token },
		url: new URL(`https://siem.townsville.cc/sources${search}`)
	} as never)) as LoadResult;
}

describe('Sources load', () => {
	it('streams sources, health, and preview samples for the first source by default', async () => {
		const client = mockClient({
			getSources: vi.fn().mockResolvedValue([fakeSource({ name: 'udm-ultra' })]),
			search: vi.fn().mockResolvedValue({
				logql: '',
				count: 2,
				entries: [{ Line: '{"a":1}' }, { Line: '{"a":2}' }]
			})
		});

		const result = await run();
		const registry = await result.registry;

		expect(registry?.previewName).toBe('udm-ultra');
		await expect(result.previewSamples).resolves.toEqual([
			{ Line: '{"a":1}' },
			{ Line: '{"a":2}' }
		]);
		await expect(result.health).resolves.toEqual(fakeHealth());
		expect(client.search).toHaveBeenCalledWith('token-123', { source: 'udm-ultra', limit: '10' });
	});

	it('uses the ?preview= query param over the first source when given', async () => {
		const client = mockClient({
			getSources: vi
				.fn()
				.mockResolvedValue([
					fakeSource({ id: 1, name: 'udm-ultra' }),
					fakeSource({ id: 2, name: 'host-1' })
				])
		});

		const result = await run('?preview=host-1');
		expect((await result.registry)?.previewName).toBe('host-1');
		await result.previewSamples;
		expect(client.search).toHaveBeenCalledWith('token-123', { source: 'host-1', limit: '10' });
	});

	it('splits sources into claimed and unclaimed', async () => {
		mockClient({
			getSources: vi
				.fn()
				.mockResolvedValue([
					fakeSource({ id: 1, name: 'a', claimed: true }),
					fakeSource({ id: 2, name: 'b', claimed: false })
				])
		});

		const registry = await (await run()).registry;
		expect(registry?.claimedSources).toHaveLength(1);
		expect(registry?.unclaimedSources).toHaveLength(1);
	});

	it('degrades preview samples to an empty array on a search error', async () => {
		mockClient({ search: vi.fn().mockRejectedValue(new SiemApiError(502, 'loki down')) });

		const result = await run();
		await expect(result.previewSamples).resolves.toEqual([]);
		expect((await result.registry)?.sources).toHaveLength(1);
	});

	it('does not throw from load when the slow sources call fails; registry resolves to null', async () => {
		mockClient({ getSources: vi.fn().mockRejectedValue(new SiemApiError(500, 'loki timeout')) });

		const result = await run();
		await expect(result.registry).resolves.toBeNull();
		await expect(result.previewSamples).resolves.toEqual([]);
	});

	it('degrades ingest health to a safe default on any error', async () => {
		mockClient({ getIngestHealth: vi.fn().mockRejectedValue(new SiemApiError(500, 'down')) });

		const result = await run();
		await expect(result.health).resolves.toEqual({
			received_events_per_source: {},
			loki_sent_events_total: 0,
			blank_messages_filtered_total: 0,
			degraded: true
		});
	});

	it('redirects to /auth/logout on a 401/403 from the auth-gating call', async () => {
		mockClient({ getAlerts: vi.fn().mockRejectedValue(new SiemApiError(401, 'invalid session')) });

		await expect(run('', 'stale-token')).rejects.toMatchObject({
			status: 302,
			location: '/auth/logout'
		});
	});

	it('surfaces a 502 when the auth-gating call fails for a reason other than auth', async () => {
		mockClient({ getAlerts: vi.fn().mockRejectedValue(new SiemApiError(500, 'boom')) });

		await expect(run()).rejects.toMatchObject({ status: 502 });
	});
});
