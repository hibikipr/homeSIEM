import { describe, it, expect, vi, afterEach } from 'vitest';
import { render, screen, fireEvent, waitFor, cleanup } from '@testing-library/svelte';
import Page from './+page.svelte';

const gotoMock = vi.fn();
vi.mock('$app/navigation', () => ({ goto: (...args: unknown[]) => gotoMock(...args) }));

// See RuleDetail.svelte.test.ts for why this is needed - @testing-library/svelte's
// auto-cleanup only self-registers when `afterEach` is a vitest global.
afterEach(() => cleanup());

describe('local-login page', () => {
	it('submits credentials and navigates home on success', async () => {
		gotoMock.mockClear();
		vi.spyOn(globalThis, 'fetch').mockResolvedValue(
			new Response(JSON.stringify({ ok: true }), { status: 200 })
		);
		render(Page);

		await fireEvent.input(screen.getByLabelText('Username'), { target: { value: 'admin' } });
		await fireEvent.input(screen.getByLabelText('Password'), { target: { value: 'hunter2' } });
		await fireEvent.click(screen.getByRole('button', { name: 'Sign in' }));

		await waitFor(() => expect(gotoMock).toHaveBeenCalledWith('/'));
		expect(fetch).toHaveBeenCalledWith(
			'/auth/local-login',
			expect.objectContaining({
				method: 'POST',
				body: JSON.stringify({ username: 'admin', password: 'hunter2' })
			})
		);
	});

	it('shows the server error message and does not navigate on failure', async () => {
		gotoMock.mockClear();
		vi.spyOn(globalThis, 'fetch').mockResolvedValue(
			new Response(JSON.stringify({ error: 'Invalid username or password.' }), { status: 401 })
		);
		render(Page);

		await fireEvent.input(screen.getByLabelText('Username'), { target: { value: 'admin' } });
		await fireEvent.input(screen.getByLabelText('Password'), { target: { value: 'wrong' } });
		await fireEvent.click(screen.getByRole('button', { name: 'Sign in' }));

		await waitFor(() => expect(screen.getByText('Invalid username or password.')).toBeTruthy());
		expect(gotoMock).not.toHaveBeenCalled();
	});
});
