import { describe, it, expect } from 'vitest';
import { filterBySeverity, serializeNdjson, severityColor } from './tail';
import type { LogEntry } from './server/siemApiClient';

function fakeEntry(overrides: Partial<LogEntry> = {}): LogEntry {
	return {
		Timestamp: '2026-08-05T00:00:00Z',
		Labels: { severity: 'info', host: 'h', program: 'p' },
		Line: 'hello',
		...overrides
	};
}

describe('filterBySeverity', () => {
	it('keeps only entries whose severity is in the active set', () => {
		const critEntry = fakeEntry({ Labels: { severity: 'crit' } });
		const debugEntry = fakeEntry({ Labels: { severity: 'debug' } });

		const result = filterBySeverity([critEntry, debugEntry], new Set(['crit']));

		expect(result).toEqual([critEntry]);
	});

	it('treats a missing severity label as info', () => {
		const noSeverity = fakeEntry({ Labels: {} });

		expect(filterBySeverity([noSeverity], new Set(['info']))).toEqual([noSeverity]);
		expect(filterBySeverity([noSeverity], new Set(['crit']))).toEqual([]);
	});

	it('returns an empty array when nothing matches', () => {
		expect(filterBySeverity([fakeEntry({ Labels: { severity: 'debug' } })], new Set(['crit']))).toEqual(
			[]
		);
	});
});

describe('serializeNdjson', () => {
	it('serializes entries as newline-delimited JSON matching the wire shape', () => {
		const entries = [fakeEntry({ Line: 'a' }), fakeEntry({ Line: 'b' })];

		const result = serializeNdjson(entries);
		const lines = result.split('\n');

		expect(lines).toHaveLength(2);
		expect(JSON.parse(lines[0])).toEqual(entries[0]);
		expect(JSON.parse(lines[1])).toEqual(entries[1]);
	});

	it('returns an empty string for no entries', () => {
		expect(serializeNdjson([])).toBe('');
	});
});

describe('severityColor', () => {
	it('maps the three most-severe syslog levels to the critical token', () => {
		expect(severityColor('emerg')).toBe('var(--color-severity-critical)');
		expect(severityColor('alert')).toBe('var(--color-severity-critical)');
		expect(severityColor('crit')).toBe('var(--color-severity-critical)');
	});

	it('maps warning and debug to their own tokens', () => {
		expect(severityColor('warning')).toBe('var(--color-severity-warning)');
		expect(severityColor('debug')).toBe('var(--color-muted-2)');
	});

	it('falls back to the info token for an unrecognized severity', () => {
		expect(severityColor('bogus')).toBe('var(--color-severity-info)');
	});
});
