import { describe, it, expect, afterEach } from 'vitest';
import { render, screen, cleanup } from '@testing-library/svelte';
import AlertRow from './AlertRow.svelte';
import type { AlertResponse } from '$lib/server/siemApiClient';

// @testing-library/svelte's own auto-cleanup only registers when `afterEach`
// is a global (this project doesn't set vitest's `globals: true`), so
// without this, each render() in this file stacks a fresh copy into the
// document instead of replacing the last one - later tests' getByRole
// queries then fail with "multiple elements found."
afterEach(() => cleanup());

function fakeAlert(overrides: Partial<AlertResponse> = {}): AlertResponse {
	return {
		id: 1,
		rule_id: 2,
		group_key: 'host-1',
		severity: 'warning',
		title: 'Something happened',
		body: 'details here',
		event_count: 3,
		state: 'open',
		first_seen_at: new Date().toISOString(),
		last_seen_at: new Date().toISOString(),
		...overrides
	};
}

describe('AlertRow', () => {
	it("links to the tab matching the alert's own state, not a hardcoded acked/open pair", () => {
		render(AlertRow, {
			props: { alert: fakeAlert({ id: 5, state: 'muted' }), ruleName: 'r', selected: false }
		});

		const link = screen.getByRole('link') as HTMLAnchorElement;
		expect(link.getAttribute('href')).toBe('/alerts?state=muted&id=5');
	});

	it('shows a muted countdown instead of the age when muted with a future muted_until', () => {
		const until = new Date(Date.now() + 45 * 60000).toISOString();
		render(AlertRow, {
			props: {
				alert: fakeAlert({ state: 'muted', muted_until: until }),
				ruleName: 'r',
				selected: false
			}
		});

		expect(screen.getByText('muted 45m')).toBeTruthy();
	});

	it('falls back to the age label for a non-muted alert', () => {
		render(AlertRow, {
			props: { alert: fakeAlert({ state: 'open' }), ruleName: 'r', selected: false }
		});

		expect(screen.queryByText(/^muted/)).toBeNull();
	});
});
