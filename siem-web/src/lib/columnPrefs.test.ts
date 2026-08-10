import { describe, it, expect, beforeEach, vi } from 'vitest';
import { loadHiddenColumns, saveHiddenColumns } from './columnPrefs';

// The "server" vitest project runs under plain Node (see vite.config.ts),
// which has no localStorage global - stub a minimal in-memory version
// rather than depend on a specific Node runtime's own implementation.
function createMemoryStorage(): Storage {
	const store = new Map<string, string>();
	return {
		getItem: (key: string) => store.get(key) ?? null,
		setItem: (key: string, value: string) => void store.set(key, value),
		removeItem: (key: string) => void store.delete(key),
		clear: () => store.clear(),
		key: (index: number) => [...store.keys()][index] ?? null,
		get length() {
			return store.size;
		}
	};
}

vi.stubGlobal('localStorage', createMemoryStorage());

beforeEach(() => {
	localStorage.clear();
});

describe('loadHiddenColumns', () => {
	it('returns an empty set when nothing is stored and no default is given', () => {
		expect(loadHiddenColumns('test-key')).toEqual(new Set());
	});

	it('returns the given default when nothing is stored yet', () => {
		expect(loadHiddenColumns('test-key', new Set(['facility']))).toEqual(new Set(['facility']));
	});

	it('returns the previously saved set even when it differs from the default', () => {
		localStorage.setItem('test-key', JSON.stringify(['facility', 'host']));
		expect(loadHiddenColumns('test-key', new Set(['program']))).toEqual(
			new Set(['facility', 'host'])
		);
	});

	it('an explicitly saved empty array wins over the default (user chose to show everything)', () => {
		localStorage.setItem('test-key', JSON.stringify([]));
		expect(loadHiddenColumns('test-key', new Set(['facility']))).toEqual(new Set());
	});

	it('returns the default for malformed stored JSON rather than throwing', () => {
		localStorage.setItem('test-key', 'not json');
		expect(loadHiddenColumns('test-key', new Set(['facility']))).toEqual(new Set(['facility']));
	});

	it('returns an empty set for validly-parsed JSON that is not an array', () => {
		localStorage.setItem('test-key', JSON.stringify({ not: 'an array' }));
		expect(loadHiddenColumns('test-key')).toEqual(new Set());
	});

	it('drops non-string entries from a stored array instead of throwing', () => {
		localStorage.setItem('test-key', JSON.stringify(['host', 42, null]));
		expect(loadHiddenColumns('test-key')).toEqual(new Set(['host']));
	});
});

describe('saveHiddenColumns', () => {
	it('persists the set so a later load returns it', () => {
		saveHiddenColumns('test-key', new Set(['facility']));
		expect(loadHiddenColumns('test-key')).toEqual(new Set(['facility']));
	});

	it('overwrites a previously saved set rather than merging', () => {
		saveHiddenColumns('test-key', new Set(['facility']));
		saveHiddenColumns('test-key', new Set(['host']));
		expect(loadHiddenColumns('test-key')).toEqual(new Set(['host']));
	});
});
