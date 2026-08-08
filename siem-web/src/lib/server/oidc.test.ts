import { describe, it, expect } from 'vitest';
import { extractOidcClaims } from './oidc';

describe('extractOidcClaims', () => {
	it('maps a full claims object', () => {
		const claims = extractOidcClaims({
			sub: 'oidc-sub-1',
			email: 'alice@townsville.cc',
			name: 'Alice',
			groups: ['siem-analysts', 'homelab']
		});
		expect(claims).toEqual({
			sub: 'oidc-sub-1',
			email: 'alice@townsville.cc',
			displayName: 'Alice',
			groups: ['siem-analysts', 'homelab'],
			picture: ''
		});
	});

	it('defaults groups to an empty array when absent', () => {
		const claims = extractOidcClaims({ sub: 'oidc-sub-1', email: 'a@b.c', name: 'A' });
		expect(claims.groups).toEqual([]);
	});

	it('falls back displayName to email, then sub, when name is absent', () => {
		expect(extractOidcClaims({ sub: 'oidc-sub-1', email: 'a@b.c' }).displayName).toBe('a@b.c');
		expect(extractOidcClaims({ sub: 'oidc-sub-1' }).displayName).toBe('oidc-sub-1');
	});

	it('throws when sub is missing', () => {
		expect(() => extractOidcClaims({ email: 'a@b.c' })).toThrow();
	});

	it('filters out non-string entries in a malformed groups array', () => {
		const claims = extractOidcClaims({ sub: 'oidc-sub-1', groups: ['ok', 42, null, 'also-ok'] });
		expect(claims.groups).toEqual(['ok', 'also-ok']);
	});

	it('maps the picture claim when present', () => {
		const claims = extractOidcClaims({
			sub: 'oidc-sub-1',
			email: 'alice@townsville.cc',
			name: 'Alice',
			picture: 'https://pocketid.townsville.cc/api/users/oidc-sub-1/profile-picture.png'
		});
		expect(claims.picture).toBe(
			'https://pocketid.townsville.cc/api/users/oidc-sub-1/profile-picture.png'
		);
	});

	it('defaults picture to an empty string when absent or non-string', () => {
		expect(extractOidcClaims({ sub: 'oidc-sub-1' }).picture).toBe('');
		expect(extractOidcClaims({ sub: 'oidc-sub-1', picture: 42 }).picture).toBe('');
	});
});
