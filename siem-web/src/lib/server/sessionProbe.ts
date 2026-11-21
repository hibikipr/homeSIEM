import { redirect, error } from '@sveltejs/kit';
import { SiemApiError, type AlertResponse, type SiemApiClient } from './siemApiClient.ts';

// Streamed (un-awaited) promises can't redirect: by the time they settle the
// response headers are long sent. Pages that stream their slow Loki-backed
// panels therefore await this one cheap call first. Every siem-api route sits
// behind the same `protect` middleware, so a stale/demoted session fails here
// with 401/403 exactly as it would on the slow call, and we redirect before
// any HTML goes out. /alerts?state=open is SQLite-only (fast), and the Wall
// needs the result anyway for its triage lane.
export async function fetchOpenAlertsOrBail(
	client: Pick<SiemApiClient, 'getAlerts'>,
	token: string
): Promise<AlertResponse[]> {
	try {
		return await client.getAlerts(token, 'open');
	} catch (err) {
		if (err instanceof SiemApiError) {
			if (err.status === 401 || err.status === 403) {
				redirect(302, '/auth/logout');
			}
			error(502, 'siem-api unavailable');
		}
		throw err;
	}
}
