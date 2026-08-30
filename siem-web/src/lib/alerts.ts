import type { AlertSample } from './server/siemApiClient';

export interface AlertStats {
	matchedEvents: number;
	distinctPorts: number[];
	sourceIps: string[];
	// One of:
	// - a comma-separated list of threatlist tags (e.g. "spamhaus") - at
	//   least one sample's IP matched a known-bad entry.
	// - "clean" - at least one sample had a real geoip/threat-intel lookup
	//   done against it (siem-ingest's enrich_geo set .threat_intel, even
	//   if null), and none matched.
	// - "unknown" - no sample ever had a lookup done at all, because none
	//   carried a public src_ip/dst_ip for enrich_geo to resolve (see
	//   vector.toml's geoip_ip selection) - not "checked, found nothing,"
	//   genuinely never checked.
	reputation: string;
}

export function deriveAlertStats(samples: AlertSample[]): AlertStats {
	const ports = new Set<number>();
	const ips = new Set<string>();
	const threatTags = new Set<string>();
	let anyChecked = false;

	for (const sample of samples) {
		let parsed: unknown;
		try {
			parsed = JSON.parse(sample.line);
		} catch {
			continue;
		}
		if (typeof parsed !== 'object' || parsed === null) continue;

		const obj = parsed as Record<string, unknown>;
		if (typeof obj.dst_port === 'number') ports.add(obj.dst_port);
		if (typeof obj.src_ip === 'string' && obj.src_ip !== '') ips.add(obj.src_ip);

		// siem-ingest's enrich_geo (vector.toml) sets .threat_intel on every
		// event it could resolve a public IP for - null when the resolved
		// IP didn't match the threatlist, a tag string when it did. The key
		// is only ever absent when no public src_ip/dst_ip existed to check
		// in the first place (see geoip_ip's src-then-dst fallback there).
		if ('threat_intel' in obj) {
			anyChecked = true;
			if (typeof obj.threat_intel === 'string' && obj.threat_intel !== '') {
				threatTags.add(obj.threat_intel);
			}
		}
	}

	const reputation =
		threatTags.size > 0 ? [...threatTags].join(', ') : anyChecked ? 'clean' : 'unknown';

	return {
		matchedEvents: samples.length,
		distinctPorts: [...ports].sort((a, b) => a - b),
		sourceIps: [...ips],
		reputation
	};
}
