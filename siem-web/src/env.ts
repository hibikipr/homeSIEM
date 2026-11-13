import { defineEnvVars } from '@sveltejs/kit/env';

// @migration-task Review usage of dynamic environment variables. They fall back to the empty string if not present, which may not be what you want.
export const variables = defineEnvVars({
	SESSION_SECRET: { schema: (input) => input },
	API_URL: { schema: (input) => input },
	TZ: { schema: (input) => input },
	GRAFANA_HOST_HEALTH_URL: { schema: (input) => input },
	OIDC_LOGOUT_URL: { schema: (input) => input },
	OIDC_ISSUER: { schema: (input) => input },
	OIDC_CLIENT_ID: { schema: (input) => input },
	APP_URL: { schema: (input) => input }
});
