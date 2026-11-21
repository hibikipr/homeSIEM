import { describe, it, expect, afterEach, beforeEach, vi } from 'vitest';
import { render, screen, cleanup, fireEvent, waitFor } from '@testing-library/svelte';

const { invalidate, invalidateAll } = vi.hoisted(() => ({
	invalidate: vi.fn(() => Promise.resolve()),
	invalidateAll: vi.fn(() => Promise.resolve())
}));
vi.mock('$app/navigation', () => ({ invalidate, invalidateAll }));

import TriageCard from './TriageCard.svelte';
import { alertOverrides } from '#lib/optimisticAlerts.svelte.ts';
import type { AlertResponse } from '#lib/server/siemApiClient.ts';

afterEach(() => cleanup());
beforeEach(() => {
	invalidate.mockClear();
	invalidateAll.mockClear();
	alertOverrides.clear();
	vi.unstubAllGlobals();
});

const alert: AlertResponse = {
	id: 7,
	rule_id: 1,
	group_key: 'k',
	severity: 'critical',
	title: 'Brute force',
	body: 'many failures',
	event_count: 5,
	state: 'open',
	first_seen_at: '2026-08-30T00:00:00Z',
	last_seen_at: '2026-08-30T00:05:00Z'
};

function deferredFetch() {
	let resolve!: (r: { ok: boolean }) => void;
	const promise = new Promise<{ ok: boolean }>((r) => (resolve = r));
	const fetchMock = vi.fn(() => promise);
	vi.stubGlobal('fetch', fetchMock);
	return { resolve, fetchMock };
}

describe('TriageCard', () => {
	it('mutes optimistically, then invalidates only the alerts key', async () => {
		const { resolve, fetchMock } = deferredFetch();
		render(TriageCard, { props: { alert } });

		await fireEvent.click(screen.getByRole('button', { name: 'Mute 1h' }));

		// Gone before the server has answered.
		expect(screen.queryByText('Brute force')).toBeNull();
		expect(fetchMock).toHaveBeenCalledWith('/api/alerts/7/mute', { method: 'POST' });
		expect(invalidate).not.toHaveBeenCalled();

		resolve({ ok: true });
		await waitFor(() => expect(invalidate).toHaveBeenCalledWith('siem:alerts'));
		expect(invalidate).toHaveBeenCalledTimes(1);
		expect(invalidateAll).not.toHaveBeenCalled();
	});

	it('rolls back and shows the error when the request fails', async () => {
		const { resolve } = deferredFetch();
		render(TriageCard, { props: { alert } });

		await fireEvent.click(screen.getByRole('button', { name: 'Mute 1h' }));
		expect(screen.queryByText('Brute force')).toBeNull();

		resolve({ ok: false });
		expect(await screen.findByText('Brute force')).toBeTruthy();
		expect(screen.getByText('Failed to mute alert.')).toBeTruthy();
		expect(invalidate).not.toHaveBeenCalled();
		expect(invalidateAll).not.toHaveBeenCalled();
	});
});
