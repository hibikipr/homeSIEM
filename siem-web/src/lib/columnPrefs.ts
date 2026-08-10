// Persists which table columns a user has hidden on Search/Live tail, per
// browser (localStorage) - there's no server-side per-user settings store
// for UI preferences like this, and it wouldn't be worth one for something
// this low-stakes.

// defaultHidden applies only the first time a browser has never saved a
// preference for this key - once anything is saved (including an explicit
// "show everything" choice, saved as an empty array), the stored value is
// authoritative even if it happens to also be empty.
export function loadHiddenColumns(
	storageKey: string,
	defaultHidden: ReadonlySet<string> = new Set()
): Set<string> {
	if (typeof localStorage === 'undefined') return new Set(defaultHidden);
	try {
		const raw = localStorage.getItem(storageKey);
		if (raw === null) return new Set(defaultHidden);
		const parsed = JSON.parse(raw);
		return Array.isArray(parsed) ? new Set(parsed.filter((v) => typeof v === 'string')) : new Set();
	} catch {
		return new Set(defaultHidden);
	}
}

export function saveHiddenColumns(storageKey: string, hidden: ReadonlySet<string>): void {
	if (typeof localStorage === 'undefined') return;
	try {
		localStorage.setItem(storageKey, JSON.stringify([...hidden]));
	} catch {
		// Storage full/disabled (private browsing, quota) - the toggle still
		// works for the current page load, it just won't persist. Not worth
		// surfacing an error for a UI preference.
	}
}
