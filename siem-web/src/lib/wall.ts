import type { AlertSeverity } from './severity';
import type { AlertResponse } from './server/siemApiClient';

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

// WCAG 1.4.1 (use of color): the heatmap's tiers used to be distinguishable
// by hue alone, and critical-vs-warning (red vs. amber) is exactly the pair
// hardest to tell apart under red-green color blindness, the most common
// form. Each tier gets its own glyph too - shape/fill degree conveys the
// same "how busy" gradient color did, but survives grayscale or a color
// vision deficiency. "critical"/"warning" get an attention glyph rather
// than a dot on the fill scale, since those two specifically need to read
// as qualitatively different from "just busy," not merely more filled.
const HEAT_TIER_GLYPHS: Record<string, string> = {
	critical: '✕',
	warning: '!',
	busy: '●',
	light: '◐',
	quiet: '○',
	none: ''
};

export function heatTierGlyph(tier: string): string {
	return HEAT_TIER_GLYPHS[tier] ?? '';
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
