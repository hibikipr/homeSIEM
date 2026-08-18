import { describe, it, expect } from 'vitest';
import {
	parseFiltersFromURL,
	filtersToSearchParams,
	rangeToSeconds,
	deriveFacetCounts,
	mergeSourceFacet,
	deriveCountryFacet,
	formatTimestamp,
	extractSrcIp,
	computeVolumeTiers,
	computeVisibleRange,
	isScrolledToBottom
} from './search';
import type { LogEntry } from './server/siemApiClient';

describe('parseFiltersFromURL', () => {
	it('reads every filter field and defaults range to 24h', () => {
		const url = new URL('https://siem.townsville.cc/search?source=udm-ultra&severity=critical');
		const filters = parseFiltersFromURL(url);
		expect(filters).toEqual({
			source: 'udm-ultra',
			host: '',
			program: '',
			severity: 'critical',
			facility: '',
			q: '',
			range: '24h'
		});
	});

	it('accepts a valid range value and rejects an invalid one', () => {
		expect(parseFiltersFromURL(new URL('https://x/search?range=15m')).range).toBe('15m');
		expect(parseFiltersFromURL(new URL('https://x/search?range=bogus')).range).toBe('24h');
	});
});

describe('filtersToSearchParams', () => {
	it('omits empty fields and never includes range', () => {
		const params = filtersToSearchParams({
			source: 'udm-ultra',
			host: '',
			program: '',
			severity: '',
			facility: '',
			q: '',
			range: '24h'
		});
		expect(params).toEqual({ source: 'udm-ultra' });
	});
});

describe('rangeToSeconds', () => {
	it('maps each range value to the right number of seconds', () => {
		expect(rangeToSeconds('15m')).toBe(900);
		expect(rangeToSeconds('24h')).toBe(86400);
		expect(rangeToSeconds('7d')).toBe(604800);
	});
});

function fakeEntry(overrides: Partial<LogEntry> = {}): LogEntry {
	return {
		Timestamp: '2026-08-05T00:00:00Z',
		Labels: { severity: 'info', program: 'sshd' },
		Line: '{}',
		...overrides
	};
}

describe('deriveFacetCounts', () => {
	it('counts and sorts by frequency descending', () => {
		const entries = [
			fakeEntry({ Labels: { severity: 'critical' } }),
			fakeEntry({ Labels: { severity: 'critical' } }),
			fakeEntry({ Labels: { severity: 'warning' } })
		];
		expect(deriveFacetCounts(entries, 'severity')).toEqual([
			{ value: 'critical', count: 2 },
			{ value: 'warning', count: 1 }
		]);
	});

	it('skips entries missing the label', () => {
		expect(deriveFacetCounts([fakeEntry({ Labels: {} })], 'severity')).toEqual([]);
	});
});

describe('mergeSourceFacet', () => {
	function knownSource(name: string, displayName = '') {
		return { name, displayName };
	}

	it('adds a known claimed source not present in the current results at count 0', () => {
		const entries = [fakeEntry({ Labels: { source: 'udm-ultra' } })];
		expect(
			mergeSourceFacet(entries, [knownSource('udm-ultra'), knownSource('homebridge')])
		).toEqual([
			{ value: 'udm-ultra', label: 'udm-ultra', count: 1 },
			{ value: 'homebridge', label: 'homebridge', count: 0 }
		]);
	});

	it('does not duplicate a source already present in the derived counts', () => {
		const entries = [
			fakeEntry({ Labels: { source: 'udm-ultra' } }),
			fakeEntry({ Labels: { source: 'udm-ultra' } })
		];
		expect(mergeSourceFacet(entries, [knownSource('udm-ultra')])).toEqual([
			{ value: 'udm-ultra', label: 'udm-ultra', count: 2 }
		]);
	});

	it('sorts multiple missing sources alphabetically', () => {
		expect(
			mergeSourceFacet(
				[],
				[knownSource('tower'), knownSource('homebridge'), knownSource('raspberrypi')]
			)
		).toEqual([
			{ value: 'homebridge', label: 'homebridge', count: 0 },
			{ value: 'raspberrypi', label: 'raspberrypi', count: 0 },
			{ value: 'tower', label: 'tower', count: 0 }
		]);
	});

	it('prefers displayName as the label while keeping value as the raw source name, for both present and missing rows', () => {
		const entries = [fakeEntry({ Labels: { source: '192.168.3.223' } })];
		expect(
			mergeSourceFacet(entries, [
				knownSource('192.168.3.223', 'Home Assistant'),
				knownSource('192.168.3.68', 'Homebridge')
			])
		).toEqual([
			{ value: '192.168.3.223', label: 'Home Assistant', count: 1 },
			{ value: '192.168.3.68', label: 'Homebridge', count: 0 }
		]);
	});

	it('sorts missing sources by displayName, not the raw name, when both are known', () => {
		const result = mergeSourceFacet(
			[],
			[knownSource('192.168.3.223', 'Home Assistant'), knownSource('192.168.3.68', 'Homebridge')]
		);
		expect(result.map((f) => f.label)).toEqual(['Home Assistant', 'Homebridge']);
	});
});

describe('deriveCountryFacet', () => {
	it('extracts geoip.country_code from the parsed line and counts it', () => {
		const entries = [
			fakeEntry({ Line: '{"geoip":{"country_code":"US"}}' }),
			fakeEntry({ Line: '{"geoip":{"country_code":"US"}}' }),
			fakeEntry({ Line: '{"geoip":{"country_code":"DE"}}' })
		];
		expect(deriveCountryFacet(entries)).toEqual([
			{ value: 'US', count: 2 },
			{ value: 'DE', count: 1 }
		]);
	});

	it('does not fall back to a "cc" field (regression: that field never exists in real geoip data)', () => {
		expect(deriveCountryFacet([fakeEntry({ Line: '{"geoip":{"cc":"US"}}' })])).toEqual([]);
	});

	it('skips lines with no geoip.country_code, including malformed JSON', () => {
		expect(deriveCountryFacet([fakeEntry({ Line: 'not json' })])).toEqual([]);
	});
});

describe('formatTimestamp', () => {
	it('pads a timestamp with no fractional seconds to millisecond precision', () => {
		expect(formatTimestamp('2026-08-06T11:52:00Z')).toBe('2026-08-06T11:52:00.000Z');
	});

	it('truncates a full nanosecond-precision timestamp to milliseconds', () => {
		expect(formatTimestamp('2026-08-06T11:52:00.123456789Z')).toBe('2026-08-06T11:52:00.123Z');
	});

	it('pads a short fractional part out to three digits', () => {
		expect(formatTimestamp('2026-08-06T11:52:00.7Z')).toBe('2026-08-06T11:52:00.700Z');
	});

	it('leaves an already-millisecond-precision timestamp unchanged', () => {
		expect(formatTimestamp('2026-08-06T11:52:00.734Z')).toBe('2026-08-06T11:52:00.734Z');
	});

	it('returns the input unchanged if it does not match the expected shape', () => {
		expect(formatTimestamp('not a timestamp')).toBe('not a timestamp');
	});
});

describe('extractSrcIp', () => {
	it('extracts src_ip from a parsed line', () => {
		expect(extractSrcIp('{"src_ip":"10.0.0.5"}')).toBe('10.0.0.5');
	});

	it('returns null for malformed JSON or a missing field', () => {
		expect(extractSrcIp('not json')).toBeNull();
		expect(extractSrcIp('{}')).toBeNull();
	});
});

describe('computeVolumeTiers', () => {
	it('marks the top ~12% as critical and the next ~18% as warning', () => {
		const buckets = Array.from({ length: 10 }, (_, i) => ({
			bucket_start: `t${i}`,
			count: i + 1
		}));
		const tiers = computeVolumeTiers(buckets);
		expect(tiers[9]).toBe('critical');
		expect(tiers[0]).toBe('normal');
	});

	it('returns an empty array for no buckets', () => {
		expect(computeVolumeTiers([])).toEqual([]);
	});
});

describe('computeVisibleRange', () => {
	it('returns a range around the scroll position with a buffer', () => {
		// scrollTop=1000, rowHeight=25 -> first visible row is index 40.
		// containerHeight=500 -> ~20 rows visible.
		const range = computeVisibleRange(1000, 500, 25, 1000);
		expect(range.startIndex).toBeLessThanOrEqual(40);
		expect(range.endIndex).toBeGreaterThanOrEqual(60);
		expect(range.offsetTop).toBe(range.startIndex * 25);
	});

	it('clamps startIndex to 0 near the top', () => {
		const range = computeVisibleRange(0, 500, 25, 1000);
		expect(range.startIndex).toBe(0);
	});

	it('clamps endIndex to totalRows near the bottom', () => {
		const range = computeVisibleRange(100000, 500, 25, 50);
		expect(range.endIndex).toBe(50);
	});

	it('clamps startIndex when scrollTop overshoots totalRows (stale scroll after filter narrows results)', () => {
		// scrollTop=100000 with rowHeight=25 implies firstVisible=4000, far beyond
		// totalRows=50. Without clamping firstVisible to a valid row index,
		// startIndex (3990) would exceed endIndex (50), producing a blank table.
		const range = computeVisibleRange(100000, 500, 25, 50);
		expect(range.startIndex).toBeLessThanOrEqual(range.endIndex);
		expect(range.startIndex).toBe(39);
		expect(range.endIndex).toBe(50);
		expect(range.offsetTop).toBe(39 * 25);
	});

	it('returns an empty range for zero total rows', () => {
		expect(computeVisibleRange(0, 500, 25, 0)).toEqual({
			startIndex: 0,
			endIndex: 0,
			offsetTop: 0
		});
	});
});

describe('isScrolledToBottom', () => {
	it('is true when the container is exactly at the bottom', () => {
		expect(isScrolledToBottom(500, 300, 800)).toBe(true);
	});

	it('is true when within the default threshold of the bottom', () => {
		expect(isScrolledToBottom(497, 300, 800)).toBe(true);
	});

	it('is false when scrolled away from the bottom', () => {
		expect(isScrolledToBottom(0, 300, 800)).toBe(false);
	});

	it('honours a custom threshold', () => {
		expect(isScrolledToBottom(480, 300, 800, 25)).toBe(true);
		expect(isScrolledToBottom(470, 300, 800, 25)).toBe(false);
	});

	it('is true for content shorter than the container', () => {
		expect(isScrolledToBottom(0, 500, 200)).toBe(true);
	});
});
