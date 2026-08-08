import { describe, it, expect } from 'vitest';
import { computeChartPoints, formatHourLabel, CHART_WIDTH, CHART_HEIGHT } from './eventsOverTime';

describe('computeChartPoints', () => {
	it('scales count to the chart height, with the max count at y=0', () => {
		const totals = [
			{ hour_start: '2026-08-02T00:00:00Z', count: 10 },
			{ hour_start: '2026-08-02T01:00:00Z', count: 100 },
			{ hour_start: '2026-08-02T02:00:00Z', count: 50 }
		];

		const points = computeChartPoints(totals);

		expect(points).toHaveLength(3);
		expect(points[1].y).toBe(0); // max count -> top of chart
		expect(points[0].y).toBe(CHART_HEIGHT * 0.9); // 10/100 of the way up
		expect(points[2].y).toBe(CHART_HEIGHT * 0.5); // 50/100 of the way up
	});

	it('spaces points evenly across the chart width', () => {
		const totals = [
			{ hour_start: '2026-08-02T00:00:00Z', count: 1 },
			{ hour_start: '2026-08-02T01:00:00Z', count: 1 },
			{ hour_start: '2026-08-02T02:00:00Z', count: 1 }
		];

		const points = computeChartPoints(totals);

		expect(points[0].x).toBe(0);
		expect(points[1].x).toBe(CHART_WIDTH / 2);
		expect(points[2].x).toBe(CHART_WIDTH);
	});

	it('handles an all-zero series without dividing by zero', () => {
		const totals = [
			{ hour_start: '2026-08-02T00:00:00Z', count: 0 },
			{ hour_start: '2026-08-02T01:00:00Z', count: 0 }
		];

		const points = computeChartPoints(totals);

		expect(points.every((p) => Number.isFinite(p.y))).toBe(true);
	});

	it('returns an empty array for an empty series', () => {
		expect(computeChartPoints([])).toEqual([]);
	});
});

describe('formatHourLabel', () => {
	it('formats an ISO timestamp as HH:00 in UTC', () => {
		expect(formatHourLabel('2026-08-02T05:00:00Z')).toBe('05:00');
		expect(formatHourLabel('2026-08-02T00:00:00Z')).toBe('00:00');
		expect(formatHourLabel('2026-08-02T23:00:00Z')).toBe('23:00');
	});
});
