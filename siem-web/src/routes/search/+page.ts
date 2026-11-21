import type { PageLoad } from './$types';
import { extractSrcIp } from '#lib/search.ts';

// Row selection lives here, not in +page.server.ts: this load reads
// `?preview=` and the server load deliberately doesn't, so clicking a row
// reruns only this (cheap) load against the entries the server already
// returned, instead of re-running the full Loki search.
//
// The entries now stream in (data.results is a promise), so the selected
// entry itself is resolved in +page.svelte once they arrive; this load only
// parses the index and starts the context lookup, chained off the same
// promise so it fires as soon as the entry is known.
export const load: PageLoad = async ({ data, url, fetch }) => {
	const previewParam = url.searchParams.get('preview');
	const parsedPreview = previewParam !== null ? Number(previewParam) : null;
	const previewIndex =
		parsedPreview !== null && Number.isInteger(parsedPreview) ? parsedPreview : null;

	// Supplementary callout - streamed (not awaited) so the inspector
	// renders immediately, and null (not a promise) when nothing is
	// selected. Resolves to null when the entry has no src_ip, the entries
	// failed to load, or the lookup fails - never rejects.
	let contextSummary: Promise<{ count: number } | null> | null = null;
	if (previewIndex !== null) {
		contextSummary = data.results
			.then(async (results) => {
				const entry = results?.entries[previewIndex] ?? null;
				const srcIp = entry ? extractSrcIp(entry.Line) : null;
				if (!srcIp) return null;
				const res = await fetch(`/api/search/context-count?ip=${encodeURIComponent(srcIp)}`);
				return res.ok ? ((await res.json()) as { count: number }) : null;
			})
			.catch((err) => {
				console.error('search: context summary lookup failed', err);
				return null;
			});
	}

	return { ...data, previewIndex, contextSummary };
};
