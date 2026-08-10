import type { LogEntry } from './server/siemApiClient';
import type { ColumnDef } from './search';

export const SYSLOG_SEVERITIES = [
	'emerg',
	'alert',
	'crit',
	'err',
	'warning',
	'notice',
	'info',
	'debug'
] as const;

export const TAIL_COLUMNS: ColumnDef[] = [
	{ key: 'time', label: 'Time (UTC)' },
	{ key: 'localTime', label: 'Local time' },
	{ key: 'severity', label: 'Severity' },
	{ key: 'host', label: 'Host' },
	{ key: 'program', label: 'Program' },
	// Never populated from a Loki stream label (facility isn't indexed as
	// one) - only from the raw line's own parsed JSON, unlike every other
	// column here. Hidden by default since it's rarely useful, but still
	// real data once shown, not the permanently-empty column it used to be.
	{ key: 'facility', label: 'Facility' },
	{ key: 'message', label: 'Message' }
];

export const TAIL_DEFAULT_HIDDEN_COLUMNS = new Set(['facility']);

export function filterBySeverity(entries: LogEntry[], activeSeverities: Set<string>): LogEntry[] {
	return entries.filter((e) => activeSeverities.has(e.Labels.severity ?? 'info'));
}

export function serializeNdjson(entries: LogEntry[]): string {
	return entries.map((e) => JSON.stringify(e)).join('\n');
}

const SEVERITY_COLOR_TOKENS: Record<string, string> = {
	emerg: 'var(--color-severity-critical)',
	alert: 'var(--color-severity-critical)',
	crit: 'var(--color-severity-critical)',
	err: 'var(--color-severity-error)',
	warning: 'var(--color-severity-warning)',
	notice: 'var(--color-severity-notice)',
	info: 'var(--color-severity-info)',
	debug: 'var(--color-muted-2)'
};

export function severityColor(severity: string): string {
	return SEVERITY_COLOR_TOKENS[severity] ?? SEVERITY_COLOR_TOKENS.info;
}
