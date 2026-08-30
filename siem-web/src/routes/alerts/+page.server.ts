import { error, redirect } from '@sveltejs/kit';
import { env } from '$env/dynamic/private';
import type { PageServerLoad } from './$types';
import { SiemApiClient, SiemApiError } from '$lib/server/siemApiClient';
import { deriveAlertStats } from '$lib/alerts';

export const load: PageServerLoad = async ({ locals, url }) => {
	const client = new SiemApiClient({ baseUrl: env.API_URL as string });
	const token = locals.sessionToken as string;

	const tabParam = url.searchParams.get('state');
	const tab: 'open' | 'acked' | 'muted' | 'rules' =
		tabParam === 'acked' || tabParam === 'muted' || tabParam === 'rules' ? tabParam : 'open';
	const selectedId = url.searchParams.get('id');

	// A source-quiet/first-seen alert's title/body is a static string
	// baked in at the moment it was raised (e.g. "source \"192.168.3.223\"
	// has gone silent") - renaming the source afterwards, in Sources,
	// never touches that already-stored text. Rather than rewrite raw
	// title/body strings (fragile - the same alert can be touched/reopened
	// many times without ever regenerating its text) or thread a fixed
	// display name through by re-raising, resolve it live at render time
	// instead: fetch the current source list once and let AlertDetail show
	// whatever Sources currently calls this alert's group_key, alongside
	// the (possibly stale) historical title. Supplementary, streamed to the
	// client (not awaited before load() returns - see the return statement)
	// rather than blocking the whole page on it, and a failure here
	// degrades to an empty lookup rather than breaking the page.
	//
	// The same live fetch also backs AlertDetail's fallback for an
	// absence-shaped alert whose stored `context` has no per-source detail
	// - either raised before AbsenceEvaluator started attaching Context at
	// all (an already-open alert from before that shipped, which won't
	// get backfilled unless it happens to touch/reopen again under the
	// exact same source or source-combination), or one whose source has
	// since been deleted from a stored context that did have it. See
	// AlertDetail's liveSources prop. sourceDisplayNames below is derived
	// from this same promise (not a second fetch) so both stream together.
	const sourcesByNamePromise = client
		.getSources(token)
		.then((sources) => Object.fromEntries(sources.map((s) => [s.name, s])))
		.catch((err) => {
			console.error('alerts: sources lookup failed', err);
			return {} as Record<string, Awaited<ReturnType<typeof client.getSources>>[number]>;
		});
	const sourceDisplayNames = sourcesByNamePromise.then((sourcesByName) =>
		Object.fromEntries(Object.entries(sourcesByName).map(([name, s]) => [name, s.display_name]))
	);

	let alerts, rules;
	try {
		[alerts, rules] = await Promise.all([
			tab === 'rules' ? Promise.resolve([]) : client.getAlerts(token, tab),
			client.getRules(token)
		]);
	} catch (err) {
		if (err instanceof SiemApiError) {
			if (err.status === 401 || err.status === 403) {
				redirect(302, '/auth/logout');
			}
			error(502, 'siem-api unavailable');
		}
		throw err;
	}

	const selectedAlert =
		tab !== 'rules' && selectedId
			? (alerts.find((a) => a.id === Number(selectedId)) ?? null)
			: null;
	const selectedRule =
		tab === 'rules' && selectedId ? (rules.find((r) => r.id === Number(selectedId)) ?? null) : null;

	let selectedSamples;
	try {
		selectedSamples = selectedAlert ? await client.getAlertSamples(token, selectedAlert.id) : [];
	} catch (err) {
		if (err instanceof SiemApiError) {
			if (err.status === 401 || err.status === 403) {
				redirect(302, '/auth/logout');
			}
			error(502, 'siem-api unavailable');
		}
		throw err;
	}

	return {
		tab,
		alerts,
		rules,
		selectedAlert,
		selectedSamples,
		stats: selectedAlert ? deriveAlertStats(selectedSamples) : null,
		selectedRule,
		sourceDisplayNames,
		liveSourcesByName: sourcesByNamePromise,
		userRole: locals.user?.role
	};
};
