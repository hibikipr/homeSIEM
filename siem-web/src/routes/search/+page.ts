import type { PageLoad } from './$types';
import { extractSrcIp } from '$lib/search';

// Row selection lives here, not in +page.server.ts: this load reads
// `?preview=` and the server load deliberately doesn't, so clicking a row
// reruns only this (cheap) load against the entries the server already
// returned, instead of re-running the full Loki search.
export const load: PageLoad = async ({ data, url, fetch }) => {
	const previewParam = url.searchParams.get('preview');
	const parsedPreview = previewParam !== null ? Number(previewParam) : null;
	const previewIndex =
		parsedPreview !== null && Number.isInteger(parsedPreview) ? parsedPreview : null;
	const selectedEntry =
		previewIndex !== null && previewIndex >= 0 && previewIndex < data.entries.length
			? data.entries[previewIndex]
			: null;

	// Supplementary callout - streamed (not awaited) so the inspector
	// renders immediately, and null (not a promise) when there's nothing
	// to look up, which {#await} in +page.svelte treats as already
	// resolved. A failure degrades to null rather than breaking the page.
	let contextSummary: Promise<{ count: number } | null> | null = null;
	const srcIp = selectedEntry ? extractSrcIp(selectedEntry.Line) : null;
	if (srcIp) {
		contextSummary = fetch(`/api/search/context-count?ip=${encodeURIComponent(srcIp)}`)
			.then((res) => (res.ok ? (res.json() as Promise<{ count: number }>) : null))
			.catch((err) => {
				console.error('search: context summary lookup failed', err);
				return null;
			});
	}

	return { ...data, previewIndex, selectedEntry, contextSummary };
};
