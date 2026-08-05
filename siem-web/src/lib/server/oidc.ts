import * as client from 'openid-client';

export interface OidcConfig {
	issuer: string;
	clientId: string;
	redirectUri: string;
}

export interface OidcClaims {
	sub: string;
	email: string;
	displayName: string;
	groups: string[];
}

export interface LoginRedirect {
	url: string;
	codeVerifier: string;
	state: string;
}

export const PKCE_COOKIE_NAME = 'siem_pkce_verifier';
export const STATE_COOKIE_NAME = 'siem_oidc_state';

let cachedConfig: client.Configuration | undefined;

async function getConfig(oidcConfig: OidcConfig): Promise<client.Configuration> {
	if (!cachedConfig) {
		// No client_secret is provided, so openid-client defaults the client
		// authentication method to `None` (public client) — correct for a
		// browser-redirect PKCE flow, which never sends a shared secret.
		cachedConfig = await client.discovery(new URL(oidcConfig.issuer), oidcConfig.clientId);
	}
	return cachedConfig;
}

export async function buildLoginRedirect(oidcConfig: OidcConfig): Promise<LoginRedirect> {
	const config = await getConfig(oidcConfig);
	const codeVerifier = client.randomPKCECodeVerifier();
	const codeChallenge = await client.calculatePKCECodeChallenge(codeVerifier);
	// Pocket ID rejects the callback with invalid_state if this is absent
	// (it requires state, unlike some providers that treat PKCE alone as
	// sufficient CSRF protection) — confirmed against a real deployment.
	const state = client.randomState();

	const authUrl = client.buildAuthorizationUrl(config, {
		redirect_uri: oidcConfig.redirectUri,
		scope: 'openid profile email groups',
		code_challenge: codeChallenge,
		code_challenge_method: 'S256',
		state
	});

	return { url: authUrl.href, codeVerifier, state };
}

export async function completeLogin(
	oidcConfig: OidcConfig,
	callbackUrl: URL,
	codeVerifier: string,
	expectedState: string
): Promise<OidcClaims> {
	const config = await getConfig(oidcConfig);
	const tokens = await client.authorizationCodeGrant(config, callbackUrl, {
		pkceCodeVerifier: codeVerifier,
		expectedState
	});
	const idTokenClaims = tokens.claims();
	if (!idTokenClaims) {
		throw new Error('no ID token claims returned from token endpoint');
	}
	return extractOidcClaims(idTokenClaims as Record<string, unknown>);
}

export function extractOidcClaims(raw: Record<string, unknown>): OidcClaims {
	if (typeof raw.sub !== 'string') {
		throw new Error('ID token missing sub claim');
	}
	const groups = Array.isArray(raw.groups)
		? raw.groups.filter((g): g is string => typeof g === 'string')
		: [];
	const email = typeof raw.email === 'string' ? raw.email : '';
	const displayName = typeof raw.name === 'string' ? raw.name : email || raw.sub;

	return { sub: raw.sub, email, displayName, groups };
}
