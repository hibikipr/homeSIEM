import { describe, it, expect, vi } from 'vitest';
import { GET } from './+server';
import { SESSION_COOKIE_NAME } from '$lib/server/session';

vi.mock('$env/dynamic/private', () => ({
  env: { OIDC_LOGOUT_URL: 'https://pocketid.townsville.cc/api/oidc/end-session' }
}));

describe('GET /auth/logout', () => {
  it('clears the session cookie and redirects to the OIDC end-session URL', async () => {
    const deleteCookie = vi.fn();

    await expect(GET({ cookies: { delete: deleteCookie } } as never)).rejects.toMatchObject({
      status: 302,
      location: 'https://pocketid.townsville.cc/api/oidc/end-session'
    });

    expect(deleteCookie).toHaveBeenCalledWith(SESSION_COOKIE_NAME, { path: '/' });
  });
});
