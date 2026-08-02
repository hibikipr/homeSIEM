declare namespace App {
	interface PrivateEnv {
		OIDC_ISSUER: string;
		OIDC_CLIENT_ID: string;
		OIDC_LOGOUT_URL: string;
		APP_URL: string;
		API_URL: string;
		SESSION_SECRET: string;
	}
}
