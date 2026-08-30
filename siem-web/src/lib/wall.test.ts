import { describe, it, expect } from 'vitest';
import { heatTierColor, heatTierGlyph, topTriageAlerts, buildSourceLabels } from './wall';
import type { AlertResponse } from './server/siemApiClient';

describe('heatTierColor', () => {
	it('maps every known tier to its token', () => {
		expect(heatTierColor('critical')).toBe('var(--color-severity-critical)');
		expect(heatTierColor('warning')).toBe('var(--color-severity-warning)');
		expect(heatTierColor('busy')).toBe('var(--color-accent-deep)');
		expect(heatTierColor('light')).toBe('var(--color-accent-tint-2)');
		expect(heatTierColor('quiet')).toBe('var(--color-accent-tint)');
		expect(heatTierColor('none')).toBe('var(--color-surface)');
	});

	it('falls back to the "none" color for an unrecognized tier', () => {
		expect(heatTierColor('bogus')).toBe('var(--color-surface)');
	});
});

describe('heatTierGlyph', () => {
	it('gives every known tier a distinct, non-empty glyph except "none"', () => {
		const tiers = ['critical', 'warning', 'busy', 'light', 'quiet'];
		const glyphs = tiers.map(heatTierGlyph);
		for (const g of glyphs) {
			expect(g.length).toBeGreaterThan(0);
		}
		// WCAG 1.4.1 (use of color): the whole point is that these must not
		// rely on color to tell apart, so no two tiers can share a glyph -
		// most importantly critical vs. warning, the pair hardest to
		// distinguish by hue under red-green color blindness.
		expect(new Set(glyphs).size).toBe(glyphs.length);
	});

	it('gives "none" an empty glyph - nothing to distinguish when there is no data', () => {
		expect(heatTierGlyph('none')).toBe('');
	});

	it('falls back to an empty glyph for an unrecognized tier', () => {
		expect(heatTierGlyph('bogus')).toBe('');
	});
});

describe('buildSourceLabels', () => {
	it('maps a source name to its display_name when set', () => {
		expect(buildSourceLabels([{ name: '192.168.3.223', displayName: 'Home Assistant' }])).toEqual({
			'192.168.3.223': 'Home Assistant'
		});
	});

	it('falls back to the raw name when displayName is empty', () => {
		expect(buildSourceLabels([{ name: 'udm-ultra', displayName: '' }])).toEqual({
			'udm-ultra': 'udm-ultra'
		});
	});

	it('returns an empty map for an empty input', () => {
		expect(buildSourceLabels([])).toEqual({});
	});
});

function alert(overrides: Partial<AlertResponse>): AlertResponse {
	return {
		id: 1,
		rule_id: 1,
		group_key: 'a',
		severity: 'info',
		title: 't',
		body: 'b',
		event_count: 1,
		state: 'open',
		first_seen_at: '2026-08-02T00:00:00Z',
		last_seen_at: '2026-08-02T00:00:00Z',
		...overrides
	};
}

describe('topTriageAlerts', () => {
	it('sorts by severity rank descending, then recency descending', () => {
		const alerts = [
			alert({ id: 1, severity: 'info', last_seen_at: '2026-08-02T03:00:00Z' }),
			alert({ id: 2, severity: 'critical', last_seen_at: '2026-08-02T01:00:00Z' }),
			alert({ id: 3, severity: 'critical', last_seen_at: '2026-08-02T02:00:00Z' }),
			alert({ id: 4, severity: 'warning', last_seen_at: '2026-08-02T04:00:00Z' })
		];

		const top = topTriageAlerts(alerts, 3);

		expect(top.map((a) => a.id)).toEqual([3, 2, 4]);
	});

	it('ranks warning above info even when info is more recent (regression: both used to rank 0)', () => {
		const alerts = [
			alert({ id: 1, severity: 'info', last_seen_at: '2026-08-02T05:00:00Z' }),
			alert({ id: 2, severity: 'warning', last_seen_at: '2026-08-02T01:00:00Z' })
		];

		const top = topTriageAlerts(alerts, 2);

		expect(top.map((a) => a.id)).toEqual([2, 1]);
	});

	it('defaults to the top 3', () => {
		const alerts = [1, 2, 3, 4, 5].map((id) => alert({ id, severity: 'critical' }));
		expect(topTriageAlerts(alerts)).toHaveLength(3);
	});
});
