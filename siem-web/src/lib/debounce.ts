// Trailing-edge debounce: `fn` runs once, `waitMs` after the last call in a
// burst. `cancel` drops a pending run (for component/effect teardown).
export function debounce(fn: () => void, waitMs: number): { call: () => void; cancel: () => void } {
	let timer: ReturnType<typeof setTimeout> | undefined;
	return {
		call() {
			clearTimeout(timer);
			timer = setTimeout(() => {
				timer = undefined;
				fn();
			}, waitMs);
		},
		cancel() {
			clearTimeout(timer);
			timer = undefined;
		}
	};
}
