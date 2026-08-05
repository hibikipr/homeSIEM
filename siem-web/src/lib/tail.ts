import type { LogEntry } from './server/siemApiClient';

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
