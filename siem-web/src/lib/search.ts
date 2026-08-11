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

// Found in production: the Source facet is derived purely from the current
// (filtered, limit-capped) result set - a claimed, actively-logging source
// (e.g. a low-volume one like Homebridge) can be completely absent from it
// with no filters applied, simply because a handful of high-volume sources
// (this host's own containers logging about themselves) exhaust the whole
// 1000-entry cap within a couple of minutes of real time. That reads as
// "this source isn't being ingested" when it's actually just crowded out
// of the current page. Merges in every known claimed source not already
// present in the derived counts, at count 0 - still clickable (via the
// same onFacetClick as any other row) to pivot the search to it directly,
// just visually distinguished as "known but not in this result set" by the
// caller.
export function mergeSourceFacet(entries: LogEntry[], knownSourceNames: string[]): FacetCount[] {
	const counts = deriveFacetCounts(entries, 'source');
	const present = new Set(counts.map((c) => c.value));
	const missing = knownSourceNames
		.filter((name) => !present.has(name))
		.sort((a, b) => a.localeCompare(b))
		.map((name) => ({ value: name, count: 0 }));
	return [...counts, ...missing];
}

export function deriveCountryFacet(entries: LogEntry[]): FacetCount[] {
	const counts = new Map<string, number>();
	for (const entry of entries) {
		const parsed = parseLogLine(entry.Line);
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
