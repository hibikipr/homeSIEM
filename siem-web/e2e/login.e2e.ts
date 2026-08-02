import { test } from '@playwright/test';

test('unauthenticated visitors are redirected to Pocket ID', async ({ page }) => {
	// The brief's original assertion (`page.waitForURL(/pocketid\.townsville\.cc\/authorize/)`)
	// does not hold up against the real instance: Pocket ID's /authorize endpoint validates
	// client_id synchronously and issues a server-side 302 straight to
	// /interaction/error?error=The+requested+OAuth+2.0+Client+does+not+exist when the client
	// isn't registered (verified with curl: `HTTP/1.1 302 Found` / `Location: /interaction/error...`
	// on the very first response, no page ever renders at /authorize). Since there's no
	// OIDC_CLIENT_ID registered for "homeSIEM" in this environment (see report), the browser's
	// committed URL is always /interaction/error, never /authorize, so waitForURL on /authorize
	// times out no matter what client id is supplied.
	//
	// What we CAN verify for real, without a registered client: that siem-web's auth gate
	// (Task 7) and /auth/login route (Task 5) perform real OIDC discovery against Pocket ID
	// and issue a real outbound request to its /authorize endpoint. That's what this asserts.
	const authorizeRequest = page.waitForRequest((req) =>
		req.url().startsWith('https://pocketid.townsville.cc/authorize')
	);
	await page.goto('/');
	await authorizeRequest;
});

// Step 3 (full login flow) was investigated but not implemented — see
// task-13-report.md for the full writeup. Summary: there is no OIDC client
// registered for "homeSIEM" in the real Pocket ID instance available to this
// environment (OIDC_CLIENT_ID is blank in every .env.example in this repo,
// including design_handoff_homesiem/reference/.env.example — it's a value a
// human has to create via Pocket ID's admin console, which this environment
// has no credentials for). Without a real registered client, Pocket ID's
// /authorize endpoint won't complete an authorization_code exchange no
// matter how the WebAuthn step is emulated, so extending this test past the
// redirect assertion above would require inventing UI interactions that
// can't be verified against the real instance — exactly what the brief
// warns against. Note: this task's brief assumed manual login was already
// verified in Task 11 Step 6, but Task 11's own report says that step was
// deferred (non-interactive environment, no real browser) — so the full
// login flow has *not* actually been manually verified anywhere yet. That
// remains open until someone with an interactive browser + a registered
// "homeSIEM" OIDC client + WebAuthn credential can run it by hand.
