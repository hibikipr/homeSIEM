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
		return typeof parsed === 'object' && parsed !== null
			? (parsed as Record<string, unknown>)
			: null;
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
