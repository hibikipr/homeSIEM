import { API_URL, GRAFANA_HOST_HEALTH_URL } from '$app/env/private';
import type { PageServerLoad } from './$types';
import { SiemApiClient, type Insight } from '#lib/server/siemApiClient.ts';
import { fetchOpenAlertsOrBail } from '#lib/server/sessionProbe.ts';
import { topTriageAlerts, buildSourceLabels, type CountryCount } from '#lib/wall.ts';

interface WallStats {
	eventCount24h: number;
	heatGrid: { source: string; hours: string[] }[];
	hourlyTotals: { hour_start: string; count: number }[];
}

export const load: PageServerLoad = async ({ locals }) => {
	const client = new SiemApiClient({ baseUrl: API_URL as string });
	const token = locals.sessionToken as string;

	// Streamed to the client rather than awaited here - none of these three
	// are needed to render the Wall's primary content (stats/alerts/heatmap
	// below), so blocking the whole page on them turned "first paint" into
	// "slowest of five network calls." SvelteKit sends the rest of the page
	// immediately and patches these in via +page.svelte's {#await} blocks
	// once each resolves. Each degrades to an empty result on failure rather
	// than ever redirecting/erroring - same posture as +layout.server.ts's
	// nav summary, just now also true of countryBreakdown (previously
	// blocking, see git history for why it moved here).

	// The Insights panel is supplementary, not gated content - a failure here
	// (including "insights isn't configured at all") must never break the
	// Wall.
	const insights: Promise<Insight[]> = client.getInsights(token).catch((err) => {
		console.error('wall: insights lookup failed', err);
		return [];
	});

	// HeatGrid's rows only ever carry the raw source name (siem-api's
	// /events/stats has no notion of display_name), so an operator-set
	// rename needs this separate lookup to show up at all - HeatGrid
	// defaults to an empty label map, so rows just show raw names until
	// this resolves.
	const sourceLabels = client
		.getSources(token)
		.then((sources) =>
			buildSourceLabels(
				sources.filter((s) => s.claimed).map((s) => ({ name: s.name, displayName: s.display_name }))
			)
		)
		.catch((err) => {
			console.error('wall: sources lookup failed', err);
			return {} as Record<string, string>;
		});

	// A real Loki-side aggregate (siem-api's queryCountryFacetCounts) over
	// the full 24h default window, not derived from a capped sample of
	// entries - this used to fetch 200 raw entries and count geoip fields
	// client-side, which came back empty in ordinary use even when
	// enrich_geo was enriching correctly (geoip-bearing events are a small
	// fraction of overall volume, easily squeezed out of any small sample).
	// entries/volume/count=false and facets=country skip every Loki scan
	// this call never reads - it used to also run the severity, program and
	// source facet aggregates plus a total count, all over the same 24h.
	const countryBreakdown: Promise<CountryCount[]> = client
		.search(token, { entries: 'false', volume: 'false', count: 'false', facets: 'country' })
		.then((result) =>
			(result.facets?.country ?? []).map((c) => ({ country: c.value, count: c.count }))
		)
		.catch((err) => {
			console.error('wall: country breakdown lookup failed', err);
			return [];
		});

	// Stats is the slow call (Loki-backed aggregate, seconds on a cold cache),
	// so it streams instead of gating the first byte. A stream can't
	// redirect, so auth is enforced by the awaited open-alerts call below
	// (SQLite-fast, same `protect` middleware, needed for the triage lane
	// anyway): a 401/403 there still sends the user to /auth/logout before
	// any HTML is sent. A failure of the stats call itself resolves to null
	// and the page renders an inline "couldn't load" message.
	const stats: Promise<WallStats | null> = client
		.getEventsStats(token)
		.then((s) => ({
			eventCount24h: s.event_count_24h,
			heatGrid: s.heat_grid,
			hourlyTotals: s.hourly_totals ?? []
		}))
		.catch((err) => {
			console.error('wall: stats lookup failed', err);
			return null;
		});

	const openAlerts = await fetchOpenAlertsOrBail(client, token);

	return {
		stats,
		sourceLabels,
		openAlertCount: openAlerts.length,
		triageAlerts: topTriageAlerts(openAlerts),
		countryBreakdown,
		insights,
		// Not every deployment has Grafana's public-dashboard sharing set up -
		// undefined here means the Wall omits the card entirely rather than
		// embedding a broken iframe.
		grafanaHostHealthUrl: (GRAFANA_HOST_HEALTH_URL as string) || undefined
	};
};
