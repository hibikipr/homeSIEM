import { describe, it, expect } from 'vitest';
import { SignJWT } from 'jose';
import { mintSessionToken, verifySessionToken, type SessionClaims } from './session';

const secret = new TextEncoder().encode('0123456789abcdef0123456789abcdef');

const testClaims: SessionClaims = {
  sub: 'oidc-sub-1',
  userId: 42,
  email: 'alice@townsville.cc',
  displayName: 'Alice',
  groups: ['siem-analysts'],
  role: 'analyst'
};

describe('mintSessionToken / verifySessionToken', () => {
  it('round-trips claims', async () => {
    const token = await mintSessionToken(testClaims, secret);
    const claims = await verifySessionToken(token, secret);
    expect(claims).toEqual(testClaims);
  });

  it('rejects a token signed with a different secret', async () => {
    const otherSecret = new TextEncoder().encode('ffffffffffffffffffffffffffffffff');
    const token = await mintSessionToken(testClaims, otherSecret);
    await expect(verifySessionToken(token, secret)).rejects.toThrow();
  });

  it('rejects an expired token', async () => {
    const expired = await new SignJWT({
      user_id: testClaims.userId,
      email: testClaims.email,
      display_name: testClaims.displayName,
      groups: testClaims.groups,
      role: testClaims.role
    })
      .setProtectedHeader({ alg: 'HS256' })
      .setSubject(testClaims.sub)
      .setIssuedAt(Math.floor(Date.now() / 1000) - 3600)
      .setExpirationTime(Math.floor(Date.now() / 1000) - 10)
      .sign(secret);

    await expect(verifySessionToken(expired, secret)).rejects.toThrow();
  });

  it('rejects a malformed token', async () => {
    await expect(verifySessionToken('not-a-jwt', secret)).rejects.toThrow();
  });
});
