import type { LogEntry } from './server/siemApiClient';
import { parseLogLine, extractField } from './logline';

export interface ColumnDef {
	key: string;
	label: string;
}

export const SEARCH_COLUMNS: ColumnDef[] = [
	{ key: 'time', label: 'Time (UTC)' },
	{ key: 'localTime', label: 'Local time' },
	{ key: 'severity', label: 'Severity' },
	{ key: 'host', label: 'Host' },
	{ key: 'program', label: 'Program' },
	{ key: 'message', label: 'Message' }
];

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
	// Display override for `value` - unused (undefined) for every facet
	// except Source, where `value` has to stay the raw source `name`
	// (what onFacetClick pivots the search on, and what Loki's `source`
	// label actually contains) while the visible text should prefer an
	// operator-set display_name. See mergeSourceFacet.
	label?: string;
	count: number;
}

export interface KnownSource {
	name: string;
	displayName: string;
}

// Derives facet counts purely from a page of already-fetched entries.
// Prefer the real Loki-side aggregate siem-api now returns (searchResponse.
// Facets) wherever one is available - this is capped at however many
// entries were fetched (1000 by default), so a rare value can be badly
// undercounted whenever a noisier one shares the same window (see
// FacetRail's facetOrFallback for why this exists only as a fallback for
// when the backend aggregate isn't available at all).
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

// Takes the Source facet's counts as computed server-side (a real Loki
// aggregate over the full time range - see siem-api's queryFacets/
// handleEventsSearch, and the Facets field doc on searchResponse) rather
// than deriving them from `entries` itself.
//
// Found in production, back when this derived from entries: a claimed,
// actively-logging source (e.g. a low-volume one like Homebridge) could be
// completely absent from the facet with no filters applied, simply because
// a handful of high-volume sources (this host's own containers logging
// about themselves) exhaust the whole 1000-entry page within a couple of
// minutes of real time. That read as "this source isn't being ingested"
// when it was actually just crowded out of the current page - the same
// class of bug as the severity undercount, just for the Source facet.
// Real backend aggregate counts fix that the same way. Still merges in
// every known claimed source not already present in the counts, at count
// 0 - still clickable (via the same onFacetClick as any other row) to
// pivot the search to it directly, just visually distinguished as "known
// but zero matches in this window" by the caller.
//
// Also found in production: the backend's source facet value is always the
// raw syslog-derived name (e.g. a bare IP for a sender with no real
// hostname) - it has no way to know about a source's operator-set
// display_name (see SourcesTable's rename feature), so EVERY row here, not
// just the merged "known but absent" ones, needs its label filled in from
// knownSources.
export function mergeSourceFacet(
	apiFacets: FacetCount[],
	knownSources: KnownSource[]
): FacetCount[] {
	const labelByName = new Map(knownSources.map((s) => [s.name, s.displayName || s.name]));

	const counts = apiFacets.map((c) => ({
		...c,
		label: labelByName.get(c.value) ?? c.value
	}));
	const present = new Set(counts.map((c) => c.value));
	const missing = knownSources
		.filter((s) => !present.has(s.name))
		.sort((a, b) => (a.displayName || a.name).localeCompare(b.displayName || b.name))
		.map((s) => ({ value: s.name, label: s.displayName || s.name, count: 0 }));
	return [...counts, ...missing];
}

export function deriveCountryFacet(entries: LogEntry[]): FacetCount[] {
	const counts = new Map<string, number>();
	for (const entry of entries) {
		const parsed = parseLogLine(entry.Line);
		if (!parsed) continue;
		const geoip = parsed.geoip;
		if (typeof geoip !== 'object' || geoip === null) continue;
		// Vector's geoip enrichment table names this field country_code,
		// not "cc" - see the matching comment in wall.ts's
		// deriveCountryBreakdown, the same bug fixed in the same way.
		const country = (geoip as Record<string, unknown>).country_code;
		if (typeof country !== 'string' || country === '') continue;
		counts.set(country, (counts.get(country) ?? 0) + 1);
	}
	return [...counts.entries()]
		.map(([value, count]) => ({ value, count }))
		.sort((a, b) => b.count - a.count);
}

// Go's time.Time JSON marshaling (RFC3339Nano) trims trailing zero
// fractional-second digits, so the same field can arrive as anywhere from
// `...:00Z` (no fraction) to `...:00.123456789Z` (9 digits) depending on
// what a given event's nanosecond component happens to be. Rendering that
// directly in a fixed-width table column truncates unpredictably — this
// normalizes every timestamp to a fixed millisecond-precision format so the
// displayed length is always the same.
export function formatTimestamp(iso: string): string {
	const match = iso.match(/^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2})(\.\d+)?Z$/);
	if (!match) return iso;
	const [, base, frac] = match;
	const ms = (frac ?? '').slice(1, 4).padEnd(3, '0');
	return `${base}.${ms}Z`;
}

export function extractSrcIp(line: string): string | null {
	return extractField(line, 'src_ip');
}

export interface VolumeBucketLike {
	bucket_start: string;
	count: number;
}

export function computeVolumeTiers(
	buckets: VolumeBucketLike[]
): Array<'normal' | 'warning' | 'critical'> {
	if (buckets.length === 0) return [];
	const sorted = buckets.map((b) => b.count).sort((a, b) => a - b);
	const percentile = (p: number) =>
		sorted[Math.min(sorted.length - 1, Math.floor(sorted.length * p))];
	const warningThreshold = percentile(0.7);
	const criticalThreshold = percentile(0.88);
	return buckets.map((b) => {
		if (b.count > criticalThreshold) return 'critical';
		if (b.count > warningThreshold) return 'warning';
		return 'normal';
	});
}

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
	const maxFirstVisible = Math.max(0, totalRows - 1);
	const firstVisible = Math.min(Math.max(0, Math.floor(scrollTop / rowHeight)), maxFirstVisible);
	const visibleCount = Math.ceil(containerHeight / rowHeight);
	const startIndex = Math.max(0, firstVisible - VIRTUALIZATION_BUFFER_ROWS);
	const endIndex = Math.min(totalRows, firstVisible + visibleCount + VIRTUALIZATION_BUFFER_ROWS);
	return { startIndex, endIndex, offsetTop: startIndex * rowHeight };
}

/** Whether a scroll container is within `threshold`px of its bottom. */
export function isScrolledToBottom(
	scrollTop: number,
	containerHeight: number,
	totalContentHeight: number,
	threshold = 4
): boolean {
	return totalContentHeight - scrollTop - containerHeight < threshold;
}
