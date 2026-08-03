import { describe, it, expect } from 'vitest';
import { deriveAlertStats } from './alerts';
import type { AlertSample } from './server/siemApiClient';

function sample(line: string): AlertSample {
	return { id: 1, ts: '2026-08-02T00:00:00Z', line };
}

describe('deriveAlertStats', () => {
	it('extracts distinct ports and source IPs from sample JSON', () => {
		const samples = [
			sample('{"src_ip":"10.0.0.5","dst_port":443}'),
			sample('{"src_ip":"10.0.0.5","dst_port":22}'),
			sample('{"src_ip":"10.0.0.9","dst_port":443}')
		];

		const stats = deriveAlertStats(samples);

		expect(stats.matchedEvents).toBe(3);
		expect(stats.distinctPorts).toEqual([22, 443]);
		expect(stats.sourceIps).toEqual(['10.0.0.5', '10.0.0.9']);
		expect(stats.reputation).toBe('unknown');
	});

	it('skips malformed JSON and entries with no src_ip/dst_port', () => {
		const samples = [sample('not json'), sample('{}'), sample('{"src_ip":"10.0.0.5"}')];

		const stats = deriveAlertStats(samples);

		expect(stats.matchedEvents).toBe(3);
		expect(stats.distinctPorts).toEqual([]);
		expect(stats.sourceIps).toEqual(['10.0.0.5']);
	});
});
