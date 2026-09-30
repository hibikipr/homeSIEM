import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { debounce } from './debounce';

describe('debounce', () => {
	beforeEach(() => vi.useFakeTimers());
	afterEach(() => vi.useRealTimers());

	it('runs once, waitMs after the last call in a burst', () => {
		const fn = vi.fn();
		const d = debounce(fn, 500);

		for (let i = 0; i < 20; i++) d.call();
		vi.advanceTimersByTime(499);
		expect(fn).not.toHaveBeenCalled();
		vi.advanceTimersByTime(1);
		expect(fn).toHaveBeenCalledTimes(1);
	});

	it('runs again for a later, separate burst', () => {
		const fn = vi.fn();
		const d = debounce(fn, 500);

		d.call();
		vi.advanceTimersByTime(500);
		d.call();
		vi.advanceTimersByTime(500);
		expect(fn).toHaveBeenCalledTimes(2);
	});

	it('cancel drops a pending run', () => {
		const fn = vi.fn();
		const d = debounce(fn, 500);

		d.call();
		d.cancel();
		vi.advanceTimersByTime(1000);
		expect(fn).not.toHaveBeenCalled();
	});
});
