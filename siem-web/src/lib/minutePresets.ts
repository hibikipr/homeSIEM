// Formats a whole number of minutes as a human label, collapsing up to the
// largest clean unit (minutes -> hours -> days) rather than ever showing
// something like "1440 minutes" - used by MinutesPicker's preset dropdown.
export function formatMinuteLabel(minutes: number): string {
	if (minutes < 60) {
		return `${minutes} minute${minutes === 1 ? '' : 's'}`;
	}
	const hours = minutes / 60;
	if (Number.isInteger(hours)) {
		if (hours < 24) {
			return `${hours} hour${hours === 1 ? '' : 's'}`;
		}
		const days = hours / 24;
		if (Number.isInteger(days)) {
			return `${days} day${days === 1 ? '' : 's'}`;
		}
		return `${hours} hours`;
	}
	return `${minutes} minutes`;
}

// For read-only display of a *_sec field (e.g. RuleDetail's meta line).
// Falls back to raw seconds for a value that isn't a whole number of
// minutes (e.g. 90s) rather than showing a misleading rounded minute count -
// values set before this UI existed, or via the API directly, aren't
// guaranteed to be clean minute multiples.
export function formatSecondsAsMinutes(seconds: number): string {
	if (seconds % 60 !== 0) {
		return `${seconds}s`;
	}
	return formatMinuteLabel(seconds / 60);
}
