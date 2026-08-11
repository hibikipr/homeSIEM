// Shared helpers for the raw Loki line format both Search and Live Tail deal
// with: siem-ingest's enrich_geo transform serializes the entire enriched
// event as a JSON object and that's what lands in Loki as the log line
// (LogEntry.Line) - the human-readable text a user actually wrote is nested
// inside it as .message, not the line itself.

export function parseLogLine(line: string): Record<string, unknown> | null {
	try {
		const parsed = JSON.parse(line);
		return typeof parsed === 'object' && parsed !== null && !Array.isArray(parsed)
			? (parsed as Record<string, unknown>)
			: null;
	} catch {
		return null;
	}
}

export function extractField(line: string, key: string): string | null {
	const parsed = parseLogLine(line);
	if (!parsed) return null;
	const value = parsed[key];
	return typeof value === 'string' ? value : null;
}

// Some sources' loggers color their own output (Homebridge's own Logger,
// most visibly - siem-ingest's severity classification for it depends on
// reading these exact codes, see vector.toml's enrich_geo transform - but
// nothing ever strips them back out for display). Useful in a real
// terminal, meaningless - and ugly, literal "\x1b[31m...\x1b[39m" bracketing
// the text - once rendered as plain text here. Strips both the raw ESC-byte
// form (imfile-forwarded sources, whose control bytes pass through
// unmodified) and rsyslog's traditional escaped "#033[...m" text form
// (every journald-sourced ANSI-colored source in this app, whose control
// bytes get escaped in transit), so neither ever reaches the UI.
// eslint-disable-next-line no-control-regex -- deliberately matching the real ESC (0x1B) control byte, not a typo.
const ANSI_ESCAPE_BYTE = /\x1b\[[0-9;]*m/g;
const ANSI_ESCAPE_TEXT = /#033\[[0-9;]*m/g;

function stripAnsi(text: string): string {
	return text.replace(ANSI_ESCAPE_BYTE, '').replace(ANSI_ESCAPE_TEXT, '');
}

// Falls back to the raw line for anything that isn't the JSON shape
// enrich_geo produces (malformed line, or a .message-less event) rather than
// showing an empty cell.
export function extractMessage(line: string): string {
	const message = extractField(line, 'message') ?? line;
	return stripAnsi(message);
}

type DatePart = 'year' | 'month' | 'day' | 'hour' | 'minute' | 'second';

// Formats an ISO-8601 UTC timestamp in an arbitrary IANA time zone as
// "YYYY-MM-DD HH:mm:ss" - used for the deployment-configured "local time"
// column alongside the raw UTC one. Falls back to the input string for an
// unparseable timestamp or an invalid time zone identifier (e.g. a typo in
// the TZ env var) rather than throwing and breaking the whole table.
export function formatTimestampInZone(iso: string, timeZone: string): string {
	const date = new Date(iso);
	if (Number.isNaN(date.getTime())) return iso;
	try {
		const parts = new Intl.DateTimeFormat('en-US', {
			timeZone,
			year: 'numeric',
			month: '2-digit',
			day: '2-digit',
			hour: '2-digit',
			minute: '2-digit',
			second: '2-digit',
			// hourCycle (not hour12) to force 00-23 - hour12: false alone still
			// renders midnight as "24" under ICU's default hour cycle for this
			// locale, which "2026-08-10 24:30:00" is not a valid clock time.
			hourCycle: 'h23'
		}).formatToParts(date);
		const get = (type: DatePart) => parts.find((p) => p.type === type)?.value ?? '';
		return `${get('year')}-${get('month')}-${get('day')} ${get('hour')}:${get('minute')}:${get('second')}`;
	} catch {
		return iso;
	}
}
