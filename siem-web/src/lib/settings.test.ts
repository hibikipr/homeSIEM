import { describe, it, expect } from 'vitest';
import { roleCapabilityLabel } from './settings';

describe('roleCapabilityLabel', () => {
	it('maps each known role to its capability description', () => {
		expect(roleCapabilityLabel('admin')).toBe('read/write/manage');
		expect(roleCapabilityLabel('analyst')).toBe('read/search/triage');
		expect(roleCapabilityLabel('viewer')).toBe('read only');
	});

	it('falls back to "unknown" for an unrecognized role', () => {
		expect(roleCapabilityLabel('bogus')).toBe('unknown');
	});
});
