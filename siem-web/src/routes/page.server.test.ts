import { describe, it, expect, vi } from 'vitest';
import { load } from './+page.server';
import * as siemApiClientModule from '$lib/server/siemApiClient';

vi.mock('$env/dynamic/private', () => ({ env: { API_URL: 'http://siem-api:8080' } }));

vi.mock('$lib/server/siemApiClient', async (importOriginal) => {
  const actual = await importOriginal<typeof import('$lib/server/siemApiClient')>();
  return { ...actual, SiemApiClient: vi.fn() };
});

describe('Wall load', () => {
  it('shapes siem-api responses into WallPageData', async () => {
    vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(
      function () {
        return {
          getEventsStats: vi.fn().mockResolvedValue({
            event_count_24h: 1240000,
            heat_grid: [{ source: 'udm-ultra', hours: ['critical', 'none'] }]
          }),
          getAlerts: vi.fn().mockResolvedValue([
            {
              id: 1,
              rule_id: 1,
              group_key: 'a',
              severity: 'critical',
              title: 't',
              body: 'b',
              event_count: 1,
              state: 'open',
              first_seen_at: '2026-08-02T00:00:00Z',
              last_seen_at: '2026-08-02T00:00:00Z'
            }
          ]),
          search: vi.fn().mockResolvedValue({
            logql: '{job="siem"}',
            count: 1,
            entries: [{ Timestamp: '2026-08-02T00:00:00Z', Labels: {}, Line: '{"geoip":{"cc":"US"}}' }]
          })
        };
      }
    );

    const result = (await load({ locals: { sessionToken: 'token-123' } } as never)) as any;

    expect(result.eventCount24h).toBe(1240000);
    expect(result.heatGrid).toEqual([{ source: 'udm-ultra', hours: ['critical', 'none'] }]);
    expect(result.openAlertCount).toBe(1);
    expect(result.triageAlerts).toHaveLength(1);
    expect(result.countryBreakdown).toEqual([{ country: 'US', count: 1 }]);
  });
});
