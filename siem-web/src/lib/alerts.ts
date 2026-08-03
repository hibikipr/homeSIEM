import type { AlertSample } from './server/siemApiClient';

export interface AlertStats {
	matchedEvents: number;
	distinctPorts: number[];
	sourceIps: string[];
	reputation: string;
}

export function deriveAlertStats(samples: AlertSample[]): AlertStats {
	const ports = new Set<number>();
	const ips = new Set<string>();

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
	}

	return {
		matchedEvents: samples.length,
		distinctPorts: [...ports].sort((a, b) => a - b),
		sourceIps: [...ips],
		reputation: 'unknown'
	};
}
