import { describe, it, expect } from 'vitest';
import { parseLogLine, extractField, extractMessage, formatTimestampInZone } from './logline';

describe('parseLogLine', () => {
	it('parses a JSON object line', () => {
		expect(parseLogLine('{"a":"b"}')).toEqual({ a: 'b' });
	});

	it('returns null for malformed JSON', () => {
		expect(parseLogLine('not json')).toBeNull();
	});

	it('returns null for valid JSON that is not an object (array, primitive, null)', () => {
		expect(parseLogLine('[1,2,3]')).toBeNull();
		expect(parseLogLine('"just a string"')).toBeNull();
		expect(parseLogLine('null')).toBeNull();
	});
});

describe('extractField', () => {
	it('extracts a top-level string field', () => {
		expect(extractField('{"facility":"user"}', 'facility')).toBe('user');
	});

	it('returns null when the field is missing, non-string, or the line is malformed', () => {
		expect(extractField('{}', 'facility')).toBeNull();
		expect(extractField('{"facility":42}', 'facility')).toBeNull();
		expect(extractField('not json', 'facility')).toBeNull();
	});
});

describe('extractMessage', () => {
	it('extracts .message from the parsed line', () => {
		expect(extractMessage('{"message":"hello world"}')).toBe('hello world');
	});

	it('falls back to the raw line when there is no .message field or the line is not JSON', () => {
		expect(extractMessage('{"other":"field"}')).toBe('{"other":"field"}');
		expect(extractMessage('plain text line')).toBe('plain text line');
	});
});

describe('formatTimestampInZone', () => {
	it('converts a UTC timestamp to the given IANA time zone', () => {
		// 2026-08-10T04:30:00Z is 2026-08-10T00:30:00 in America/New_York (EDT, UTC-4).
		expect(formatTimestampInZone('2026-08-10T04:30:00Z', 'America/New_York')).toBe(
			'2026-08-10 00:30:00'
		);
	});

	it('handles a date that rolls to the previous day in the target zone', () => {
		// 2026-08-10T02:00:00Z is 2026-08-09T22:00:00 in America/New_York.
		expect(formatTimestampInZone('2026-08-10T02:00:00Z', 'America/New_York')).toBe(
			'2026-08-09 22:00:00'
		);
	});

	it('leaves the timestamp effectively unchanged (formatted) for UTC', () => {
		expect(formatTimestampInZone('2026-08-10T04:30:00Z', 'UTC')).toBe('2026-08-10 04:30:00');
	});

	it('returns the input unchanged for an unparseable timestamp', () => {
		expect(formatTimestampInZone('not a timestamp', 'America/New_York')).toBe('not a timestamp');
	});

	it('returns the input unchanged for an invalid time zone identifier', () => {
		expect(formatTimestampInZone('2026-08-10T04:30:00Z', 'Not/A_Zone')).toBe(
			'2026-08-10T04:30:00Z'
		);
	});
});
