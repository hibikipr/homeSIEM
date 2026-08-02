// See https://svelte.dev/docs/kit/types#app.d.ts
// for information about these interfaces
import type { AlertResponse } from '$lib/server/siemApiClient';
import type { CountryCount } from '$lib/wall';

declare global {
	namespace App {
		// interface Error {}
		interface Locals {
			user?: { userId: number; email: string; displayName: string; groups: string[]; role: string };
			sessionToken?: string;
		}
		interface PageData {
			user?: { userId: number; email: string; displayName: string; groups: string[]; role: string };
			activeRoute?: string;
			eventCount24h?: number;
			heatGrid?: { source: string; hours: string[] }[];
			openAlertCount?: number;
			triageAlerts?: AlertResponse[];
			countryBreakdown?: CountryCount[];
		}
		// interface PageState {}
		// interface Platform {}
	}
}

declare module '@phosphor-icons/web/regular';

export {};
