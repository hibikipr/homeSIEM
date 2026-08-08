export const CHART_WIDTH = 760;
export const CHART_HEIGHT = 140;

export interface ChartPoint {
	x: number;
	y: number;
	hourStart: string;
	count: number;
}

export function computeChartPoints(totals: { hour_start: string; count: number }[]): ChartPoint[] {
	if (totals.length === 0) return [];

	const maxCount = Math.max(1, ...totals.map((t) => t.count));

	return totals.map((t, i) => ({
		x: totals.length > 1 ? (i / (totals.length - 1)) * CHART_WIDTH : CHART_WIDTH / 2,
		y: CHART_HEIGHT - (t.count / maxCount) * CHART_HEIGHT,
		hourStart: t.hour_start,
		count: t.count
	}));
}

export function formatHourLabel(iso: string): string {
	const d = new Date(iso);
	return `${String(d.getUTCHours()).padStart(2, '0')}:00`;
}
