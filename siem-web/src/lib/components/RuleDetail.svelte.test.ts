import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, waitFor, cleanup } from '@testing-library/svelte';
import RuleDetail from './RuleDetail.svelte';
import type { RuleResponse } from '$lib/server/siemApiClient';

// @testing-library/svelte's own auto-cleanup only registers when `afterEach`
// is a global (this project doesn't set vitest's `globals: true`), so
// without this, each render() in this file stacks a fresh copy into the
// document instead of replacing the last one - later tests' getByRole
// queries then fail with "multiple elements found."
afterEach(() => cleanup());

function fakeRule(overrides: Partial<RuleResponse> = {}): RuleResponse {
	return {
		id: 9,
		name: 'search-alert',
		shape: 'threshold',
		logql: '{job="siem"}',
		window_sec: 60,
		threshold: 5,
		group_by: [],
		severity: 'warning',
		destinations: ['inapp'],
		cooldown_sec: 3600,
		interval_sec: 60,
		enabled: true,
		...overrides
	};
}

describe('RuleDetail', () => {
	beforeEach(() => {
		vi.restoreAllMocks();
	});

	it('renders the rule name, severity, and enabled status', () => {
		render(RuleDetail, { props: { rule: fakeRule() } });

		expect(screen.getByRole('heading', { name: 'search-alert' })).toBeTruthy();
		expect(screen.getByText('severity: warning')).toBeTruthy();
		expect(screen.getByText('enabled')).toBeTruthy();
	});

	it('hides Edit/Enable-Disable/Delete when canEdit is false', () => {
		render(RuleDetail, { props: { rule: fakeRule(), canEdit: false } });

		expect(screen.queryByRole('button', { name: 'Edit' })).toBeNull();
		expect(screen.queryByRole('button', { name: 'Delete' })).toBeNull();
		expect(screen.queryByRole('button', { name: 'Disable' })).toBeNull();
	});

	it('shows Edit/Disable/Delete when canEdit is true, and Edit calls onEdit', async () => {
		const onEdit = vi.fn();
		render(RuleDetail, { props: { rule: fakeRule(), canEdit: true, onEdit } });

		await fireEvent.click(screen.getByRole('button', { name: 'Edit' }));

		expect(onEdit).toHaveBeenCalledOnce();
	});

	it('does not call fetch when the delete confirmation is dismissed', async () => {
		vi.spyOn(window, 'confirm').mockReturnValue(false);
		const fetchSpy = vi.spyOn(globalThis, 'fetch');
		const onDeleted = vi.fn();
		render(RuleDetail, { props: { rule: fakeRule(), canEdit: true, onDeleted } });

		await fireEvent.click(screen.getByRole('button', { name: 'Delete' }));

		expect(window.confirm).toHaveBeenCalledWith(`Delete "search-alert"? This can't be undone.`);
		expect(fetchSpy).not.toHaveBeenCalled();
		expect(onDeleted).not.toHaveBeenCalled();
	});

	it('DELETEs the rule and calls onDeleted when the confirmation is accepted', async () => {
		vi.spyOn(window, 'confirm').mockReturnValue(true);
		vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(null, { status: 204 }));
		const onDeleted = vi.fn();
		render(RuleDetail, { props: { rule: fakeRule(), canEdit: true, onDeleted } });

		await fireEvent.click(screen.getByRole('button', { name: 'Delete' }));

		await waitFor(() => expect(onDeleted).toHaveBeenCalledOnce());
		expect(fetch).toHaveBeenCalledWith('/api/rules/9', { method: 'DELETE' });
	});

	it('shows an error and does not call onDeleted when the delete request fails', async () => {
		vi.spyOn(window, 'confirm').mockReturnValue(true);
		vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(null, { status: 403 }));
		const onDeleted = vi.fn();
		render(RuleDetail, { props: { rule: fakeRule(), canEdit: true, onDeleted } });

		await fireEvent.click(screen.getByRole('button', { name: 'Delete' }));

		await waitFor(() =>
			expect(screen.getByText("You don't have permission to delete rules.")).toBeTruthy()
		);
		expect(onDeleted).not.toHaveBeenCalled();
	});
});
