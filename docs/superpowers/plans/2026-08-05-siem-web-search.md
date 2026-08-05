# siem-web: Search screen — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship the Search screen (`/search`) — a query bar, a volume ribbon, a facet rail,
a real virtualized result table (10,000-row capable), and an event inspector with rule
creation — per `docs/superpowers/specs/2026-08-05-siem-web-search-design.md`.

**Architecture:** One siem-api addition (`GET /events/search` gains a `volume` field, a
real Loki-side bucketed count query) plus a SvelteKit route that fetches up to 10,000
entries in one SSR call and derives everything else — facets, table windowing, the
"Filter to SRC" / context-callout lookups — client-side or via one more bounded search
call in the same load function. Rule creation (`POST /rules`) already exists and needs no
changes.

**Tech Stack:** Go 1.x (siem-api), SvelteKit 5 + TypeScript + Vitest (siem-web). No new
dependencies — virtualization is plain fixed-row-height windowed rendering, no library.

## Global Constraints

- Response field JSON names are snake_case (`volume`, `bucket_start`, etc.), matching
  every existing siem-api response type.
- The query bar renders as plain labeled filter inputs (source/host/program/severity/
  facility/free-text) plus a Search button and a 15m/24h/7d segmented control — **not**
  the mockup's inline-editable token-pill UI. Same underlying filtering capability (all
  six fields, same compiled LogQL), simpler first pass — this codebase has no existing
  `<input>`/`<form>` precedent anywhere yet, so this plan establishes a plain one rather
  than a novel pill-editor interaction.
- "Filter to SRC" and the inspector's "Context callout" both reuse the existing `q`
  (free-text, `|= "..."` line-filter) parameter — **no backend change for either.** Only
  the volume-bucket addition is a real backend change this plan makes.
- The Source-country facet is **display-only** (no click-to-filter) — `GET /events/search`
  has no country/geoip query parameter to filter by. Severity and Program facets *are*
  clickable-to-filter, since both are real query params.
- Virtualization is fixed-row-height windowed rendering in plain Svelte (absolutely
  positioned rows inside a spacer div sized to `totalRows × rowHeight`) — no new
  dependency. `computeVisibleRange` (the windowing math) is a pure, TDD'd function.
- No unit tests for Svelte components — presentational/DOM-driven, matching every prior
  screen's convention. `search.ts`'s pure helpers are TDD'd; `+page.server.ts` load
  functions get Vitest coverage matching the Alerts/Sources/Live-tail precedent.
- Run `npm run check`, `npm run lint`, and `npm run test:unit -- --run` — all three,
  separately — before considering any siem-web task done. (`npm run check` passing does
  not mean lint passes; this bit two tasks in the Live-tail plan.)

---

### Task 1: siem-api — `GET /events/search` gains `volume`

**Files:**
- Modify: `siem-api/internal/api/events.go`
- Modify: `siem-api/internal/api/events_test.go`

**Interfaces:**
- Consumes: `s.deps.Loki.QueryMatrix(ctx, logql string, start, end time.Time, step
  time.Duration) (loki.MatrixResult, error)` — already used identically in `stats.go`.
- Produces: `searchResponse.Volume []volumeBucket` (`volumeBucket{BucketStart time.Time,
  Count int64}`, JSON `{bucket_start, count}`), always a non-nil (possibly empty) slice —
  never `null` in the JSON response, and never fails the request if the volume sub-query
  errors (degrades to `[]`, same "supplementary data never 502s the primary response"
  precedent as Sources' ingest-health panel).

- [ ] **Step 1: Write the failing tests**

Add to `siem-api/internal/api/events_test.go` (create the file if it doesn't exist yet —
check first; if `handleEventsSearch` has no existing test file, add the necessary imports:
`encoding/json`, `net/http`, `net/http/httptest`, `strings`, `testing`, and
`github.com/hibikipr/homeSIEM/siem-api/internal/loki`):

```go
func TestEventsSearch_IncludesVolumeBuckets(t *testing.T) {
	fakeLoki := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("query")
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(query, "count_over_time") {
			w.Write([]byte(`{"status":"success","data":{"resultType":"matrix","result":[
				{"metric":{},"values":[[1700000000,"3"],[1700000300,"7"]]}
			]}}`))
			return
		}
		w.Write([]byte(`{"status":"success","data":{"result":[]}}`))
	}))
	defer fakeLoki.Close()

	s, st := newTestServer(t)
	s.deps.Loki = loki.New(fakeLoki.URL, fakeLoki.Client())

	token := authToken(t, st, "viewer", 100)
	req := httptest.NewRequest(http.MethodGet, "/events/search?start=2023-11-14T00:00:00Z&end=2023-11-14T01:00:00Z", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	var resp searchResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if len(resp.Volume) != 2 || resp.Volume[0].Count != 3 || resp.Volume[1].Count != 7 {
		t.Fatalf("Volume = %+v", resp.Volume)
	}
}

func TestEventsSearch_SucceedsWhenVolumeQueryFails(t *testing.T) {
	fakeLoki := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("query")
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(query, "count_over_time") {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Write([]byte(`{"status":"success","data":{"result":[
			{"stream":{"job":"siem"},"values":[["1700000000000000000","hello"]]}
		]}}`))
	}))
	defer fakeLoki.Close()

	s, st := newTestServer(t)
	s.deps.Loki = loki.New(fakeLoki.URL, fakeLoki.Client())

	token := authToken(t, st, "viewer", 100)
	req := httptest.NewRequest(http.MethodGet, "/events/search", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (volume failure must not fail the whole request), body=%s", rec.Code, rec.Body.String())
	}
	var resp searchResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if resp.Count != 1 {
		t.Fatalf("Count = %d, want 1", resp.Count)
	}
	if resp.Volume == nil || len(resp.Volume) != 0 {
		t.Fatalf("Volume = %+v, want a non-nil empty slice", resp.Volume)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd siem-api && go test ./internal/api/... -run TestEventsSearch`
Expected: FAIL — `searchResponse` has no field `Volume` (compile error).

- [ ] **Step 3: Implement the handler changes**

In `siem-api/internal/api/events.go`, replace the `searchResponse` struct and
`handleEventsSearch`, and add the new helper, as follows:

```go
type volumeBucket struct {
	BucketStart time.Time `json:"bucket_start"`
	Count       int64     `json:"count"`
}

type searchResponse struct {
	LogQL   string          `json:"logql"`
	Count   int             `json:"count"`
	Entries []loki.LogEntry `json:"entries"`
	Volume  []volumeBucket  `json:"volume"`
}

func (s *Server) handleEventsSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	filters := loki.Filters{
		Source:   q.Get("source"),
		Host:     q.Get("host"),
		Program:  q.Get("program"),
		Severity: q.Get("severity"),
		Facility: q.Get("facility"),
		FreeText: q.Get("q"),
	}
	logql := loki.BuildQuery(s.deps.JobLabel, filters)

	end := time.Now().UTC()
	start := end.Add(-24 * time.Hour)
	if v := q.Get("start"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			start = t
		}
	}
	if v := q.Get("end"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			end = t
		}
	}
	limit := 1000
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}

	result, err := s.deps.Loki.QueryRange(r.Context(), logql, start, end, limit)
	if err != nil {
		s.deps.Logger.Error("events search: query failed", "error", err)
		http.Error(w, "query failed", http.StatusBadGateway)
		return
	}

	volume, err := s.queryVolumeBuckets(r.Context(), logql, start, end)
	if err != nil {
		s.deps.Logger.Error("events search: volume query failed", "error", err)
		volume = []volumeBucket{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(searchResponse{LogQL: logql, Count: len(result.Entries), Entries: result.Entries, Volume: volume})
}

func (s *Server) handleEventsTail(w http.ResponseWriter, r *http.Request) {
	s.deps.Hub.ServeHTTP("tail", w, r)
}

const volumeBucketCount = 72

// queryVolumeBuckets returns real Loki-side counts (not derived from the
// capped entries sample above, since results are limited but true event
// volume in a busy window may exceed that limit) across `volumeBucketCount`
// evenly-sized buckets spanning [start, end].
func (s *Server) queryVolumeBuckets(ctx context.Context, logql string, start, end time.Time) ([]volumeBucket, error) {
	total := end.Sub(start)
	if total <= 0 {
		return []volumeBucket{}, nil
	}
	bucketWidth := total / volumeBucketCount
	if bucketWidth < time.Second {
		bucketWidth = time.Second
	}
	bucketSeconds := int64(math.Ceil(bucketWidth.Seconds()))
	countLogQL := fmt.Sprintf("count_over_time(%s[%ds])", logql, bucketSeconds)

	result, err := s.deps.Loki.QueryMatrix(ctx, countLogQL, start, end, time.Duration(bucketSeconds)*time.Second)
	if err != nil {
		return nil, err
	}
	if len(result.Series) == 0 {
		return []volumeBucket{}, nil
	}

	buckets := make([]volumeBucket, len(result.Series[0].Samples))
	for i, sample := range result.Series[0].Samples {
		buckets[i] = volumeBucket{BucketStart: sample.Timestamp, Count: int64(sample.Value)}
	}
	return buckets, nil
}
```

Add `"fmt"` and `"math"` to `events.go`'s import block (both new — the file currently
imports `encoding/json`, `net/http`, `strconv`, `time`, and the `loki` package).

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd siem-api && go test ./...`
Expected: PASS (whole suite, including the new volume tests and every pre-existing
`events_test.go`/`stats_test.go` case).

- [ ] **Step 5: Commit**

```bash
git add siem-api/internal/api/events.go siem-api/internal/api/events_test.go
git commit -m "Add volume buckets to GET /events/search"
```

---

### Task 2: siem-web — `search.ts` filter/facet/color helpers

**Files:**
- Create: `siem-web/src/lib/search.ts`
- Create: `siem-web/src/lib/search.test.ts`

**Interfaces:**
- Consumes: `LogEntry` from `siemApiClient.ts` (existing).
- Produces: `SearchFilters`, `parseFiltersFromURL`, `filtersToSearchParams`,
  `rangeToSeconds`, `FacetCount`, `deriveFacetCounts`, `deriveCountryFacet`,
  `extractSrcIp`, `computeVolumeTiers` — consumed by Task 6's load function and Tasks 7-10's
  components.

- [ ] **Step 1: Write the failing tests**

Create `siem-web/src/lib/search.test.ts`:

```ts
import { describe, it, expect } from 'vitest';
import {
	parseFiltersFromURL,
	filtersToSearchParams,
	rangeToSeconds,
	deriveFacetCounts,
	deriveCountryFacet,
	extractSrcIp,
	computeVolumeTiers
} from './search';
import type { LogEntry } from './server/siemApiClient';

describe('parseFiltersFromURL', () => {
	it('reads every filter field and defaults range to 24h', () => {
		const url = new URL('https://siem.townsville.cc/search?source=udm-ultra&severity=critical');
		const filters = parseFiltersFromURL(url);
		expect(filters).toEqual({
			source: 'udm-ultra',
			host: '',
			program: '',
			severity: 'critical',
			facility: '',
			q: '',
			range: '24h'
		});
	});

	it('accepts a valid range value and rejects an invalid one', () => {
		expect(parseFiltersFromURL(new URL('https://x/search?range=15m')).range).toBe('15m');
		expect(parseFiltersFromURL(new URL('https://x/search?range=bogus')).range).toBe('24h');
	});
});

describe('filtersToSearchParams', () => {
	it('omits empty fields and never includes range', () => {
		const params = filtersToSearchParams({
			source: 'udm-ultra',
			host: '',
			program: '',
			severity: '',
			facility: '',
			q: '',
			range: '24h'
		});
		expect(params).toEqual({ source: 'udm-ultra' });
	});
});

describe('rangeToSeconds', () => {
	it('maps each range value to the right number of seconds', () => {
		expect(rangeToSeconds('15m')).toBe(900);
		expect(rangeToSeconds('24h')).toBe(86400);
		expect(rangeToSeconds('7d')).toBe(604800);
	});
});

function fakeEntry(overrides: Partial<LogEntry> = {}): LogEntry {
	return {
		Timestamp: '2026-08-05T00:00:00Z',
		Labels: { severity: 'info', program: 'sshd' },
		Line: '{}',
		...overrides
	};
}

describe('deriveFacetCounts', () => {
	it('counts and sorts by frequency descending', () => {
		const entries = [
			fakeEntry({ Labels: { severity: 'critical' } }),
			fakeEntry({ Labels: { severity: 'critical' } }),
			fakeEntry({ Labels: { severity: 'warning' } })
		];
		expect(deriveFacetCounts(entries, 'severity')).toEqual([
			{ value: 'critical', count: 2 },
			{ value: 'warning', count: 1 }
		]);
	});

	it('skips entries missing the label', () => {
		expect(deriveFacetCounts([fakeEntry({ Labels: {} })], 'severity')).toEqual([]);
	});
});

describe('deriveCountryFacet', () => {
	it('extracts geoip.cc from the parsed line and counts it', () => {
		const entries = [
			fakeEntry({ Line: '{"geoip":{"cc":"US"}}' }),
			fakeEntry({ Line: '{"geoip":{"cc":"US"}}' }),
			fakeEntry({ Line: '{"geoip":{"cc":"DE"}}' })
		];
		expect(deriveCountryFacet(entries)).toEqual([
			{ value: 'US', count: 2 },
			{ value: 'DE', count: 1 }
		]);
	});

	it('skips lines with no geoip.cc, including malformed JSON', () => {
		expect(deriveCountryFacet([fakeEntry({ Line: 'not json' })])).toEqual([]);
	});
});

describe('extractSrcIp', () => {
	it('extracts src_ip from a parsed line', () => {
		expect(extractSrcIp('{"src_ip":"10.0.0.5"}')).toBe('10.0.0.5');
	});

	it('returns null for malformed JSON or a missing field', () => {
		expect(extractSrcIp('not json')).toBeNull();
		expect(extractSrcIp('{}')).toBeNull();
	});
});

describe('computeVolumeTiers', () => {
	it('marks the top ~12% as critical and the next ~18% as warning', () => {
		const buckets = Array.from({ length: 10 }, (_, i) => ({
			bucket_start: `t${i}`,
			count: i + 1
		}));
		const tiers = computeVolumeTiers(buckets);
		expect(tiers[9]).toBe('critical');
		expect(tiers[0]).toBe('normal');
	});

	it('returns an empty array for no buckets', () => {
		expect(computeVolumeTiers([])).toEqual([]);
	});
});
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd siem-web && npm run test:unit -- --run search.test`
Expected: FAIL — `Cannot find module './search'`.

- [ ] **Step 3: Implement the helpers**

Create `siem-web/src/lib/search.ts`:

```ts
import type { LogEntry } from './server/siemApiClient';

export interface SearchFilters {
	source: string;
	host: string;
	program: string;
	severity: string;
	facility: string;
	q: string;
	range: '15m' | '24h' | '7d';
}

export function parseFiltersFromURL(url: URL): SearchFilters {
	const params = url.searchParams;
	const range = params.get('range');
	return {
		source: params.get('source') ?? '',
		host: params.get('host') ?? '',
		program: params.get('program') ?? '',
		severity: params.get('severity') ?? '',
		facility: params.get('facility') ?? '',
		q: params.get('q') ?? '',
		range: range === '15m' || range === '7d' ? range : '24h'
	};
}

export function filtersToSearchParams(filters: SearchFilters): Record<string, string> {
	const params: Record<string, string> = {};
	if (filters.source) params.source = filters.source;
	if (filters.host) params.host = filters.host;
	if (filters.program) params.program = filters.program;
	if (filters.severity) params.severity = filters.severity;
	if (filters.facility) params.facility = filters.facility;
	if (filters.q) params.q = filters.q;
	return params;
}

export function rangeToSeconds(range: SearchFilters['range']): number {
	switch (range) {
		case '15m':
			return 15 * 60;
		case '7d':
			return 7 * 24 * 60 * 60;
		default:
			return 24 * 60 * 60;
	}
}

export interface FacetCount {
	value: string;
	count: number;
}

export function deriveFacetCounts(entries: LogEntry[], labelKey: string): FacetCount[] {
	const counts = new Map<string, number>();
	for (const entry of entries) {
		const value = entry.Labels[labelKey];
		if (!value) continue;
		counts.set(value, (counts.get(value) ?? 0) + 1);
	}
	return [...counts.entries()]
		.map(([value, count]) => ({ value, count }))
		.sort((a, b) => b.count - a.count);
}

function parseLine(line: string): Record<string, unknown> | null {
	try {
		const parsed = JSON.parse(line);
		return typeof parsed === 'object' && parsed !== null ? (parsed as Record<string, unknown>) : null;
	} catch {
		return null;
	}
}

export function deriveCountryFacet(entries: LogEntry[]): FacetCount[] {
	const counts = new Map<string, number>();
	for (const entry of entries) {
		const parsed = parseLine(entry.Line);
		if (!parsed) continue;
		const geoip = parsed.geoip;
		if (typeof geoip !== 'object' || geoip === null) continue;
		const country = (geoip as Record<string, unknown>).cc;
		if (typeof country !== 'string' || country === '') continue;
		counts.set(country, (counts.get(country) ?? 0) + 1);
	}
	return [...counts.entries()]
		.map(([value, count]) => ({ value, count }))
		.sort((a, b) => b.count - a.count);
}

export function extractSrcIp(line: string): string | null {
	const parsed = parseLine(line);
	if (!parsed) return null;
	const value = parsed.src_ip;
	return typeof value === 'string' ? value : null;
}

export interface VolumeBucketLike {
	bucket_start: string;
	count: number;
}

export function computeVolumeTiers(buckets: VolumeBucketLike[]): Array<'normal' | 'warning' | 'critical'> {
	if (buckets.length === 0) return [];
	const sorted = buckets.map((b) => b.count).sort((a, b) => a - b);
	const percentile = (p: number) => sorted[Math.min(sorted.length - 1, Math.floor(sorted.length * p))];
	const warningThreshold = percentile(0.7);
	const criticalThreshold = percentile(0.88);
	return buckets.map((b) => {
		if (b.count > criticalThreshold) return 'critical';
		if (b.count > warningThreshold) return 'warning';
		return 'normal';
	});
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd siem-web && npm run test:unit -- --run search.test`
Expected: PASS (12 tests).

- [ ] **Step 5: Commit**

```bash
git add siem-web/src/lib/search.ts siem-web/src/lib/search.test.ts
git commit -m "Add search.ts: filter/facet/color helpers"
```

---

### Task 3: siem-web — `search.ts` virtualization math

**Files:**
- Modify: `siem-web/src/lib/search.ts`
- Modify: `siem-web/src/lib/search.test.ts`

**Interfaces:**
- Produces: `VisibleRange`, `computeVisibleRange(scrollTop, containerHeight, rowHeight,
  totalRows)` — consumed by Task 9's `ResultTable.svelte`.

- [ ] **Step 1: Write the failing tests**

Add to `siem-web/src/lib/search.test.ts`:

```ts
import { computeVisibleRange } from './search';

describe('computeVisibleRange', () => {
	it('returns a range around the scroll position with a buffer', () => {
		// scrollTop=1000, rowHeight=25 -> first visible row is index 40.
		// containerHeight=500 -> ~20 rows visible.
		const range = computeVisibleRange(1000, 500, 25, 1000);
		expect(range.startIndex).toBeLessThanOrEqual(40);
		expect(range.endIndex).toBeGreaterThanOrEqual(60);
		expect(range.offsetTop).toBe(range.startIndex * 25);
	});

	it('clamps startIndex to 0 near the top', () => {
		const range = computeVisibleRange(0, 500, 25, 1000);
		expect(range.startIndex).toBe(0);
	});

	it('clamps endIndex to totalRows near the bottom', () => {
		const range = computeVisibleRange(100000, 500, 25, 50);
		expect(range.endIndex).toBe(50);
	});

	it('returns an empty range for zero total rows', () => {
		expect(computeVisibleRange(0, 500, 25, 0)).toEqual({ startIndex: 0, endIndex: 0, offsetTop: 0 });
	});
});
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd siem-web && npm run test:unit -- --run search.test`
Expected: FAIL — `computeVisibleRange is not a function`.

- [ ] **Step 3: Implement the function**

Add to `siem-web/src/lib/search.ts`:

```ts
export interface VisibleRange {
	startIndex: number;
	endIndex: number; // exclusive
	offsetTop: number;
}

const VIRTUALIZATION_BUFFER_ROWS = 10;

export function computeVisibleRange(
	scrollTop: number,
	containerHeight: number,
	rowHeight: number,
	totalRows: number
): VisibleRange {
	if (totalRows === 0 || rowHeight <= 0) {
		return { startIndex: 0, endIndex: 0, offsetTop: 0 };
	}
	const firstVisible = Math.floor(scrollTop / rowHeight);
	const visibleCount = Math.ceil(containerHeight / rowHeight);
	const startIndex = Math.max(0, firstVisible - VIRTUALIZATION_BUFFER_ROWS);
	const endIndex = Math.min(totalRows, firstVisible + visibleCount + VIRTUALIZATION_BUFFER_ROWS);
	return { startIndex, endIndex, offsetTop: startIndex * rowHeight };
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd siem-web && npm run test:unit -- --run search.test`
Expected: PASS (16 tests total in this file).

- [ ] **Step 5: Commit**

```bash
git add siem-web/src/lib/search.ts siem-web/src/lib/search.test.ts
git commit -m "Add computeVisibleRange: fixed-row-height virtualization math"
```

---

### Task 4: siem-web — `SiemApiClient` additions

**Files:**
- Modify: `siem-web/src/lib/server/siemApiClient.ts`
- Modify: `siem-web/src/lib/server/siemApiClient.test.ts`

**Interfaces:**
- Produces: `VolumeBucket` type, `SearchResponse.volume` field, `CreateRuleRequest` type,
  `SiemApiClient.createRule` method — consumed by Task 6's load function and Task 5's
  proxy route.

- [ ] **Step 1: Write the failing tests**

Add to `siem-web/src/lib/server/siemApiClient.test.ts`:

```ts
it('search parses the volume field from the response', async () => {
	const fetchFn = fakeFetch({
		logql: '{job="siem"}',
		count: 1,
		entries: [],
		volume: [{ bucket_start: '2026-08-05T00:00:00Z', count: 3 }]
	});
	const client = new SiemApiClient({ baseUrl: 'http://siem-api:8080' }, fetchFn);

	const result = await client.search('token-123', {});

	expect(result.volume).toEqual([{ bucket_start: '2026-08-05T00:00:00Z', count: 3 }]);
});

it('createRule POSTs to /rules with Authorization and parses the response', async () => {
	const fetchFn = fakeFetch(
		{
			id: 9,
			name: 'search-alert',
			shape: 'threshold',
			logql: '{job="siem"}',
			window_sec: 60,
			group_by: [],
			severity: 'warning',
			destinations: ['inapp'],
			cooldown_sec: 3600,
			interval_sec: 60,
			enabled: true
		},
		201
	);
	const client = new SiemApiClient({ baseUrl: 'http://siem-api:8080' }, fetchFn);

	const result = await client.createRule('token-123', {
		name: 'search-alert',
		shape: 'threshold',
		logql: '{job="siem"}',
		window_sec: 60,
		group_by: [],
		severity: 'warning',
		destinations: ['inapp'],
		cooldown_sec: 3600,
		interval_sec: 60,
		enabled: true
	});

	expect(result.id).toBe(9);
	const [url, init] = fetchFn.mock.calls[0];
	expect(url).toBe('http://siem-api:8080/rules');
	expect(init?.method).toBe('POST');
	expect((init?.headers as Record<string, string>).Authorization).toBe('Bearer token-123');
});
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd siem-web && npm run test:unit -- --run siemApiClient`
Expected: FAIL — `result.volume` is `undefined` (first test doesn't fail hard but the
assertion fails); `client.createRule is not a function` (second test).

- [ ] **Step 3: Implement the client additions**

In `siem-web/src/lib/server/siemApiClient.ts`, add near the other response interfaces
(after `SearchResponse`):

```ts
export interface VolumeBucket {
	bucket_start: string;
	count: number;
}
```

Update `SearchResponse` to include it:

```ts
export interface SearchResponse {
	logql: string;
	count: number;
	entries: LogEntry[];
	volume: VolumeBucket[];
}
```

Add near `RuleResponse`:

```ts
export interface CreateRuleRequest {
	name: string;
	shape: string;
	logql: string;
	window_sec: number;
	threshold?: number;
	group_by: string[];
	severity: string;
	destinations: string[];
	cooldown_sec: number;
	interval_sec: number;
	enabled: boolean;
}
```

Add to the `SiemApiClient` class (after `getRules`):

```ts
	async createRule(sessionToken: string, req: CreateRuleRequest): Promise<RuleResponse> {
		return this.request<RuleResponse>('/rules', {
			method: 'POST',
			headers: { 'Content-Type': 'application/json', ...this.authInit(sessionToken).headers },
			body: JSON.stringify(req)
		});
	}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd siem-web && npm run test:unit -- --run siemApiClient`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add siem-web/src/lib/server/siemApiClient.ts siem-web/src/lib/server/siemApiClient.test.ts
git commit -m "Add volume field and createRule to SiemApiClient"
```

---

### Task 5: siem-web — rule-creation proxy route

**Files:**
- Create: `siem-web/src/routes/api/search/rules/+server.ts`
- Create: `siem-web/src/routes/api/search/rules/server.test.ts`

**Interfaces:**
- Consumes: `Task 4`'s `SiemApiClient.createRule`.
- Produces: `POST /api/search/rules`, consumed by `Task 10`'s `RuleFromEventForm.svelte`.

- [ ] **Step 1: Write the failing tests**

Create `siem-web/src/routes/api/search/rules/server.test.ts`:

```ts
import { describe, it, expect, vi } from 'vitest';
import { POST } from './+server';
import * as siemApiClientModule from '$lib/server/siemApiClient';

vi.mock('$env/dynamic/private', () => ({ env: { API_URL: 'http://siem-api:8080' } }));

vi.mock('$lib/server/siemApiClient', async (importOriginal) => {
	const actual = await importOriginal<typeof import('$lib/server/siemApiClient')>();
	return { ...actual, SiemApiClient: vi.fn() };
});

function fakeRuleRequest() {
	return {
		name: 'search-alert',
		shape: 'threshold',
		logql: '{job="siem"}',
		window_sec: 60,
		threshold: 5,
		group_by: [],
		severity: 'warning',
		destinations: ['inapp'],
		cooldown_sec: 3600,
		interval_sec: 60,
		enabled: true
	};
}

describe('POST /api/search/rules', () => {
	it('calls createRule with the session token and returns 201', async () => {
		const createRuleMock = vi.fn().mockResolvedValue({ id: 9, name: 'search-alert' });
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return { createRule: createRuleMock };
		});

		const response = await POST({
			request: new Request('http://x/api/search/rules', {
				method: 'POST',
				body: JSON.stringify(fakeRuleRequest())
			}),
			locals: { sessionToken: 'token-123' }
		} as never);

		expect(createRuleMock).toHaveBeenCalledWith('token-123', fakeRuleRequest());
		expect(response.status).toBe(201);
	});

	it('propagates a SiemApiError status code as a JSON error response', async () => {
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return {
				createRule: vi
					.fn()
					.mockRejectedValue(new siemApiClientModule.SiemApiError(403, 'denied'))
			};
		});

		const response = await POST({
			request: new Request('http://x/api/search/rules', {
				method: 'POST',
				body: JSON.stringify(fakeRuleRequest())
			}),
			locals: { sessionToken: 'token-123' }
		} as never);

		expect(response.status).toBe(403);
	});
});
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd siem-web && npm run test:unit -- --run routes/api/search`
Expected: FAIL — `Cannot find module './+server'`.

- [ ] **Step 3: Implement the route**

Create `siem-web/src/routes/api/search/rules/+server.ts`:

```ts
import { json } from '@sveltejs/kit';
import { env } from '$env/dynamic/private';
import type { RequestHandler } from './$types';
import { SiemApiClient, SiemApiError } from '$lib/server/siemApiClient';
import type { CreateRuleRequest } from '$lib/server/siemApiClient';

export const POST: RequestHandler = async ({ request, locals }) => {
	const client = new SiemApiClient({ baseUrl: env.API_URL as string });
	const token = locals.sessionToken as string;
	const body = (await request.json()) as CreateRuleRequest;

	try {
		const rule = await client.createRule(token, body);
		return json(rule, { status: 201 });
	} catch (err) {
		if (err instanceof SiemApiError) {
			return json({ error: err.message }, { status: err.status });
		}
		throw err;
	}
};
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd siem-web && npm run test:unit -- --run routes/api/search`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add siem-web/src/routes/api/search
git commit -m "Add POST /api/search/rules proxy route"
```

---

### Task 6: siem-web — `/search` load function

**Files:**
- Create: `siem-web/src/routes/search/+page.server.ts`
- Create: `siem-web/src/routes/search/page.server.test.ts`

**Interfaces:**
- Consumes: `Task 4`'s `SiemApiClient.search` (existing method, new response shape),
  `Task 2`/`Task 3`'s `parseFiltersFromURL`/`filtersToSearchParams`/`rangeToSeconds`/
  `extractSrcIp`.
- Produces: the `PageData` shape `Task 11`'s `+page.svelte` renders: `{ filters, logql,
  count, entries, volume, previewIndex, selectedEntry, contextSummary }`.

- [ ] **Step 1: Write the failing tests**

Create `siem-web/src/routes/search/page.server.test.ts`:

```ts
import { describe, it, expect, vi } from 'vitest';
import { load } from './+page.server';
import * as siemApiClientModule from '$lib/server/siemApiClient';
import { SiemApiError } from '$lib/server/siemApiClient';

vi.mock('$env/dynamic/private', () => ({ env: { API_URL: 'http://siem-api:8080' } }));

vi.mock('$lib/server/siemApiClient', async (importOriginal) => {
	const actual = await importOriginal<typeof import('$lib/server/siemApiClient')>();
	return { ...actual, SiemApiClient: vi.fn() };
});

function fakeSearchResult(overrides: Record<string, unknown> = {}) {
	return {
		logql: '{job="siem"}',
		count: 1,
		entries: [{ Timestamp: '2026-08-05T00:00:00Z', Labels: { severity: 'info' }, Line: '{}' }],
		volume: [],
		...overrides
	};
}

describe('Search load', () => {
	it('fetches with limit=10000 and returns the search result', async () => {
		const searchMock = vi.fn().mockResolvedValue(fakeSearchResult());
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return { search: searchMock };
		});

		const result = (await load({
			locals: { sessionToken: 'token-123' },
			url: new URL('https://siem.townsville.cc/search')
		} as never)) as Exclude<Awaited<ReturnType<typeof load>>, void>;

		expect(result.count).toBe(1);
		expect(searchMock).toHaveBeenCalledWith(
			'token-123',
			expect.objectContaining({ limit: '10000' })
		);
	});

	it('has no selected entry or context summary when ?preview= is absent', async () => {
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return { search: vi.fn().mockResolvedValue(fakeSearchResult()) };
		});

		const result = (await load({
			locals: { sessionToken: 'token-123' },
			url: new URL('https://siem.townsville.cc/search')
		} as never)) as Exclude<Awaited<ReturnType<typeof load>>, void>;

		expect(result.selectedEntry).toBeNull();
		expect(result.contextSummary).toBeNull();
	});

	it('resolves the selected entry from ?preview= and fetches a context summary when src_ip is present', async () => {
		const searchMock = vi
			.fn()
			.mockResolvedValueOnce(
				fakeSearchResult({
					entries: [
						{
							Timestamp: '2026-08-05T00:00:00Z',
							Labels: { severity: 'critical' },
							Line: '{"src_ip":"10.0.0.5"}'
						}
					]
				})
			)
			.mockResolvedValueOnce(fakeSearchResult({ count: 4, entries: [] }));
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return { search: searchMock };
		});

		const result = (await load({
			locals: { sessionToken: 'token-123' },
			url: new URL('https://siem.townsville.cc/search?preview=0')
		} as never)) as Exclude<Awaited<ReturnType<typeof load>>, void>;

		expect(result.selectedEntry?.Line).toBe('{"src_ip":"10.0.0.5"}');
		expect(result.contextSummary).toEqual({ count: 4 });
		expect(searchMock).toHaveBeenCalledTimes(2);
	});

	it('redirects to /auth/logout on a 401/403 from the primary search', async () => {
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return { search: vi.fn().mockRejectedValue(new SiemApiError(401, 'invalid session')) };
		});

		await expect(
			load({
				locals: { sessionToken: 'stale-token' },
				url: new URL('https://siem.townsville.cc/search')
			} as never)
		).rejects.toMatchObject({ status: 302, location: '/auth/logout' });
	});

	it('surfaces a 502 when siem-api fails for a reason other than auth', async () => {
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return { search: vi.fn().mockRejectedValue(new SiemApiError(500, 'boom')) };
		});

		await expect(
			load({
				locals: { sessionToken: 'token-123' },
				url: new URL('https://siem.townsville.cc/search')
			} as never)
		).rejects.toMatchObject({ status: 502 });
	});
});
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd siem-web && npm run test:unit -- --run routes/search`
Expected: FAIL — `Cannot find module './+page.server'`.

- [ ] **Step 3: Implement the load function**

Create `siem-web/src/routes/search/+page.server.ts`:

```ts
import { error, redirect } from '@sveltejs/kit';
import { env } from '$env/dynamic/private';
import type { PageServerLoad } from './$types';
import { SiemApiClient, SiemApiError } from '$lib/server/siemApiClient';
import { parseFiltersFromURL, filtersToSearchParams, rangeToSeconds, extractSrcIp } from '$lib/search';

export const load: PageServerLoad = async ({ locals, url }) => {
	const client = new SiemApiClient({ baseUrl: env.API_URL as string });
	const token = locals.sessionToken as string;

	const filters = parseFiltersFromURL(url);
	const end = new Date();
	const start = new Date(end.getTime() - rangeToSeconds(filters.range) * 1000);

	let result;
	try {
		result = await client.search(token, {
			...filtersToSearchParams(filters),
			start: start.toISOString(),
			end: end.toISOString(),
			limit: '10000'
		});
	} catch (err) {
		if (err instanceof SiemApiError) {
			if (err.status === 401 || err.status === 403) {
				redirect(302, '/auth/logout');
			}
			error(502, 'siem-api unavailable');
		}
		throw err;
	}

	const previewParam = url.searchParams.get('preview');
	const previewIndex = previewParam !== null ? Number(previewParam) : null;
	const selectedEntry =
		previewIndex !== null && previewIndex >= 0 && previewIndex < result.entries.length
			? result.entries[previewIndex]
			: null;

	let contextSummary: { count: number } | null = null;
	if (selectedEntry) {
		const srcIp = extractSrcIp(selectedEntry.Line);
		if (srcIp) {
			try {
				const contextResult = await client.search(token, {
					q: srcIp,
					start: new Date(end.getTime() - 24 * 60 * 60 * 1000).toISOString(),
					end: end.toISOString(),
					limit: '1'
				});
				contextSummary = { count: contextResult.count };
			} catch {
				// Context callout is supplementary — a failure here shouldn't
				// take down the rest of the page.
				contextSummary = null;
			}
		}
	}

	return {
		filters,
		logql: result.logql,
		count: result.count,
		entries: result.entries,
		volume: result.volume,
		previewIndex,
		selectedEntry,
		contextSummary
	};
};
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd siem-web && npm run test:unit -- --run routes/search`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add siem-web/src/routes/search/+page.server.ts siem-web/src/routes/search/page.server.test.ts
git commit -m "Add /search load function"
```

---

### Task 7: siem-web — `QueryBar.svelte` and `VolumeRibbon.svelte`

**Files:**
- Create: `siem-web/src/lib/components/QueryBar.svelte`
- Create: `siem-web/src/lib/components/VolumeRibbon.svelte`

**Interfaces:**
- Consumes: `SearchFilters` (Task 2), `computeVolumeTiers` (Task 2), `VolumeBucket`
  (Task 4).
- Produces: both consumed by `Task 11`'s `+page.svelte`. No unit tests — presentational,
  per this project's convention.

- [ ] **Step 1: Implement `QueryBar.svelte`**

Create `siem-web/src/lib/components/QueryBar.svelte`:

```svelte
<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import type { SearchFilters } from '$lib/search';

	let {
		filters,
		logql,
		count,
		onAlertOnThis
	}: {
		filters: SearchFilters;
		logql: string;
		count: number;
		onAlertOnThis: () => void;
	} = $props();

	let source = $state(filters.source);
	let host = $state(filters.host);
	let program = $state(filters.program);
	let severity = $state(filters.severity);
	let facility = $state(filters.facility);
	let q = $state(filters.q);

	const RANGES: SearchFilters['range'][] = ['15m', '24h', '7d'];

	function submit(event: SubmitEvent) {
		event.preventDefault();
		const params = new URLSearchParams();
		if (source) params.set('source', source);
		if (host) params.set('host', host);
		if (program) params.set('program', program);
		if (severity) params.set('severity', severity);
		if (facility) params.set('facility', facility);
		if (q) params.set('q', q);
		params.set('range', filters.range);
		goto(resolve(`/search?${params.toString()}`));
	}

	function setRange(range: SearchFilters['range']) {
		const params = new URLSearchParams(window.location.search);
		params.set('range', range);
		goto(resolve(`/search?${params.toString()}`));
	}
</script>

<form class="query-bar" onsubmit={submit}>
	<input class="field" placeholder="source" bind:value={source} />
	<input class="field" placeholder="host" bind:value={host} />
	<input class="field" placeholder="program" bind:value={program} />
	<input class="field" placeholder="severity" bind:value={severity} />
	<input class="field" placeholder="facility" bind:value={facility} />
	<input class="field wide" placeholder="free text" bind:value={q} />
	<button type="submit" class="go">Search</button>

	<div class="range">
		{#each RANGES as r (r)}
			<button
				type="button"
				class="range-btn"
				class:active={filters.range === r}
				onclick={() => setRange(r)}
			>
				{r}
			</button>
		{/each}
	</div>

	<button type="button" class="action" disabled title="Saved searches aren't built yet">
		Save
	</button>
	<button type="button" class="action" onclick={onAlertOnThis}>Alert on this</button>
</form>

<div class="meta">
	<span class="mono count">{count.toLocaleString()} events</span>
	<span class="logql mono">{logql}</span>
</div>

<style>
	.query-bar {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--space-2);
		background: var(--color-surface);
		box-shadow: inset 0 0 0 1px var(--color-accent-tint-2);
		border-radius: var(--radius-default);
		padding: var(--space-3);
	}
	.field {
		background: var(--color-surface-2);
		border: none;
		border-radius: var(--radius-sm);
		color: var(--color-text);
		padding: var(--space-1) var(--space-2);
		font-size: var(--text-table);
		width: 110px;
	}
	.field.wide {
		flex: 1 1 200px;
		width: auto;
	}
	.go {
		background: var(--color-accent-tint-2);
		color: var(--color-accent-lighter);
		border: none;
		border-radius: var(--radius-sm);
		padding: var(--space-1) var(--space-3);
		font-size: var(--text-label);
		cursor: pointer;
	}
	.range {
		display: flex;
		gap: var(--space-1);
		margin-left: var(--space-2);
	}
	.range-btn {
		background: transparent;
		box-shadow: inset 0 0 0 1px var(--color-line-2);
		color: var(--color-muted);
		border: none;
		border-radius: var(--radius-sm);
		padding: var(--space-1) var(--space-2);
		font-size: var(--text-label);
		cursor: pointer;
	}
	.range-btn.active {
		background: var(--color-accent-tint);
		box-shadow: inset 0 0 0 1px var(--color-accent-deep);
		color: var(--color-accent-lighter);
	}
	.action {
		margin-left: auto;
		background: var(--color-surface-2);
		color: var(--color-text);
		border: none;
		border-radius: var(--radius-sm);
		padding: var(--space-1) var(--space-3);
		font-size: var(--text-label);
		cursor: pointer;
	}
	.action:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}
	.meta {
		display: flex;
		gap: var(--space-3);
		align-items: baseline;
		margin-top: var(--space-2);
		font-size: var(--text-label);
	}
	.count {
		color: var(--color-text);
	}
	.logql {
		color: var(--color-muted-2);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
</style>
```

- [ ] **Step 2: Implement `VolumeRibbon.svelte`**

Create `siem-web/src/lib/components/VolumeRibbon.svelte`:

```svelte
<script lang="ts">
	import { computeVolumeTiers } from '$lib/search';
	import type { VolumeBucket } from '$lib/server/siemApiClient';

	let { volume }: { volume: VolumeBucket[] } = $props();

	let tiers = $derived(computeVolumeTiers(volume));
	let maxCount = $derived(Math.max(1, ...volume.map((b) => b.count)));
</script>

<div class="ribbon">
	{#each volume as bucket, i (bucket.bucket_start)}
		<div
			class="bar tier-{tiers[i]}"
			style:height="{Math.max(2, (bucket.count / maxCount) * 100)}%"
			title="{bucket.count} events"
		></div>
	{/each}
</div>

<style>
	.ribbon {
		display: flex;
		align-items: flex-end;
		gap: 2px;
		height: 56px;
		background: var(--color-surface-2);
		border-radius: var(--radius-sm);
		padding: 0 var(--space-2);
		margin-top: var(--space-3);
	}
	.bar {
		flex: 1;
		min-width: 0;
		border-radius: 1px;
		background: var(--color-accent-tint-2);
	}
	.bar.tier-warning {
		background: var(--color-severity-warning);
	}
	.bar.tier-critical {
		background: var(--color-severity-critical);
	}
</style>
```

- [ ] **Step 3: Typecheck**

Run: `cd siem-web && npm run check && npm run lint`
Expected: no new errors from these two files.

- [ ] **Step 4: Commit**

```bash
git add siem-web/src/lib/components/QueryBar.svelte siem-web/src/lib/components/VolumeRibbon.svelte
git commit -m "Add QueryBar and VolumeRibbon components"
```

---

### Task 8: siem-web — `FacetRail.svelte`

**Files:**
- Create: `siem-web/src/lib/components/FacetRail.svelte`

**Interfaces:**
- Consumes: `LogEntry` (`siemApiClient.ts`), `deriveFacetCounts`/`deriveCountryFacet`
  (Task 2).
- Produces: consumed by `Task 11`'s `+page.svelte`. No unit tests, per convention.

- [ ] **Step 1: Implement the component**

Create `siem-web/src/lib/components/FacetRail.svelte`:

```svelte
<script lang="ts">
	import { deriveFacetCounts, deriveCountryFacet } from '$lib/search';
	import type { LogEntry } from '$lib/server/siemApiClient';

	let {
		entries,
		onFacetClick
	}: {
		entries: LogEntry[];
		onFacetClick: (field: string, value: string) => void;
	} = $props();

	let severities = $derived(deriveFacetCounts(entries, 'severity'));
	let programs = $derived(deriveFacetCounts(entries, 'program'));
	let countries = $derived(deriveCountryFacet(entries));
</script>

<aside class="facets">
	<section>
		<h2>Severity</h2>
		{#each severities as facet (facet.value)}
			<button class="facet-row" onclick={() => onFacetClick('severity', facet.value)}>
				<span class="dot severity-{facet.value}"></span>
				<span class="name">{facet.value}</span>
				<span class="count mono">{facet.count}</span>
			</button>
		{/each}
	</section>
	<section>
		<h2>Program</h2>
		{#each programs as facet (facet.value)}
			<button class="facet-row" onclick={() => onFacetClick('program', facet.value)}>
				<span class="name">{facet.value}</span>
				<span class="count mono">{facet.count}</span>
			</button>
		{/each}
	</section>
	<section>
		<h2>Source country</h2>
		{#each countries as facet (facet.value)}
			<div class="facet-row display-only">
				<span class="name">{facet.value}</span>
				<span class="count mono">{facet.count}</span>
			</div>
		{/each}
	</section>
</aside>

<style>
	.facets {
		width: 184px;
		flex-shrink: 0;
		display: flex;
		flex-direction: column;
		gap: var(--space-4);
	}
	h2 {
		font-size: var(--text-eyebrow);
		text-transform: uppercase;
		color: var(--color-muted-2);
		margin: 0 0 var(--space-2);
	}
	.facet-row {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		width: 100%;
		background: none;
		border: none;
		color: var(--color-text-2);
		padding: var(--space-1) 0;
		font-size: var(--text-table);
		cursor: pointer;
		text-align: left;
	}
	.facet-row.display-only {
		cursor: default;
	}
	.facet-row:hover:not(.display-only) {
		color: var(--color-accent-light);
	}
	.name {
		flex: 1;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.count {
		color: var(--color-muted-2);
	}
	.dot {
		width: 7px;
		height: 7px;
		border-radius: 50%;
		flex-shrink: 0;
	}
	.dot.severity-critical {
		background: var(--color-severity-critical);
	}
	.dot.severity-warning {
		background: var(--color-severity-warning);
	}
	.dot.severity-info,
	.dot.severity-notice {
		background: var(--color-severity-info);
	}
</style>
```

- [ ] **Step 2: Typecheck**

Run: `cd siem-web && npm run check && npm run lint`
Expected: no new errors.

- [ ] **Step 3: Commit**

```bash
git add siem-web/src/lib/components/FacetRail.svelte
git commit -m "Add FacetRail component"
```

---

### Task 9: siem-web — `ResultTable.svelte` (virtualized table)

**Files:**
- Create: `siem-web/src/lib/components/ResultTable.svelte`

**Interfaces:**
- Consumes: `LogEntry` (`siemApiClient.ts`), `computeVisibleRange` (Task 3).
- Produces: consumed by `Task 11`'s `+page.svelte`. No unit tests — this component owns
  DOM measurement (`clientHeight`/`scrollTop`) that can't be meaningfully exercised in
  Vitest's jsdom environment; the windowing *math* itself is already fully covered by
  `Task 3`'s `computeVisibleRange` tests.

This is the highest-risk component in this plan (the first virtualized list in this
codebase). Implement it exactly as given below, then verify manually (Step 3) rather than
assuming correctness from typecheck alone.

- [ ] **Step 1: Implement the component**

Create `siem-web/src/lib/components/ResultTable.svelte`:

```svelte
<script lang="ts">
	import type { LogEntry } from '$lib/server/siemApiClient';
	import { computeVisibleRange } from '$lib/search';

	const ROW_HEIGHT = 28;

	let {
		entries,
		selectedIndex,
		onSelect
	}: {
		entries: LogEntry[];
		selectedIndex: number | null;
		onSelect: (index: number) => void;
	} = $props();

	let containerEl: HTMLDivElement;
	let scrollTop = $state(0);
	let containerHeight = $state(0);

	function handleScroll() {
		if (!containerEl) return;
		scrollTop = containerEl.scrollTop;
	}

	function measure() {
		if (containerEl) containerHeight = containerEl.clientHeight;
	}

	$effect(() => {
		measure();
	});

	let range = $derived(computeVisibleRange(scrollTop, containerHeight, ROW_HEIGHT, entries.length));
	let visibleEntries = $derived(entries.slice(range.startIndex, range.endIndex));
</script>

<svelte:window onresize={measure} />

<div class="table-wrap">
	<div class="header-row">
		<span class="col-time">Time</span>
		<span class="col-severity"></span>
		<span class="col-host">Host</span>
		<span class="col-program">Program</span>
		<span class="col-message">Message</span>
	</div>
	<div class="scroll-container" bind:this={containerEl} onscroll={handleScroll}>
		<div class="spacer" style:height="{entries.length * ROW_HEIGHT}px">
			{#each visibleEntries as entry, i (range.startIndex + i)}
				<button
					class="row"
					class:selected={range.startIndex + i === selectedIndex}
					style:top="{(range.startIndex + i) * ROW_HEIGHT}px"
					onclick={() => onSelect(range.startIndex + i)}
				>
					<span class="col-time mono">{entry.Timestamp}</span>
					<span class="col-severity">
						<span class="dot severity-{entry.Labels.severity ?? 'info'}"></span>
					</span>
					<span class="col-host mono">{entry.Labels.host ?? ''}</span>
					<span class="col-program mono">{entry.Labels.program ?? ''}</span>
					<span class="col-message">{entry.Line}</span>
				</button>
			{/each}
		</div>
	</div>
</div>

<style>
	.table-wrap {
		flex: 1 1 auto;
		min-width: 0;
		background: var(--color-surface-2);
		border-radius: var(--radius-default);
		box-shadow: inset var(--shadow-flat);
		overflow: hidden;
	}
	.header-row {
		display: flex;
		gap: var(--space-3);
		padding: var(--space-2) var(--space-3);
		background: var(--color-surface-3);
		font-size: var(--text-eyebrow);
		text-transform: uppercase;
		color: var(--color-muted-2);
	}
	.scroll-container {
		height: 60vh;
		overflow-y: auto;
		position: relative;
	}
	.spacer {
		position: relative;
	}
	.row {
		position: absolute;
		left: 0;
		right: 0;
		display: flex;
		align-items: center;
		gap: var(--space-3);
		height: 28px;
		padding: 0 var(--space-3);
		background: none;
		border: none;
		width: 100%;
		text-align: left;
		cursor: pointer;
		font-size: 12px;
	}
	.row:nth-child(even) {
		background: rgba(255, 255, 255, 0.015);
	}
	.row.selected {
		background: var(--row-selected-bg);
	}
	.row:hover {
		background: var(--row-hover-bg);
	}
	.mono {
		font-family: var(--font-mono);
	}
	.col-time {
		width: 66px;
		color: var(--color-muted);
		flex-shrink: 0;
	}
	.col-severity {
		width: 14px;
		flex-shrink: 0;
	}
	.col-host {
		width: 88px;
		flex-shrink: 0;
	}
	.col-program {
		width: 78px;
		flex-shrink: 0;
		color: var(--color-accent-light);
	}
	.col-message {
		flex: 1;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		color: var(--color-text-2);
	}
	.dot {
		display: inline-block;
		width: 8px;
		height: 8px;
		border-radius: 50%;
	}
	.dot.severity-critical {
		background: var(--color-severity-critical);
	}
	.dot.severity-warning {
		background: var(--color-severity-warning);
	}
	.dot.severity-info,
	.dot.severity-notice {
		background: var(--color-severity-info);
	}
</style>
```

- [ ] **Step 2: Typecheck**

Run: `cd siem-web && npm run check && npm run lint`
Expected: no new errors.

- [ ] **Step 3: Manual verification**

This component can't be meaningfully unit-tested (DOM scroll measurement), so verify it by
hand: temporarily wire it into any existing route (or use `npm run dev` plus a throwaway
test page) with a synthetic array of ~5,000 fake `LogEntry` objects, and confirm: (a) the
scrollbar's size/position looks proportional to a 5,000-row list, not a ~50-row list; (b)
scrolling through the whole list shows different rows the whole way, with no visible gaps
or duplicate rows; (c) resizing the browser window doesn't break rendering. Remove any
throwaway wiring before committing — this step is verification only, not a deliverable.
Note what you observed in the task report.

- [ ] **Step 4: Commit**

```bash
git add siem-web/src/lib/components/ResultTable.svelte
git commit -m "Add ResultTable: fixed-row-height virtualized result list"
```

---

### Task 10: siem-web — `EventInspector.svelte` and `RuleFromEventForm.svelte`

**Files:**
- Create: `siem-web/src/lib/components/EventInspector.svelte`
- Create: `siem-web/src/lib/components/RuleFromEventForm.svelte`

**Interfaces:**
- Consumes: `LogEntry` (`siemApiClient.ts`), `extractSrcIp` (Task 2).
- Produces: both consumed by `Task 11`'s `+page.svelte`. No unit tests, per convention.
  `RuleFromEventForm.svelte` is the first `<form>`/`<input>` UI in this codebase — no
  existing precedent to mirror, styled fresh from design tokens.

- [ ] **Step 1: Implement `EventInspector.svelte`**

Create `siem-web/src/lib/components/EventInspector.svelte`:

```svelte
<script lang="ts">
	import type { LogEntry } from '$lib/server/siemApiClient';
	import { extractSrcIp } from '$lib/search';

	let {
		entry,
		contextSummary,
		onFilterToSrc,
		onRuleFromThis
	}: {
		entry: LogEntry | null;
		contextSummary: { count: number } | null;
		onFilterToSrc: (srcIp: string) => void;
		onRuleFromThis: (entry: LogEntry) => void;
	} = $props();

	function parsedFields(line: string): [string, string][] {
		try {
			const parsed = JSON.parse(line);
			if (typeof parsed !== 'object' || parsed === null) return [];
			return Object.entries(parsed).map(([k, v]) => [k, JSON.stringify(v)]);
		} catch {
			return [];
		}
	}
</script>

<aside class="inspector">
	{#if !entry}
		<p class="empty">Select an event to see its detail.</p>
	{:else}
		{@const srcIp = extractSrcIp(entry.Line)}
		<div class="header">
			<span class="dot severity-{entry.Labels.severity ?? 'info'}"></span>
			<span class="title">Event detail</span>
		</div>
		<pre class="raw mono">{entry.Line}</pre>
		<dl class="fields mono">
			{#each parsedFields(entry.Line) as [key, value] (key)}
				<dt>{key}</dt>
				<dd>{value}</dd>
			{/each}
		</dl>
		<div class="actions">
			{#if srcIp}
				<button onclick={() => onFilterToSrc(srcIp)}>Filter to SRC</button>
			{/if}
			<button onclick={() => onRuleFromThis(entry)}>Rule from this</button>
		</div>
		{#if contextSummary}
			<div class="context">
				{contextSummary.count} matching event{contextSummary.count === 1 ? '' : 's'} from this
				source IP in the last 24h.
			</div>
		{/if}
	{/if}
</aside>

<style>
	.inspector {
		width: 284px;
		flex-shrink: 0;
	}
	.empty {
		color: var(--color-muted-2);
		font-size: var(--text-body);
	}
	.header {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		margin-bottom: var(--space-3);
	}
	.title {
		font-size: var(--text-section-head);
		color: var(--color-muted);
	}
	.dot {
		width: 8px;
		height: 8px;
		border-radius: 50%;
	}
	.dot.severity-critical {
		background: var(--color-severity-critical);
	}
	.dot.severity-warning {
		background: var(--color-severity-warning);
	}
	.dot.severity-info,
	.dot.severity-notice {
		background: var(--color-severity-info);
	}
	.raw {
		background: var(--color-surface-2);
		border-radius: var(--radius-default);
		padding: var(--space-3);
		font-size: var(--text-log-row);
		white-space: pre-wrap;
		word-break: break-word;
		margin: 0 0 var(--space-3);
	}
	.fields {
		margin: 0 0 var(--space-3);
		display: grid;
		grid-template-columns: 96px 1fr;
		gap: var(--space-1) var(--space-3);
		font-size: var(--text-label);
	}
	dt {
		color: var(--color-muted);
	}
	dd {
		margin: 0;
		color: var(--color-accent-lighter);
		overflow-wrap: anywhere;
	}
	.actions {
		display: flex;
		gap: var(--space-2);
		margin-bottom: var(--space-3);
	}
	.actions button {
		background: var(--color-surface-2);
		color: var(--color-text);
		border: none;
		border-radius: var(--radius-sm);
		padding: var(--space-1) var(--space-3);
		font-size: var(--text-label);
		cursor: pointer;
	}
	.context {
		background: var(--color-accent-tint);
		border-radius: var(--radius-default);
		padding: var(--space-3);
		font-size: var(--text-table);
		color: var(--color-text-2);
	}
	.mono {
		font-family: var(--font-mono);
	}
</style>
```

- [ ] **Step 2: Implement `RuleFromEventForm.svelte`**

Create `siem-web/src/lib/components/RuleFromEventForm.svelte`:

```svelte
<script lang="ts">
	let {
		defaultName,
		defaultLogql,
		onClose
	}: {
		defaultName: string;
		defaultLogql: string;
		onClose: () => void;
	} = $props();

	let name = $state(defaultName);
	let logql = $state(defaultLogql);
	let windowSec = $state(60);
	let threshold = $state(5);
	let severity = $state('warning');
	let submitting = $state(false);
	let error = $state<string | null>(null);

	async function submit(event: SubmitEvent) {
		event.preventDefault();
		submitting = true;
		error = null;
		try {
			const response = await fetch('/api/search/rules', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({
					name,
					shape: 'threshold',
					logql,
					window_sec: windowSec,
					threshold,
					group_by: [],
					severity,
					destinations: ['inapp'],
					cooldown_sec: 3600,
					interval_sec: 60,
					enabled: true
				})
			});
			if (!response.ok) {
				error = 'Failed to create rule.';
				return;
			}
			onClose();
		} finally {
			submitting = false;
		}
	}
</script>

<div class="overlay">
	<form class="rule-form" onsubmit={submit}>
		<h2>Create rule</h2>
		<label>
			Name
			<input bind:value={name} required />
		</label>
		<label>
			LogQL
			<textarea bind:value={logql} required></textarea>
		</label>
		<label>
			Window (seconds)
			<input type="number" bind:value={windowSec} min="1" />
		</label>
		<label>
			Threshold
			<input type="number" bind:value={threshold} min="1" />
		</label>
		<label>
			Severity
			<select bind:value={severity}>
				<option value="critical">critical</option>
				<option value="warning">warning</option>
				<option value="info">info</option>
			</select>
		</label>
		{#if error}
			<p class="error">{error}</p>
		{/if}
		<div class="actions">
			<button type="button" onclick={onClose}>Cancel</button>
			<button type="submit" disabled={submitting}>
				{submitting ? 'Creating…' : 'Create rule'}
			</button>
		</div>
	</form>
</div>

<style>
	.overlay {
		position: fixed;
		inset: 0;
		background: rgba(0, 0, 0, 0.5);
		display: flex;
		align-items: center;
		justify-content: center;
		z-index: 10;
	}
	.rule-form {
		background: var(--color-surface);
		border-radius: var(--radius-lg);
		box-shadow: var(--shadow-raised);
		padding: var(--space-6);
		width: 360px;
		display: flex;
		flex-direction: column;
		gap: var(--space-3);
	}
	h2 {
		margin: 0;
		font-size: var(--text-section-head);
	}
	label {
		display: flex;
		flex-direction: column;
		gap: var(--space-1);
		font-size: var(--text-label);
		color: var(--color-muted);
	}
	input,
	select,
	textarea {
		background: var(--color-surface-2);
		border: none;
		border-radius: var(--radius-sm);
		color: var(--color-text);
		padding: var(--space-2);
		font-size: var(--text-table);
		font-family: inherit;
	}
	textarea {
		font-family: var(--font-mono);
		font-size: var(--text-label);
		min-height: 60px;
		resize: vertical;
	}
	.error {
		color: var(--color-severity-critical);
		font-size: var(--text-label);
		margin: 0;
	}
	.actions {
		display: flex;
		justify-content: flex-end;
		gap: var(--space-2);
		margin-top: var(--space-2);
	}
	.actions button {
		border: none;
		border-radius: var(--radius-sm);
		padding: var(--space-1) var(--space-3);
		font-size: var(--text-label);
		cursor: pointer;
	}
	.actions button[type='submit'] {
		background: var(--color-accent-tint-2);
		color: var(--color-accent-lighter);
	}
	.actions button[type='button'] {
		background: var(--color-surface-2);
		color: var(--color-text);
	}
	.actions button:disabled {
		opacity: 0.6;
		cursor: default;
	}
</style>
```

- [ ] **Step 3: Typecheck**

Run: `cd siem-web && npm run check && npm run lint`
Expected: no new errors.

- [ ] **Step 4: Commit**

```bash
git add siem-web/src/lib/components/EventInspector.svelte siem-web/src/lib/components/RuleFromEventForm.svelte
git commit -m "Add EventInspector and RuleFromEventForm components"
```

---

### Task 11: siem-web — `/search` page assembly

**Files:**
- Create: `siem-web/src/routes/search/+page.svelte`
- Modify: `siem-web/src/lib/components/Nav.svelte`

**Interfaces:**
- Consumes: `Task 6`'s `PageData`, `Task 7`/`8`/`9`/`10`'s six components.

- [ ] **Step 1: Implement the page**

Create `siem-web/src/routes/search/+page.svelte`:

```svelte
<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import QueryBar from '$lib/components/QueryBar.svelte';
	import VolumeRibbon from '$lib/components/VolumeRibbon.svelte';
	import FacetRail from '$lib/components/FacetRail.svelte';
	import ResultTable from '$lib/components/ResultTable.svelte';
	import EventInspector from '$lib/components/EventInspector.svelte';
	import RuleFromEventForm from '$lib/components/RuleFromEventForm.svelte';
	import type { LogEntry } from '$lib/server/siemApiClient';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	let ruleFormSeed = $state<{ name: string; logql: string } | null>(null);

	function selectRow(index: number) {
		const params = new URLSearchParams(window.location.search);
		params.set('preview', String(index));
		goto(resolve(`/search?${params.toString()}`), { noScroll: true, keepFocus: true });
	}

	function filterToSrc(srcIp: string) {
		const params = new URLSearchParams(window.location.search);
		params.set('q', srcIp);
		goto(resolve(`/search?${params.toString()}`));
	}

	function facetClick(field: string, value: string) {
		const params = new URLSearchParams(window.location.search);
		params.set(field, value);
		goto(resolve(`/search?${params.toString()}`));
	}

	function alertOnThis() {
		ruleFormSeed = { name: 'search-alert', logql: data.logql };
	}

	function ruleFromEvent(entry: LogEntry) {
		ruleFormSeed = { name: 'event-rule', logql: data.logql };
	}
</script>

<div class="search-screen">
	<QueryBar
		filters={data.filters}
		logql={data.logql}
		count={data.count}
		onAlertOnThis={alertOnThis}
	/>
	<VolumeRibbon volume={data.volume} />
	<div class="body">
		<FacetRail entries={data.entries} onFacetClick={facetClick} />
		<ResultTable entries={data.entries} selectedIndex={data.previewIndex} onSelect={selectRow} />
		<EventInspector
			entry={data.selectedEntry}
			contextSummary={data.contextSummary}
			onFilterToSrc={filterToSrc}
			onRuleFromThis={ruleFromEvent}
		/>
	</div>
</div>

{#if ruleFormSeed}
	<RuleFromEventForm
		defaultName={ruleFormSeed.name}
		defaultLogql={ruleFormSeed.logql}
		onClose={() => (ruleFormSeed = null)}
	/>
{/if}

<style>
	.search-screen {
		padding: var(--space-5) var(--space-6);
	}
	.body {
		display: flex;
		gap: var(--space-5);
		margin-top: var(--space-4);
		align-items: flex-start;
	}
</style>
```

- [ ] **Step 2: Drop the now-unnecessary `Pathname` assertion in `Nav.svelte`**

In `siem-web/src/lib/components/Nav.svelte`, the `/search` route now exists, so drop its
forward-declaration cast (per the file's own comment, "drop each assertion as its route
lands"). Change:

```ts
		{ label: 'Search', href: '/search' as Pathname },
```

to:

```ts
		{ label: 'Search', href: '/search' },
```

While in this file, also update the stale comment above `navItems` — it currently claims
"the other five screens are separate future sub-projects" and lists which paths still
need casts; after this task, only `/settings` remains cast. Update the comment to reflect
that (one sentence is enough — don't over-elaborate).

- [ ] **Step 3: Typecheck, lint, and run the full test suite**

Run: `cd siem-web && npm run check && npm run lint && npm run test:unit -- --run`
Expected: no new type errors, no lint errors, all tests (existing + this plan's) pass.

- [ ] **Step 4: Manual verification**

Run `cd siem-web && npm run dev`. Since this environment has no real siem-api/siem-ingest,
confirm at minimum: `/search` fails gracefully (redirect to login, or a clean 502 error
page) rather than crashing with an unhandled exception or a Svelte compile error. Note
what you actually observed.

- [ ] **Step 5: Commit**

```bash
git add siem-web/src/routes/search/+page.svelte siem-web/src/lib/components/Nav.svelte
git commit -m "Assemble the Search screen page and wire the nav link"
```
