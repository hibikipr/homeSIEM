import { describe, it, expect, afterEach } from 'vitest';
import { render, screen, cleanup } from '@testing-library/svelte';
import AlertDetail from './AlertDetail.svelte';
import type { AlertResponse } from '$lib/server/siemApiClient';
import type { AlertStats } from '$lib/alerts';

// See RuleDetail.svelte.test.ts for why this is needed - @testing-library/svelte's
// auto-cleanup only self-registers when `afterEach` is a vitest global.
afterEach(() => cleanup());

function fakeAlert(overrides: Partial<AlertResponse> = {}): AlertResponse {
	return {
		id: 1,
		rule_id: 9,
		group_key: '192.168.3.44',
		severity: 'warning',
		title: 'Repeated auth failure',
		body: '12 events from 192.168.3.44',
		event_count: 12,
		state: 'open',
		first_seen_at: '2026-08-30T00:00:00Z',
		last_seen_at: '2026-08-30T00:05:00Z',
		...overrides
	};
}

const fakeStats: AlertStats = {
	matchedEvents: 12,
	distinctPorts: [22, 443],
	sourceIps: ['192.168.3.44'],
	reputation: 'unknown'
};

const baseProps = {
	alert: fakeAlert(),
	samples: [],
	stats: fakeStats,
	rule: undefined,
	sourceDisplayNames: {},
	liveSources: {}
};

describe('AlertDetail', () => {
	it('hides Acknowledge and Mute when canEdit is false, but always shows Block at gateway', () => {
		render(AlertDetail, { props: { ...baseProps, canEdit: false } });

		expect(screen.queryByRole('button', { name: 'Acknowledge' })).toBeNull();
		expect(screen.queryByRole('button', { name: 'Mute rule 1h' })).toBeNull();
		expect(screen.getByRole('button', { name: 'Block at gateway' })).toBeTruthy();
	});

	it('shows Acknowledge and Mute when canEdit is true', () => {
		render(AlertDetail, { props: { ...baseProps, canEdit: true } });

		expect(screen.getByRole('button', { name: 'Acknowledge' })).toBeTruthy();
		expect(screen.getByRole('button', { name: 'Mute rule 1h' })).toBeTruthy();
	});

	it('defaults canEdit to false when the prop is omitted', () => {
		render(AlertDetail, { props: baseProps });

		expect(screen.queryByRole('button', { name: 'Acknowledge' })).toBeNull();
	});
});
