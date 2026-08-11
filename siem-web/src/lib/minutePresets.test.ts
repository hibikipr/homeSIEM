import { describe, it, expect } from 'vitest';
import { formatMinuteLabel, formatSecondsAsMinutes } from './minutePresets';

describe('formatMinuteLabel', () => {
	it('formats sub-hour values as minutes', () => {
		expect(formatMinuteLabel(1)).toBe('1 minute');
		expect(formatMinuteLabel(5)).toBe('5 minutes');
		expect(formatMinuteLabel(30)).toBe('30 minutes');
	});

	it('collapses clean hour values to hours', () => {
		expect(formatMinuteLabel(60)).toBe('1 hour');
		expect(formatMinuteLabel(120)).toBe('2 hours');
		expect(formatMinuteLabel(360)).toBe('6 hours');
	});

	it('collapses clean day values to days', () => {
		expect(formatMinuteLabel(1440)).toBe('1 day');
		expect(formatMinuteLabel(2880)).toBe('2 days');
	});

	it('falls back to minutes for values that are not a clean unit', () => {
		expect(formatMinuteLabel(90)).toBe('90 minutes');
	});
});

describe('formatSecondsAsMinutes', () => {
	it('formats a whole-minute value via formatMinuteLabel', () => {
		expect(formatSecondsAsMinutes(900)).toBe('15 minutes');
		expect(formatSecondsAsMinutes(3600)).toBe('1 hour');
	});

	it('falls back to raw seconds for a value that is not a clean minute multiple', () => {
		expect(formatSecondsAsMinutes(90)).toBe('90s');
	});
});
