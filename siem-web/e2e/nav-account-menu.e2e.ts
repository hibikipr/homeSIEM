import { test, expect } from '@playwright/test';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import { mintSessionToken, SESSION_COOKIE_NAME } from '../src/lib/server/session';

// Regression test for the bug this branch fixes: the Nav bar avatar used to be
// a plain `<a href="/auth/logout">` that deleted the session cookie on a single
// click with zero warning. It's now a toggle button that opens a dropdown, and
// "Sign out" inside that dropdown is the only element that still navigates to
// /auth/logout. This test asserts both halves of that contract.

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const BASE_URL = 'http://localhost:4173';

// The webServer runs a production preview build (`pnpm run preview`), which
// loads siem-web/.env for its runtime env vars, including SESSION_SECRET. To
// mint a cookie the running server will actually accept, we have to sign it
// with that exact secret rather than a hardcoded one.
function loadSessionSecret(): Uint8Array {
	const envPath = path.resolve(__dirname, '../.env');
	const envContents = readFileSync(envPath, 'utf-8');
	const match = envContents.match(/^SESSION_SECRET=(.*)$/m);
	if (!match) {
		throw new Error(`SESSION_SECRET not found in ${envPath}`);
	}
	return new Uint8Array(Buffer.from(match[1].trim(), 'base64'));
}

test('avatar click opens the menu without signing out; Sign out navigates to /auth/logout', async ({
	page,
	context
}) => {
	const secret = loadSessionSecret();
	const token = await mintSessionToken(
		{
			sub: 'oidc-sub-e2e',
			userId: 1,
			email: 'alice@townsville.cc',
			displayName: 'Alice Analyst',
			groups: ['siem-analysts'],
			role: 'admin',
			picture: ''
		},
		secret
	);

	await context.addCookies([
		{
			name: SESSION_COOKIE_NAME,
			value: token,
			url: BASE_URL,
			httpOnly: true,
			secure: true,
			sameSite: 'Lax'
		}
	]);

	await page.goto('/');
	const urlBeforeClick = page.url();

	// Clicking the avatar alone must never navigate away or drop the session.
	await page.getByRole('button', { name: 'Account menu' }).click();

	expect(page.url()).toBe(urlBeforeClick);
	const cookiesAfterOpen = await context.cookies(BASE_URL);
	expect(cookiesAfterOpen.some((cookie) => cookie.name === SESSION_COOKIE_NAME)).toBe(true);

	const menu = page.locator('.account-menu');
	await expect(menu).toBeVisible();
	await expect(menu.locator('.account-menu-name')).toHaveText('Alice Analyst');
	await expect(menu.locator('.account-menu-email')).toHaveText('alice@townsville.cc');

	// Only "Sign out" inside the open menu is allowed to navigate to /auth/logout.
	const logoutRequest = page.waitForRequest((req) => req.url().includes('/auth/logout'));
	await menu.getByRole('link', { name: 'Sign out' }).click();
	await logoutRequest;
});
