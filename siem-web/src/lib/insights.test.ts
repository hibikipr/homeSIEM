import { describe, it, expect } from 'vitest';
import { uniquePrograms } from './insights';
import type { InsightEvidence } from './server/siemApiClient';

function fakeEvidence(overrides: Partial<InsightEvidence> = {}): InsightEvidence {
	return { program: 'nginx-proxy-manager', sample_message: 'boom', count: 1, ...overrides };
}

describe('uniquePrograms', () => {
	it('returns an empty list for no evidence', () => {
		expect(uniquePrograms([])).toEqual([]);
	});

	it('dedupes repeated programs, preserving first-seen order', () => {
		const evidence = [
			fakeEvidence({ program: 'siem-ingest' }),
			fakeEvidence({ program: 'nginx-proxy-manager' }),
			fakeEvidence({ program: 'siem-ingest' })
		];

		expect(uniquePrograms(evidence)).toEqual(['siem-ingest', 'nginx-proxy-manager']);
	});

	it('surfaces a blank program as "(unknown)" instead of dropping it', () => {
		expect(uniquePrograms([fakeEvidence({ program: '' })])).toEqual(['(unknown)']);
	});
});
