// Dependency keys for SvelteKit's `depends()` / `invalidate()`. A server load
// registers the keys it reads from; an action invalidates only the key(s) it
// actually changed instead of `invalidateAll()` re-running every load on the
// page (including slow Loki-backed ones).
//
// Keys must be a valid URI per SvelteKit, hence the `siem:` scheme.
export const DEP_ALERTS = 'siem:alerts'; // open/acked/muted alert lists, nav open-alert count
export const DEP_SOURCES = 'siem:sources'; // source registry, display names, claim state
export const DEP_INSIGHTS = 'siem:insights'; // insights + muted insight patterns
export const DEP_RULES = 'siem:rules'; // alert rules
export const DEP_SETTINGS = 'siem:settings'; // role mappings, notification/ollama settings
export const DEP_STATS = 'siem:stats'; // Wall event stats / heat grid / country breakdown
