import { describe, it, expect, afterEach } from 'vitest';
import { render, screen, fireEvent, cleanup } from '@testing-library/svelte';
import ParserPreview from './ParserPreview.svelte';
import type { LogEntry } from '$lib/server/siemApiClient';

// See RuleDetail.svelte.test.ts for why this is needed - @testing-library/svelte's
// auto-cleanup only self-registers when `afterEach` is a vitest global.
afterEach(() => cleanup());

function entry(line: string, ts: string): LogEntry {
	return { Timestamp: ts, Labels: {}, Line: line };
}

// The "Raw line" card and the history-row preview can show overlapping text
// for a short test fixture (the row truncates at 80 chars, well past what's
// convenient to type in a test) - reading the card's own <pre> directly,
// rather than a global getByText, is what actually asserts which sample is
// selected regardless of line length.
function rawLineText(container: HTMLElement): string | null {
	return container.querySelector('.card pre')?.textContent ?? null;
}

describe('ParserPreview', () => {
	it('shows the empty state when there are no samples', () => {
		render(ParserPreview, { props: { sourceName: 'udm-ultra', samples: [] } });

		expect(screen.getByText('No recent events from this source yet.')).toBeTruthy();
	});

	it('shows the raw/parsed cards for a single sample with no history list', () => {
		const samples = [entry('{"msg":"hello"}', '2026-08-30T00:00:00.000Z')];
		render(ParserPreview, { props: { sourceName: 'udm-ultra', samples } });

		expect(screen.getByText('{"msg":"hello"}')).toBeTruthy();
		expect(screen.queryByRole('listbox')).toBeNull();
	});

	it('shows a clickable history list for multiple samples and switches the cards on click', async () => {
		const samples = [
			entry('{"msg":"newest"}', '2026-08-30T00:02:00.000Z'),
			entry('{"msg":"older"}', '2026-08-30T00:01:00.000Z')
		];
		const { container } = render(ParserPreview, { props: { sourceName: 'udm-ultra', samples } });

		// Newest sample selected by default.
		expect(rawLineText(container)).toBe('{"msg":"newest"}');

		const rows = screen.getAllByRole('option');
		expect(rows).toHaveLength(2);
		await fireEvent.click(rows[1]);

		expect(rawLineText(container)).toBe('{"msg":"older"}');
	});

	it('resets the selection to the newest sample when the source changes', async () => {
		const sourceA = [
			entry('{"msg":"a-newest"}', '2026-08-30T00:02:00.000Z'),
			entry('{"msg":"a-older"}', '2026-08-30T00:01:00.000Z')
		];
		const { container, rerender } = render(ParserPreview, {
			props: { sourceName: 'source-a', samples: sourceA }
		});
		await fireEvent.click(screen.getAllByRole('option')[1]);
		expect(rawLineText(container)).toBe('{"msg":"a-older"}');

		const sourceB = [entry('{"msg":"b-newest"}', '2026-08-30T00:03:00.000Z')];
		await rerender({ sourceName: 'source-b', samples: sourceB });

		expect(rawLineText(container)).toBe('{"msg":"b-newest"}');
	});
});
