import type { AlertResponse, LogEntry } from './server/siemApiClient';

const HEAT_TIER_COLORS: Record<string, string> = {
	critical: 'var(--color-severity-critical)',
	warning: 'var(--color-severity-warning)',
	busy: 'var(--color-accent-deep)',
	light: 'var(--color-accent-tint-2)',
	quiet: 'var(--color-accent-tint)',
	none: 'var(--color-surface)'
};

export function heatTierColor(tier: string): string {
	return HEAT_TIER_COLORS[tier] ?? HEAT_TIER_COLORS.none;
}

const SEVERITY_RANK: Record<string, number> = { critical: 3, warning: 2, info: 1 };

export function topTriageAlerts(alerts: AlertResponse[], count = 3): AlertResponse[] {
	return [...alerts]
		.sort((a, b) => {
			const rankDiff = (SEVERITY_RANK[b.severity] ?? 0) - (SEVERITY_RANK[a.severity] ?? 0);
			if (rankDiff !== 0) return rankDiff;
			return new Date(b.last_seen_at).getTime() - new Date(a.last_seen_at).getTime();
		})
		.slice(0, count);
}

export interface CountryCount {
	country: string;
	count: number;
}

export function deriveCountryBreakdown(entries: LogEntry[]): CountryCount[] {
	const counts = new Map<string, number>();

	for (const entry of entries) {
		let parsed: unknown;
		try {
			parsed = JSON.parse(entry.Line);
		} catch {
			continue;
		}
		if (typeof parsed !== 'object' || parsed === null) continue;

		const geoip = (parsed as Record<string, unknown>).geoip;
		if (typeof geoip !== 'object' || geoip === null) continue;

		const country = (geoip as Record<string, unknown>).cc;
		if (typeof country !== 'string' || country === '') continue;

		counts.set(country, (counts.get(country) ?? 0) + 1);
	}

	return [...counts.entries()]
		.map(([country, count]) => ({ country, count }))
		.sort((a, b) => b.count - a.count);
}
