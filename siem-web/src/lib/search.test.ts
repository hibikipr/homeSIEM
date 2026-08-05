import { describe, it, expect } from 'vitest';
import {
	parseFiltersFromURL,
	filtersToSearchParams,
	rangeToSeconds,
	deriveFacetCounts,
	deriveCountryFacet,
	extractSrcIp,
	computeVolumeTiers
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

describe('deriveCountryFacet', () => {
	it('extracts geoip.cc from the parsed line and counts it', () => {
		const entries = [
			fakeEntry({ Line: '{"geoip":{"cc":"US"}}' }),
			fakeEntry({ Line: '{"geoip":{"cc":"US"}}' }),
			fakeEntry({ Line: '{"geoip":{"cc":"DE"}}' })
		];
		expect(deriveCountryFacet(entries)).toEqual([
			{ value: 'US', count: 2 },
			{ value: 'DE', count: 1 }
		]);
	});

	it('skips lines with no geoip.cc, including malformed JSON', () => {
		expect(deriveCountryFacet([fakeEntry({ Line: 'not json' })])).toEqual([]);
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
