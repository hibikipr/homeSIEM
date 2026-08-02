import { describe, it, expect } from 'vitest';
import { heatTierColor, topTriageAlerts, deriveCountryBreakdown } from './wall';
import type { AlertResponse, LogEntry } from './server/siemApiClient';

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

function alert(overrides: Partial<AlertResponse>): AlertResponse {
  return {
    id: 1,
    rule_id: 1,
    group_key: 'a',
    severity: 'low',
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
      alert({ id: 1, severity: 'low', last_seen_at: '2026-08-02T03:00:00Z' }),
      alert({ id: 2, severity: 'critical', last_seen_at: '2026-08-02T01:00:00Z' }),
      alert({ id: 3, severity: 'critical', last_seen_at: '2026-08-02T02:00:00Z' }),
      alert({ id: 4, severity: 'medium', last_seen_at: '2026-08-02T04:00:00Z' })
    ];

    const top = topTriageAlerts(alerts, 3);

    expect(top.map((a) => a.id)).toEqual([3, 2, 4]);
  });

  it('defaults to the top 3', () => {
    const alerts = [1, 2, 3, 4, 5].map((id) => alert({ id, severity: 'critical' }));
    expect(topTriageAlerts(alerts)).toHaveLength(3);
  });
});

describe('deriveCountryBreakdown', () => {
  function entry(line: string): LogEntry {
    return { Timestamp: '2026-08-02T00:00:00Z', Labels: {}, Line: line };
  }

  it('counts geoip.cc occurrences and sorts descending', () => {
    const entries = [
      entry('{"geoip":{"cc":"US"}}'),
      entry('{"geoip":{"cc":"US"}}'),
      entry('{"geoip":{"cc":"DE"}}')
    ];

    expect(deriveCountryBreakdown(entries)).toEqual([
      { country: 'US', count: 2 },
      { country: 'DE', count: 1 }
    ]);
  });

  it('skips entries with no geoip data or malformed JSON', () => {
    const entries = [entry('not json'), entry('{}'), entry('{"geoip":{}}'), entry('{"geoip":{"cc":"US"}}')];
    expect(deriveCountryBreakdown(entries)).toEqual([{ country: 'US', count: 1 }]);
  });
});
