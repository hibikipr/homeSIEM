import { render, screen } from '@testing-library/svelte';
import { describe, expect, it } from 'vitest';
import SettingsPage from './+page.svelte';

describe('Settings page', () => {
	it('renders the authentication experience from the design requirements', () => {
		render(SettingsPage);

		expect(screen.getByRole('heading', { name: 'Authentication' })).toBeTruthy();
		expect(screen.getByText('Group → role mapping')).toBeTruthy();
		expect(screen.getByRole('button', { name: /test connection/i })).toBeTruthy();
		expect(screen.getByText('Continue with PocketID')).toBeTruthy();
	});
});
