import { render } from 'svelte/server';
import { describe, expect, it } from 'vitest';
import SettingsPage from './settings/+page.svelte';

describe('Settings page', () => {
	it('renders the authentication experience from the design requirements', () => {
		const { body } = render(SettingsPage);
		const html = body.toString();

		expect(html).toContain('Authentication');
		expect(html).toContain('Group → role mapping');
		expect(html).toContain('Test connection');
		expect(html).toContain('Continue with PocketID');
	});
});
