import { describe, it, expect } from 'vitest';
import { splitClaimedUnclaimed, formatEventsPerMin, formatLastSeen } from './sources';
import type { SourceResponse } from './server/siemApiClient';

function fakeSource(overrides: Partial<SourceResponse> = {}): SourceResponse {
	return {
		id: 1,
		name: 'udm-ultra',
		display_name: '',
		address: '10.0.0.1',
		transport: 'udp/514',
		parser: 'unifi-os',
		claimed: true,
		heartbeat_sec: 900,
		status: 'healthy',
		events_per_min: 0,
		...overrides
	};
}

describe('splitClaimedUnclaimed', () => {
	it('splits sources by claimed status', () => {
		const claimed = fakeSource({ id: 1, claimed: true });
		const unclaimed = fakeSource({ id: 2, claimed: false });

		const result = splitClaimedUnclaimed([claimed, unclaimed]);

		expect(result.claimed).toEqual([claimed]);
		expect(result.unclaimed).toEqual([unclaimed]);
	});
});

describe('formatEventsPerMin', () => {
	it('rounds values of 1 or more to the nearest integer', () => {
		expect(formatEventsPerMin(12.6)).toBe('13');
	});

	it('shows one decimal place for sub-1 rates instead of rounding to 0', () => {
		expect(formatEventsPerMin(0.4)).toBe('0.4');
	});
});

describe('formatLastSeen', () => {
	it('returns "never" when last_seen_at is undefined', () => {
		expect(formatLastSeen(undefined)).toBe('never');
	});

	it('formats a recent timestamp in minutes', () => {
		const iso = new Date(Date.now() - 5 * 60_000).toISOString();
		expect(formatLastSeen(iso)).toBe('5m ago');
	});

	it('formats an older timestamp in hours', () => {
		const iso = new Date(Date.now() - 3 * 3_600_000).toISOString();
		expect(formatLastSeen(iso)).toBe('3h ago');
	});
});
