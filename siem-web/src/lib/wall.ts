import type { AlertSeverity } from './severity';
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

// HeatGrid's rows come from siem-api's /events/stats (heat_grid[].source),
// which - like every other place a raw source name reaches the frontend
// (see search.ts's FacetCount.label) - only ever has the raw name, never
// an operator-set display_name. Builds a name -> label lookup from the
// claimed sources list so HeatGrid can show the rename without needing to
// carry display_name through the stats endpoint itself.
export function buildSourceLabels(
	knownSources: { name: string; displayName: string }[]
): Record<string, string> {
	const labels: Record<string, string> = {};
	for (const s of knownSources) {
		labels[s.name] = s.displayName || s.name;
	}
	return labels;
}

const SEVERITY_RANK: Record<AlertSeverity, number> = { critical: 3, warning: 2, info: 1 };

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

		// Vector's geoip enrichment table (see enrich_geo in vector.toml)
		// names this field country_code - there's no "cc" field in the
		// actual enriched data at all, despite that being what this file
		// read for as long as this widget has existed. Found in production:
		// confirmed directly against a live captured event that .geoip.cc
		// was always undefined, so this widget could never have populated
		// even on a sample that did contain a geoip-enriched entry.
		const country = (geoip as Record<string, unknown>).country_code;
		if (typeof country !== 'string' || country === '') continue;

		counts.set(country, (counts.get(country) ?? 0) + 1);
	}

	return [...counts.entries()]
		.map(([country, count]) => ({ country, count }))
		.sort((a, b) => b.count - a.count);
}
