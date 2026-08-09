import { test, expect } from '@playwright/test';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import { mintSessionToken, SESSION_COOKIE_NAME } from '../src/lib/server/session';

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

function loadApiUrl(): string {
	const envPath = path.resolve(__dirname, '../.env');
	const envContents = readFileSync(envPath, 'utf-8');
	const match = envContents.match(/^API_URL=(.*)$/m);
	if (!match) {
		throw new Error(`API_URL not found in ${envPath}`);
	}
	return match[1].trim();
}

async function deleteRuleByName(apiUrl: string, token: string, name: string): Promise<void> {
	const listResponse = await fetch(`${apiUrl}/rules`, {
		headers: { Authorization: `Bearer ${token}` }
	});
	if (!listResponse.ok) return;
	const rules = (await listResponse.json()) as { id: number; name: string }[];
	const created = rules.find((r) => r.name === name);
	if (!created) return;
	await fetch(`${apiUrl}/rules/${created.id}`, {
		method: 'DELETE',
		headers: { Authorization: `Bearer ${token}` }
	});
}

// This test exercises the real create-rule flow end to end (no mocking), so it
// requires a live siem-api reachable at the API_URL configured in siem-web/.env.
// Without one, the /alerts?state=rules page load itself fails with a 502 before
// any Playwright selector runs — same requirement the rest of this e2e suite
// already has for reads; this test is the first one in the suite to also write
// (create a rule), so it cleans up after itself via a direct siem-api call below.
test('creating a rule from a template shows it in the Rules list', async ({ page, context }) => {
	const apiUrl = loadApiUrl();
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

	await page.goto(`${BASE_URL}/alerts?state=rules`);

	const newRuleButton = page.getByRole('button', { name: '+ New rule' });
	await expect(newRuleButton).toBeVisible();
	await newRuleButton.click();

	const uniqueName = `e2e-vpn-connect-${Date.now()}`;
	await page.getByLabel('Template').selectOption({ label: 'VPN connection' });
	await page.getByLabel('Name').fill(uniqueName);
	await page.getByRole('button', { name: 'Create rule' }).click();

	try {
		await expect(page.locator('.rule-form')).toHaveCount(0);
		await expect(page.getByText(uniqueName)).toBeVisible();
	} finally {
		await deleteRuleByName(apiUrl, token, uniqueName);
	}
});
