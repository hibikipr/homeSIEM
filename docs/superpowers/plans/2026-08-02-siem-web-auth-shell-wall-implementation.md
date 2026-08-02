# siem-web: auth/BFF + shell + Wall screen Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the first slice of `siem-web` — OIDC login through Pocket ID, the session/BFF layer that lets `siem-api` trust `siem-web`, the global nav chrome, and Screen 1 (Wall) — per `docs/superpowers/specs/2026-08-02-siem-web-auth-shell-wall-design.md`.

**Architecture:** One SvelteKit app. Server routes/hooks are the "thin BFF" (OIDC callback, session cookie, proxying to siem-api); Svelte components are the presentation layer. No separate backend process.

**Tech Stack:** SvelteKit, pnpm, `openid-client` (OIDC/PKCE), `jose` (internal JWT), Vitest + `@testing-library/svelte`, Playwright.

## Global Constraints

- Module root: `siem-web/` at repo root.
- Package manager: pnpm exclusively — no npm/yarn lockfiles.
- Session cookie name: `siem_session`. Attributes: `HttpOnly; Secure; SameSite=Lax; Path=/`, 12-hour max age, matching the handoff's "Session lifetime 12 hours" setting.
- The internal session JWT is HS256, signed with `SIEM_SESSION_SECRET` (base64, same env var siem-api reads), and MUST carry exactly these claims for siem-api's `TokenVerifier` (already built, not to be changed) to accept it: standard `sub` (registered claim) plus `user_id`, `email`, `display_name`, `groups` (JSON key names, snake_case) and a standard `exp`.
- **No OIDC token of any kind ever reaches client-side JavaScript.** All OIDC/JWKS work and all `Authorization: Bearer` calls to siem-api happen in `+server.ts`/`+page.server.ts`/`hooks.server.ts` (server-only SvelteKit code), never in `<script>` blocks that ship to the browser.
- Styling: CSS custom properties in `src/lib/styles/tokens.css`, Svelte scoped `<style>` per component. No Tailwind, no CSS-in-JS. Every color/spacing/radius/shadow value in component styles must be a `var(--...)` reference — never a raw hex or px literal duplicating a token.
- Design fidelity is **final** — colors, type, spacing, and layout come directly from `design_handoff_homesiem/README.md`'s "Design tokens" and "Screen 1 — Wall" sections (copy that file's `design_handoff_homesiem/` bundle into this worktree first if it isn't already present — it's gitignored reference material, not tracked by git, so a fresh worktree won't have it: `cp -R <a worktree that has it>/design_handoff_homesiem .`). Do not improvise values not present in that document.
- **Known cross-service quirk, not to be silently "fixed" here:** `siem-api`'s `loki.LogEntry` Go struct has no JSON tags, so `/events/search`'s `entries` array serializes fields as `Timestamp`/`Labels`/`Line` (capitalized) — inconsistent with every other siem-api response, which uses `snake_case`. TypeScript types in this plan match that actual wire format exactly. Flag it as a future siem-api fix; do not paper over it by guessing a different casing.
- Testing split: TDD (Vitest) for everything with real logic — session/cookie handling, siem-api client, claims mapping, Wall's data-shaping helpers, `hooks.server.ts`'s auth-gate logic. One Playwright e2e test for the full login flow. **No unit tests for presentational Svelte components** — `.svelte` markup/styling is verified by running the dev server and comparing against `design_handoff_homesiem/designs/homeSIEM Console - Build.dc.html`, not asserted in test code.
- `openid-client`'s exact API surface may have shifted between versions since this plan was written. Tasks touching it give the intended shape from the current major version's functional API, but the implementer MUST check `node_modules/openid-client`'s actual type definitions (or `pnpm why openid-client` / its README) before finalizing that task's code, and adjust import names/call shapes to match what's actually installed — treat the plan's `oidc.ts` code as a strong draft, not gospel, for the parts that call into the library directly. Everything else in this plan (session.ts, siemApiClient.ts, hooks.server.ts, wall.ts, component code) has no such uncertainty and should be implemented exactly as written.
- Every task's code must build cleanly (`pnpm check` — SvelteKit's type/svelte-check) and its tests must pass (`pnpm test`) before moving to the next task.
- Commit after every task with a message describing what the task added — no bundling multiple tasks into one commit.

---

### Task 1: Project scaffold

**Files:**
- Create: `siem-web/` (entire SvelteKit project scaffold)
- Create: `siem-web/src/lib/styles/tokens.css` (empty file, populated in Task 8)
- Create: `siem-web/.env.example`

**Interfaces:**
- Produces: a buildable, empty SvelteKit app that every later task adds to.

- [ ] **Step 1: Scaffold the SvelteKit project**

Run this **non-interactively** (confirmed working syntax — do not use the bare/interactive form, it will hang waiting for prompts you have no way to answer):
```bash
pnpm dlx sv create siem-web --template minimal --types ts --add prettier eslint vitest playwright --install pnpm
```
This scaffolds with TypeScript, Prettier, ESLint, Vitest, and Playwright, and runs `pnpm install` for you. If `sv create` isn't available in this environment's installed tooling version, fall back to `pnpm create svelte@latest siem-web` and check ITS `--help` output for the equivalent non-interactive flags before running it — do not attempt an interactive scaffold in a non-interactive shell either way. Note in your report which command you used.

- [ ] **Step 2: Add the remaining dependencies**

```bash
cd siem-web
pnpm add openid-client jose
```
(Vitest, `@testing-library/svelte`, and Playwright should already be present from Step 1's prompts — if `@testing-library/svelte` wasn't added by the scaffold, add it: `pnpm add -D @testing-library/svelte`.)

- [ ] **Step 3: Create the styles directory and an empty tokens file**

```bash
mkdir -p src/lib/styles
touch src/lib/styles/tokens.css
```

- [ ] **Step 4: Write `.env.example`**

`siem-web/.env.example`:
```
# siem-web configuration
API_URL=http://siem-api:8080
APP_URL=https://siem.townsville.cc
SESSION_SECRET=          # 32 random bytes, base64 — same value as siem-api's SIEM_SESSION_SECRET
OIDC_ISSUER=https://pocketid.townsville.cc
OIDC_CLIENT_ID=          # from Pocket ID, client "homeSIEM"
OIDC_LOGOUT_URL=https://pocketid.townsville.cc/api/oidc/end-session
```

- [ ] **Step 5: Copy the gitignored design reference bundle into this worktree**

The `design_handoff_homesiem/` folder is reference material excluded from git — it won't exist in a fresh worktree. Copy it from wherever it's already present on disk (e.g. the `siem-api-implementation` worktree, or the main checkout):
```bash
cp -R /Users/hibikipr/Documents/GitHub/homeSIEM/.claude/worktrees/siem-api-implementation/design_handoff_homesiem /Users/hibikipr/Documents/GitHub/homeSIEM/.claude/worktrees/siem-web-console/design_handoff_homesiem
```
(Adjust the source path if that worktree no longer exists — any checkout that has the folder works. It must end up at `siem-web-console/design_handoff_homesiem/`, a sibling of `siem-web/`.)

- [ ] **Step 6: Verify the scaffold builds**

Run: `cd siem-web && pnpm check && pnpm build`
Expected: both succeed with no errors (an empty/default SvelteKit landing page is fine — later tasks replace it).

- [ ] **Step 7: Commit**

```bash
git add siem-web .gitignore
git commit -m "Scaffold siem-web SvelteKit project"
```
(`design_handoff_homesiem/` should already be gitignored at the repo root from the siem-api work — confirm `git status` doesn't show it as untracked-to-be-added; if it does, add `design_handoff_homesiem/` to `.gitignore` before committing.)

---

### Task 2: `src/lib/server/session.ts` — internal JWT + cookie config

**Files:**
- Create: `siem-web/src/lib/server/session.ts`
- Test: `siem-web/src/lib/server/session.test.ts`

**Interfaces:**
- Consumes: `jose` (Task 1's dependency).
- Produces: `SessionClaims`, `SESSION_COOKIE_NAME`, `SESSION_COOKIE_OPTIONS`, `mintSessionToken`, `verifySessionToken` — consumed by Task 6 (`auth/callback`, mints), Task 7 (`hooks.server.ts`, verifies), and Task 5's logout route (reads the cookie name to clear it).

```ts
export interface SessionClaims {
  sub: string;
  userId: number;
  email: string;
  displayName: string;
  groups: string[];
  role: string;
}
export const SESSION_COOKIE_NAME = 'siem_session';
export const SESSION_COOKIE_OPTIONS: { path: string; httpOnly: boolean; secure: boolean; sameSite: 'lax'; maxAge: number };
export function mintSessionToken(claims: SessionClaims, secret: Uint8Array): Promise<string>;
export function verifySessionToken(token: string, secret: Uint8Array): Promise<SessionClaims>;
```

The JWT payload this mints is the exact wire format `siem-api`'s `TokenVerifier` (already built) expects: `sub` as the JWT's standard subject claim, `user_id`/`email`/`display_name`/`groups` as top-level JSON claims, `exp` set 12 hours out. `role` is an EXTRA claim beyond what siem-api's `TokenVerifier` reads — Go's `encoding/json` ignores unrecognized fields on decode, so this is safe to include and doesn't affect siem-api's verification. It exists purely so `hooks.server.ts` (Task 7) can populate `event.locals.user.role` for the nav chrome's "user's name with their role" display (Task 9) without a round-trip to siem-api on every request — siem-api itself never trusts this claim for authorization, it re-resolves role from `groups` on every request as already designed.

- [ ] **Step 1: Write the failing test**

`siem-web/src/lib/server/session.test.ts`:
```ts
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
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd siem-web && pnpm vitest run src/lib/server/session.test.ts`
Expected: FAIL — `session.ts` doesn't exist yet.

- [ ] **Step 3: Write the implementation**

`siem-web/src/lib/server/session.ts`:
```ts
import { SignJWT, jwtVerify } from 'jose';

export interface SessionClaims {
  sub: string;
  userId: number;
  email: string;
  displayName: string;
  groups: string[];
  role: string;
}

export const SESSION_COOKIE_NAME = 'siem_session';

export const SESSION_COOKIE_OPTIONS = {
  path: '/',
  httpOnly: true,
  secure: true,
  sameSite: 'lax' as const,
  maxAge: 60 * 60 * 12
};

export async function mintSessionToken(claims: SessionClaims, secret: Uint8Array): Promise<string> {
  return new SignJWT({
    user_id: claims.userId,
    email: claims.email,
    display_name: claims.displayName,
    groups: claims.groups,
    role: claims.role
  })
    .setProtectedHeader({ alg: 'HS256' })
    .setSubject(claims.sub)
    .setIssuedAt()
    .setExpirationTime('12h')
    .sign(secret);
}

export async function verifySessionToken(token: string, secret: Uint8Array): Promise<SessionClaims> {
  const { payload } = await jwtVerify(token, secret, { algorithms: ['HS256'] });

  if (typeof payload.sub !== 'string') {
    throw new Error('session token missing sub claim');
  }

  return {
    sub: payload.sub,
    userId: payload.user_id as number,
    email: payload.email as string,
    displayName: payload.display_name as string,
    groups: (payload.groups as string[]) ?? [],
    role: payload.role as string
  };
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd siem-web && pnpm vitest run src/lib/server/session.test.ts`
Expected: PASS (all 4 tests).

- [ ] **Step 5: Commit**

```bash
git add siem-web/src/lib/server/session.ts siem-web/src/lib/server/session.test.ts
git commit -m "Add siem-web session: internal JWT mint/verify and cookie config"
```

---

### Task 3: `src/lib/server/siemApiClient.ts` — typed siem-api client

**Files:**
- Create: `siem-web/src/lib/server/siemApiClient.ts`
- Test: `siem-web/src/lib/server/siemApiClient.test.ts`

**Interfaces:**
- Consumes: nothing from earlier tasks (a fetch function is injected for testability).
- Produces: `SiemApiClient`, `SiemApiError`, and the response/request types below — consumed by Task 6 (`auth/callback`, `establishSession`), Task 10 (Wall's `load`, `getEventsStats`/`getAlerts`/`search`), and Task 12 (tail proxy, needs the base URL + auth header pattern, though it streams rather than using this client directly).

```ts
export interface LogEntry { Timestamp: string; Labels: Record<string, string>; Line: string; }
export interface EventsStatsResponse { event_count_24h: number; heat_grid: { source: string; hours: string[] }[]; }
export interface AlertResponse {
  id: number; rule_id: number; group_key: string; severity: string; title: string; body: string;
  event_count: number; state: string; first_seen_at: string; last_seen_at: string;
  acked_by?: number; acked_at?: string;
}
export interface SearchResponse { logql: string; count: number; entries: LogEntry[]; }
export interface EstablishSessionRequest { subject: string; email: string; display_name: string; groups: string[]; }
export interface EstablishSessionResponse { user_id: number; role: string; display_name: string; }

export class SiemApiError extends Error {
  constructor(public status: number, message: string);
}

export class SiemApiClient {
  constructor(config: { baseUrl: string }, fetchFn?: typeof fetch);
  establishSession(req: EstablishSessionRequest): Promise<EstablishSessionResponse>;
  getEventsStats(sessionToken: string): Promise<EventsStatsResponse>;
  getAlerts(sessionToken: string, state?: string): Promise<AlertResponse[]>;
  search(sessionToken: string, params: Record<string, string>): Promise<SearchResponse>;
}
```

`LogEntry`'s `Timestamp`/`Labels`/`Line` field casing intentionally matches siem-api's actual (untagged-struct) wire format — see this plan's Global Constraints note. `establishSession` sends no `Authorization` header (siem-api's `/auth/session` is unauthenticated by design — see the design spec's Auth flow section); every other method requires a `sessionToken` and sends `Authorization: Bearer <token>`.

- [ ] **Step 1: Write the failing test**

`siem-web/src/lib/server/siemApiClient.test.ts`:
```ts
import { describe, it, expect, vi } from 'vitest';
import { SiemApiClient, SiemApiError } from './siemApiClient';

function fakeFetch(body: unknown, status = 200) {
  return vi.fn(async (_url: string, _init?: RequestInit) => {
    return new Response(JSON.stringify(body), { status });
  });
}

describe('SiemApiClient', () => {
  it('getEventsStats attaches Authorization and parses the response', async () => {
    const fetchFn = fakeFetch({ event_count_24h: 1240000, heat_grid: [] });
    const client = new SiemApiClient({ baseUrl: 'http://siem-api:8080' }, fetchFn);

    const result = await client.getEventsStats('token-123');

    expect(result.event_count_24h).toBe(1240000);
    const [url, init] = fetchFn.mock.calls[0];
    expect(url).toBe('http://siem-api:8080/events/stats');
    expect((init?.headers as Record<string, string>).Authorization).toBe('Bearer token-123');
  });

  it('getAlerts appends the state query param when given', async () => {
    const fetchFn = fakeFetch([]);
    const client = new SiemApiClient({ baseUrl: 'http://siem-api:8080' }, fetchFn);

    await client.getAlerts('token-123', 'open');

    const [url] = fetchFn.mock.calls[0];
    expect(url).toBe('http://siem-api:8080/alerts?state=open');
  });

  it('getAlerts omits the query string when no state is given', async () => {
    const fetchFn = fakeFetch([]);
    const client = new SiemApiClient({ baseUrl: 'http://siem-api:8080' }, fetchFn);

    await client.getAlerts('token-123');

    const [url] = fetchFn.mock.calls[0];
    expect(url).toBe('http://siem-api:8080/alerts');
  });

  it('establishSession POSTs JSON with no Authorization header', async () => {
    const fetchFn = fakeFetch({ user_id: 7, role: 'analyst', display_name: 'Alice' });
    const client = new SiemApiClient({ baseUrl: 'http://siem-api:8080' }, fetchFn);

    const result = await client.establishSession({
      subject: 'sub-1',
      email: 'alice@townsville.cc',
      display_name: 'Alice',
      groups: ['siem-analysts']
    });

    expect(result.user_id).toBe(7);
    const [url, init] = fetchFn.mock.calls[0];
    expect(url).toBe('http://siem-api:8080/auth/session');
    expect(init?.method).toBe('POST');
    expect((init?.headers as Record<string, string>).Authorization).toBeUndefined();
    expect(JSON.parse(init?.body as string)).toEqual({
      subject: 'sub-1',
      email: 'alice@townsville.cc',
      display_name: 'Alice',
      groups: ['siem-analysts']
    });
  });

  it('throws SiemApiError with the status code on a non-OK response', async () => {
    const fetchFn = fakeFetch({ error: 'denied' }, 403);
    const client = new SiemApiClient({ baseUrl: 'http://siem-api:8080' }, fetchFn);

    await expect(client.getEventsStats('token-123')).rejects.toMatchObject({
      name: 'SiemApiError',
      status: 403
    });
  });
});
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd siem-web && pnpm vitest run src/lib/server/siemApiClient.test.ts`
Expected: FAIL — `siemApiClient.ts` doesn't exist yet.

- [ ] **Step 3: Write the implementation**

`siem-web/src/lib/server/siemApiClient.ts`:
```ts
export interface LogEntry {
  Timestamp: string;
  Labels: Record<string, string>;
  Line: string;
}

export interface EventsStatsResponse {
  event_count_24h: number;
  heat_grid: { source: string; hours: string[] }[];
}

export interface AlertResponse {
  id: number;
  rule_id: number;
  group_key: string;
  severity: string;
  title: string;
  body: string;
  event_count: number;
  state: string;
  first_seen_at: string;
  last_seen_at: string;
  acked_by?: number;
  acked_at?: string;
}

export interface SearchResponse {
  logql: string;
  count: number;
  entries: LogEntry[];
}

export interface EstablishSessionRequest {
  subject: string;
  email: string;
  display_name: string;
  groups: string[];
}

export interface EstablishSessionResponse {
  user_id: number;
  role: string;
  display_name: string;
}

export class SiemApiError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.name = 'SiemApiError';
    this.status = status;
  }
}

export class SiemApiClient {
  private baseUrl: string;
  private fetchFn: typeof fetch;

  constructor(config: { baseUrl: string }, fetchFn: typeof fetch = fetch) {
    this.baseUrl = config.baseUrl;
    this.fetchFn = fetchFn;
  }

  async establishSession(req: EstablishSessionRequest): Promise<EstablishSessionResponse> {
    return this.request<EstablishSessionResponse>('/auth/session', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(req)
    });
  }

  async getEventsStats(sessionToken: string): Promise<EventsStatsResponse> {
    return this.request<EventsStatsResponse>('/events/stats', this.authInit(sessionToken));
  }

  async getAlerts(sessionToken: string, state?: string): Promise<AlertResponse[]> {
    const path = state ? `/alerts?state=${encodeURIComponent(state)}` : '/alerts';
    return this.request<AlertResponse[]>(path, this.authInit(sessionToken));
  }

  async search(sessionToken: string, params: Record<string, string>): Promise<SearchResponse> {
    const qs = new URLSearchParams(params).toString();
    const path = qs ? `/events/search?${qs}` : '/events/search';
    return this.request<SearchResponse>(path, this.authInit(sessionToken));
  }

  private authInit(sessionToken: string): RequestInit {
    return { headers: { Authorization: `Bearer ${sessionToken}` } };
  }

  private async request<T>(path: string, init: RequestInit): Promise<T> {
    const res = await this.fetchFn(`${this.baseUrl}${path}`, init);
    if (!res.ok) {
      throw new SiemApiError(res.status, await res.text());
    }
    return res.json() as Promise<T>;
  }
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd siem-web && pnpm vitest run src/lib/server/siemApiClient.test.ts`
Expected: PASS (all 5 tests).

- [ ] **Step 5: Commit**

```bash
git add siem-web/src/lib/server/siemApiClient.ts siem-web/src/lib/server/siemApiClient.test.ts
git commit -m "Add siem-web siemApiClient: typed HTTP client for siem-api"
```

---

### Task 4: `src/lib/server/oidc.ts` — OIDC/PKCE wrapper

**Files:**
- Create: `siem-web/src/lib/server/oidc.ts`
- Test: `siem-web/src/lib/server/oidc.test.ts`

**Interfaces:**
- Consumes: `openid-client` (Task 1's dependency) — **verify its actual API against `node_modules/openid-client`'s type definitions before writing this task's `buildLoginRedirect`/`completeLogin`/`getConfig` code; the shape below is this plan's best-effort sketch of the current major version's functional API, not a guarantee.** `extractOidcClaims` has no such uncertainty — it's a pure function with no library dependency.
- Produces: `OidcConfig`, `OidcClaims`, `LoginRedirect`, `PKCE_COOKIE_NAME`, `buildLoginRedirect`, `completeLogin`, `extractOidcClaims` — consumed by Task 5 (`auth/login`, `buildLoginRedirect` + `PKCE_COOKIE_NAME` to set the verifier cookie) and Task 6 (`auth/callback`, `completeLogin` + `PKCE_COOKIE_NAME` to read it back).

```ts
export const PKCE_COOKIE_NAME = 'siem_pkce_verifier';
export interface OidcConfig { issuer: string; clientId: string; redirectUri: string; }
export interface OidcClaims { sub: string; email: string; displayName: string; groups: string[]; }
export interface LoginRedirect { url: string; codeVerifier: string; }
export function buildLoginRedirect(config: OidcConfig): Promise<LoginRedirect>;
export function completeLogin(config: OidcConfig, callbackUrl: URL, codeVerifier: string): Promise<OidcClaims>;
export function extractOidcClaims(raw: Record<string, unknown>): OidcClaims;
```

`PKCE_COOKIE_NAME` lives here (not in `session.ts`) since it's specific to the OIDC handshake, not the app's own session — it's short-lived (10 minutes, set in Task 5) and gone before a real session cookie ever exists.

Only `extractOidcClaims` is TDD'd here — it's the one pure, dependency-free piece: mapping a verified ID token's raw claims object (which may be missing `groups` or `name`) into the shape the rest of the app uses. `buildLoginRedirect`/`completeLogin` are thin wrappers around `openid-client`'s network calls (discovery, authorization URL construction, code exchange) — they're exercised by Task 13's end-to-end test against the real Pocket ID instance, not by a unit test that would otherwise need to mock a third-party SDK's exact call shape.

- [ ] **Step 1: Write the failing test (for `extractOidcClaims` only)**

`siem-web/src/lib/server/oidc.test.ts`:
```ts
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
      groups: ['siem-analysts', 'homelab']
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
});
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd siem-web && pnpm vitest run src/lib/server/oidc.test.ts`
Expected: FAIL — `oidc.ts` doesn't exist yet.

- [ ] **Step 3: Write the implementation**

First, check `node_modules/openid-client`'s type definitions (e.g. `node_modules/openid-client/build/index.d.ts` or wherever its types live) for the actual exported function names and signatures for: discovery/configuration setup, PKCE verifier/challenge generation, authorization URL building, and authorization-code-grant token exchange. Adjust the `import` and the bodies of `buildLoginRedirect`/`completeLogin`/`getConfig` below to match what's actually there — the following is this plan's best-effort sketch, written against the functional API `openid-client` has used in recent major versions:

`siem-web/src/lib/server/oidc.ts`:
```ts
import * as client from 'openid-client';

export interface OidcConfig {
  issuer: string;
  clientId: string;
  redirectUri: string;
}

export interface OidcClaims {
  sub: string;
  email: string;
  displayName: string;
  groups: string[];
}

export interface LoginRedirect {
  url: string;
  codeVerifier: string;
}

export const PKCE_COOKIE_NAME = 'siem_pkce_verifier';

let cachedConfig: client.Configuration | undefined;

async function getConfig(oidcConfig: OidcConfig): Promise<client.Configuration> {
  if (!cachedConfig) {
    cachedConfig = await client.discovery(new URL(oidcConfig.issuer), oidcConfig.clientId);
  }
  return cachedConfig;
}

export async function buildLoginRedirect(oidcConfig: OidcConfig): Promise<LoginRedirect> {
  const config = await getConfig(oidcConfig);
  const codeVerifier = client.randomPKCECodeVerifier();
  const codeChallenge = await client.calculatePKCECodeChallenge(codeVerifier);

  const authUrl = client.buildAuthorizationUrl(config, {
    redirect_uri: oidcConfig.redirectUri,
    scope: 'openid profile email groups',
    code_challenge: codeChallenge,
    code_challenge_method: 'S256'
  });

  return { url: authUrl.href, codeVerifier };
}

export async function completeLogin(
  oidcConfig: OidcConfig,
  callbackUrl: URL,
  codeVerifier: string
): Promise<OidcClaims> {
  const config = await getConfig(oidcConfig);
  const tokens = await client.authorizationCodeGrant(config, callbackUrl, {
    pkceCodeVerifier: codeVerifier
  });
  const idTokenClaims = tokens.claims();
  if (!idTokenClaims) {
    throw new Error('no ID token claims returned from token endpoint');
  }
  return extractOidcClaims(idTokenClaims as Record<string, unknown>);
}

export function extractOidcClaims(raw: Record<string, unknown>): OidcClaims {
  if (typeof raw.sub !== 'string') {
    throw new Error('ID token missing sub claim');
  }
  const groups = Array.isArray(raw.groups)
    ? raw.groups.filter((g): g is string => typeof g === 'string')
    : [];
  const email = typeof raw.email === 'string' ? raw.email : '';
  const displayName = typeof raw.name === 'string' ? raw.name : email || raw.sub;

  return { sub: raw.sub, email, displayName, groups };
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd siem-web && pnpm vitest run src/lib/server/oidc.test.ts`
Expected: PASS (all 5 `extractOidcClaims` tests — `buildLoginRedirect`/`completeLogin` have no unit test in this task, per the design's testing split).

- [ ] **Step 5: Verify the whole file still compiles**

Run: `cd siem-web && pnpm check`
Expected: no type errors — this is what actually validates `buildLoginRedirect`/`completeLogin` against `openid-client`'s real types, in lieu of a unit test.

- [ ] **Step 6: Commit**

```bash
git add siem-web/src/lib/server/oidc.ts siem-web/src/lib/server/oidc.test.ts
git commit -m "Add siem-web oidc: PKCE login/callback wrapper and claims extraction"
```

---

### Task 5: `auth/login` and `auth/logout` routes

**Files:**
- Create: `siem-web/src/routes/auth/login/+server.ts`
- Create: `siem-web/src/routes/auth/logout/+server.ts`
- Test: `siem-web/src/routes/auth/login/server.test.ts`
- Test: `siem-web/src/routes/auth/logout/server.test.ts`

**Interfaces:**
- Consumes: `buildLoginRedirect`, `PKCE_COOKIE_NAME` (Task 4), `SESSION_COOKIE_NAME` (Task 2).
- Produces: the `GET` handlers themselves, exercised end-to-end by Task 13's Playwright test; no other task imports from these route files.

SvelteKit route handlers are plain async functions taking a `RequestEvent`-shaped object — test them by constructing a minimal fake event (just the fields the handler touches) rather than spinning up a real server. `redirect(status, location)` from `@sveltejs/kit` throws rather than returning; catch it with `.rejects.toMatchObject(...)`.

- [ ] **Step 1: Write the failing tests**

`siem-web/src/routes/auth/login/server.test.ts`:
```ts
import { describe, it, expect, vi } from 'vitest';
import { GET } from './+server';
import * as oidc from '$lib/server/oidc';

vi.mock('$env/dynamic/private', () => ({
  env: {
    OIDC_ISSUER: 'https://pocketid.townsville.cc',
    OIDC_CLIENT_ID: 'homeSIEM',
    APP_URL: 'https://siem.townsville.cc'
  }
}));

vi.mock('$lib/server/oidc', async (importOriginal) => {
  const actual = await importOriginal<typeof import('$lib/server/oidc')>();
  return { ...actual, buildLoginRedirect: vi.fn() };
});

describe('GET /auth/login', () => {
  it('redirects to the OIDC authorization URL and sets the PKCE cookie', async () => {
    vi.mocked(oidc.buildLoginRedirect).mockResolvedValue({
      url: 'https://pocketid.townsville.cc/authorize?state=abc',
      codeVerifier: 'verifier-abc'
    });
    const setCookie = vi.fn();

    await expect(GET({ cookies: { set: setCookie } } as never)).rejects.toMatchObject({
      status: 302,
      location: 'https://pocketid.townsville.cc/authorize?state=abc'
    });

    expect(setCookie).toHaveBeenCalledWith(
      oidc.PKCE_COOKIE_NAME,
      'verifier-abc',
      expect.objectContaining({ httpOnly: true, maxAge: 600 })
    );
  });
});
```

`siem-web/src/routes/auth/logout/server.test.ts`:
```ts
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
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd siem-web && pnpm vitest run src/routes/auth/login/server.test.ts src/routes/auth/logout/server.test.ts`
Expected: FAIL — neither `+server.ts` exists yet.

- [ ] **Step 3: Write the implementations**

`siem-web/src/routes/auth/login/+server.ts`:
```ts
import { redirect } from '@sveltejs/kit';
import { env } from '$env/dynamic/private';
import type { RequestHandler } from './$types';
import { buildLoginRedirect, PKCE_COOKIE_NAME } from '$lib/server/oidc';

export const GET: RequestHandler = async ({ cookies }) => {
  const { url, codeVerifier } = await buildLoginRedirect({
    issuer: env.OIDC_ISSUER,
    clientId: env.OIDC_CLIENT_ID,
    redirectUri: `${env.APP_URL}/auth/callback`
  });

  cookies.set(PKCE_COOKIE_NAME, codeVerifier, {
    path: '/',
    httpOnly: true,
    secure: true,
    sameSite: 'lax',
    maxAge: 600
  });

  redirect(302, url);
};
```

`siem-web/src/routes/auth/logout/+server.ts`:
```ts
import { redirect } from '@sveltejs/kit';
import { env } from '$env/dynamic/private';
import type { RequestHandler } from './$types';
import { SESSION_COOKIE_NAME } from '$lib/server/session';

export const GET: RequestHandler = async ({ cookies }) => {
  cookies.delete(SESSION_COOKIE_NAME, { path: '/' });
  redirect(302, env.OIDC_LOGOUT_URL);
};
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd siem-web && pnpm vitest run src/routes/auth/login/server.test.ts src/routes/auth/logout/server.test.ts`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add siem-web/src/routes/auth/login siem-web/src/routes/auth/logout
git commit -m "Add siem-web auth/login and auth/logout routes"
```

---

### Task 6: `auth/callback` route — the crux of the login flow

**Files:**
- Create: `siem-web/src/routes/auth/callback/+server.ts`
- Test: `siem-web/src/routes/auth/callback/server.test.ts`

**Interfaces:**
- Consumes: `completeLogin`, `PKCE_COOKIE_NAME` (Task 4); `mintSessionToken`, `SESSION_COOKIE_NAME`, `SESSION_COOKIE_OPTIONS` (Task 2); `SiemApiClient`, `SiemApiError` (Task 3).
- Produces: the `GET` handler, exercised end-to-end by Task 13.

Orchestrates steps 2-5 of the design's Auth flow section: read the PKCE cookie → exchange the code (`completeLogin`) → call siem-api's `/auth/session` → mint the session cookie → redirect to `/`.

- [ ] **Step 1: Write the failing test**

`siem-web/src/routes/auth/callback/server.test.ts`:
```ts
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { GET } from './+server';
import * as oidc from '$lib/server/oidc';
import * as siemApiClientModule from '$lib/server/siemApiClient';

vi.mock('$env/dynamic/private', () => ({
  env: {
    OIDC_ISSUER: 'https://pocketid.townsville.cc',
    OIDC_CLIENT_ID: 'homeSIEM',
    APP_URL: 'https://siem.townsville.cc',
    API_URL: 'http://siem-api:8080',
    SESSION_SECRET: Buffer.from('0123456789abcdef0123456789abcdef').toString('base64')
  }
}));

vi.mock('$lib/server/oidc', async (importOriginal) => {
  const actual = await importOriginal<typeof import('$lib/server/oidc')>();
  return { ...actual, completeLogin: vi.fn() };
});

vi.mock('$lib/server/siemApiClient', async (importOriginal) => {
  const actual = await importOriginal<typeof import('$lib/server/siemApiClient')>();
  return { ...actual, SiemApiClient: vi.fn() };
});

function fakeEvent(pkceCookie: string | undefined) {
  const cookieStore = new Map<string, string>();
  if (pkceCookie) cookieStore.set(oidc.PKCE_COOKIE_NAME, pkceCookie);
  return {
    url: new URL('https://siem.townsville.cc/auth/callback?code=abc&state=xyz'),
    cookies: {
      get: (name: string) => cookieStore.get(name),
      delete: vi.fn((name: string) => cookieStore.delete(name)),
      set: vi.fn((name: string, value: string) => cookieStore.set(name, value))
    }
  };
}

describe('GET /auth/callback', () => {
  beforeEach(() => {
    vi.mocked(oidc.completeLogin).mockResolvedValue({
      sub: 'oidc-sub-1',
      email: 'alice@townsville.cc',
      displayName: 'Alice',
      groups: ['siem-analysts']
    });
    vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(
      () =>
        ({
          establishSession: vi.fn().mockResolvedValue({ user_id: 7, role: 'analyst', display_name: 'Alice' })
        }) as never
    );
  });

  it('errors with 400 when the PKCE cookie is missing', async () => {
    const event = fakeEvent(undefined);
    await expect(GET(event as never)).rejects.toMatchObject({ status: 400 });
  });

  it('sets the session cookie and redirects to / on success', async () => {
    const event = fakeEvent('verifier-abc');

    await expect(GET(event as never)).rejects.toMatchObject({ status: 302, location: '/' });

    expect(event.cookies.set).toHaveBeenCalledWith(
      'siem_session',
      expect.any(String),
      expect.objectContaining({ httpOnly: true })
    );
    expect(event.cookies.delete).toHaveBeenCalledWith(oidc.PKCE_COOKIE_NAME, { path: '/' });
  });

  it('propagates an error when siem-api denies session establishment', async () => {
    vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(
      () =>
        ({
          establishSession: vi.fn().mockRejectedValue(new siemApiClientModule.SiemApiError(403, 'denied'))
        }) as never
    );
    const event = fakeEvent('verifier-abc');

    await expect(GET(event as never)).rejects.toMatchObject({ status: 403 });
  });
});
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd siem-web && pnpm vitest run src/routes/auth/callback/server.test.ts`
Expected: FAIL — `+server.ts` doesn't exist yet.

- [ ] **Step 3: Write the implementation**

`siem-web/src/routes/auth/callback/+server.ts`:
```ts
import { redirect, error } from '@sveltejs/kit';
import { env } from '$env/dynamic/private';
import type { RequestHandler } from './$types';
import { completeLogin, PKCE_COOKIE_NAME } from '$lib/server/oidc';
import { mintSessionToken, SESSION_COOKIE_NAME, SESSION_COOKIE_OPTIONS } from '$lib/server/session';
import { SiemApiClient, SiemApiError } from '$lib/server/siemApiClient';

export const GET: RequestHandler = async ({ url, cookies }) => {
  const codeVerifier = cookies.get(PKCE_COOKIE_NAME);
  if (!codeVerifier) {
    error(400, 'missing PKCE verifier cookie');
  }
  cookies.delete(PKCE_COOKIE_NAME, { path: '/' });

  const claims = await completeLogin(
    {
      issuer: env.OIDC_ISSUER,
      clientId: env.OIDC_CLIENT_ID,
      redirectUri: `${env.APP_URL}/auth/callback`
    },
    url,
    codeVerifier
  );

  const apiClient = new SiemApiClient({ baseUrl: env.API_URL });
  let session;
  try {
    session = await apiClient.establishSession({
      subject: claims.sub,
      email: claims.email,
      display_name: claims.displayName,
      groups: claims.groups
    });
  } catch (err) {
    if (err instanceof SiemApiError) {
      error(err.status, err.message);
    }
    throw err;
  }

  const secret = Buffer.from(env.SESSION_SECRET, 'base64');
  const token = await mintSessionToken(
    {
      sub: claims.sub,
      userId: session.user_id,
      email: claims.email,
      displayName: session.display_name,
      groups: claims.groups,
      role: session.role
    },
    secret
  );

  cookies.set(SESSION_COOKIE_NAME, token, SESSION_COOKIE_OPTIONS);
  redirect(302, '/');
};
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd siem-web && pnpm vitest run src/routes/auth/callback/server.test.ts`
Expected: PASS (all 3 tests).

- [ ] **Step 5: Commit**

```bash
git add siem-web/src/routes/auth/callback
git commit -m "Add siem-web auth/callback: OIDC code exchange, siem-api session establishment, cookie mint"
```

---

### Task 7: `src/hooks.server.ts` — the auth gate

**Files:**
- Create: `siem-web/src/hooks.server.ts`
- Create: `siem-web/src/app.d.ts` modification (add `Locals.user` typing)
- Test: `siem-web/src/hooks.server.test.ts`

**Interfaces:**
- Consumes: `verifySessionToken`, `SESSION_COOKIE_NAME` (Task 2).
- Produces: the `handle` hook, and `event.locals.user: { userId, email, displayName, groups, role } | undefined` — consumed by Task 8 (`+layout.server.ts`, reads `locals.user` for the nav chrome) and Task 10 (Wall's `load`, reads `locals.sessionToken` for calling siem-api — see note below).

**Important:** `event.locals.user` carries the decoded claims for display purposes, but the raw session token string itself (needed for `Authorization: Bearer <token>` calls to siem-api) must ALSO be available to `load` functions — store it as `event.locals.sessionToken: string` alongside `user`, so Task 10 doesn't need to re-read the cookie itself.

```ts
// src/app.d.ts additions
declare global {
  namespace App {
    interface Locals {
      user?: { userId: number; email: string; displayName: string; groups: string[]; role: string };
      sessionToken?: string;
    }
  }
}
```

- [ ] **Step 1: Write the failing test**

`siem-web/src/hooks.server.test.ts`:
```ts
import { describe, it, expect, vi } from 'vitest';
import { handle } from './hooks.server';
import { mintSessionToken, SESSION_COOKIE_NAME } from '$lib/server/session';

vi.mock('$env/dynamic/private', () => ({
  env: { SESSION_SECRET: Buffer.from('0123456789abcdef0123456789abcdef').toString('base64') }
}));

const secret = new TextEncoder().encode('0123456789abcdef0123456789abcdef');

function fakeEvent(pathname: string, cookieValue: string | undefined) {
  const locals: Record<string, unknown> = {};
  return {
    event: {
      url: new URL(`https://siem.townsville.cc${pathname}`),
      cookies: {
        get: () => cookieValue,
        delete: vi.fn()
      },
      locals
    },
    locals
  };
}

describe('handle', () => {
  it('passes through /auth/login without requiring a session', async () => {
    const { event } = fakeEvent('/auth/login', undefined);
    const resolve = vi.fn().mockResolvedValue(new Response('ok'));

    await handle({ event: event as never, resolve });

    expect(resolve).toHaveBeenCalled();
  });

  it('redirects to /auth/login when no session cookie is present', async () => {
    const { event } = fakeEvent('/', undefined);
    const resolve = vi.fn();

    await expect(handle({ event: event as never, resolve })).rejects.toMatchObject({
      status: 302,
      location: '/auth/login'
    });
    expect(resolve).not.toHaveBeenCalled();
  });

  it('redirects to /auth/login and clears the cookie when the session token is invalid', async () => {
    const { event } = fakeEvent('/', 'not-a-valid-jwt');
    const resolve = vi.fn();

    await expect(handle({ event: event as never, resolve })).rejects.toMatchObject({ status: 302 });
    expect(event.cookies.delete).toHaveBeenCalledWith(SESSION_COOKIE_NAME, { path: '/' });
  });

  it('attaches locals.user and locals.sessionToken and resolves when the session is valid', async () => {
    const token = await mintSessionToken(
      {
        sub: 'oidc-sub-1',
        userId: 42,
        email: 'alice@townsville.cc',
        displayName: 'Alice',
        groups: ['siem-analysts'],
        role: 'analyst'
      },
      secret
    );
    const { event, locals } = fakeEvent('/', token);
    const resolve = vi.fn().mockResolvedValue(new Response('ok'));

    await handle({ event: event as never, resolve });

    expect(resolve).toHaveBeenCalled();
    expect(locals.user).toMatchObject({ userId: 42, displayName: 'Alice', role: 'analyst' });
    expect(locals.sessionToken).toBe(token);
  });
});
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd siem-web && pnpm vitest run src/hooks.server.test.ts`
Expected: FAIL — `hooks.server.ts` doesn't exist yet.

- [ ] **Step 3: Write the implementation**

`siem-web/src/hooks.server.ts`:
```ts
import { redirect, type Handle } from '@sveltejs/kit';
import { env } from '$env/dynamic/private';
import { verifySessionToken, SESSION_COOKIE_NAME } from '$lib/server/session';

const PUBLIC_PREFIXES = ['/auth/login', '/auth/callback', '/auth/logout'];

export const handle: Handle = async ({ event, resolve }) => {
  if (PUBLIC_PREFIXES.some((prefix) => event.url.pathname.startsWith(prefix))) {
    return resolve(event);
  }

  const token = event.cookies.get(SESSION_COOKIE_NAME);
  if (!token) {
    redirect(302, '/auth/login');
  }

  try {
    const secret = Buffer.from(env.SESSION_SECRET, 'base64');
    const claims = await verifySessionToken(token, secret);
    event.locals.user = {
      userId: claims.userId,
      email: claims.email,
      displayName: claims.displayName,
      groups: claims.groups,
      role: claims.role
    };
    event.locals.sessionToken = token;
  } catch {
    event.cookies.delete(SESSION_COOKIE_NAME, { path: '/' });
    redirect(302, '/auth/login');
  }

  return resolve(event);
};
```

Add the `Locals` typing from the Interfaces section above to `siem-web/src/app.d.ts` (the scaffold already generates this file with an empty `App` namespace — extend it, don't replace the whole file).

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd siem-web && pnpm vitest run src/hooks.server.test.ts`
Expected: PASS (all 4 tests).

- [ ] **Step 5: Run `pnpm check`**

Run: `cd siem-web && pnpm check`
Expected: no type errors — confirms the `app.d.ts` typing matches how `hooks.server.ts` actually assigns `locals.user`/`locals.sessionToken`.

- [ ] **Step 6: Commit**

```bash
git add siem-web/src/hooks.server.ts siem-web/src/hooks.server.test.ts siem-web/src/app.d.ts
git commit -m "Add siem-web hooks.server: session verification and auth gate"
```

---

### Task 8: Design tokens + global chrome (nav, layout)

**Files:**
- Modify: `siem-web/src/lib/styles/tokens.css` (populate — was created empty in Task 1)
- Create: `siem-web/src/lib/components/Nav.svelte`
- Create: `siem-web/src/routes/+layout.svelte`
- Create: `siem-web/src/routes/+layout.server.ts`

**Interfaces:**
- Consumes: `event.locals.user` (Task 7).
- Produces: `tokens.css`'s custom properties (referenced by every later component task) and the app's persistent chrome, into which Task 11's `+page.svelte` renders.

This task is pure transcription + presentational markup — no TDD, per the Global Constraints testing split. Verification is `pnpm check` (build/type correctness) now, and a visual dev-server comparison against `design_handoff_homesiem/designs/homeSIEM Console - Build.dc.html` once Task 12 gives the layout something to render around.

- [ ] **Step 1: Add the Phosphor icon package**

```bash
cd siem-web && pnpm add @phosphor-icons/web
```

- [ ] **Step 2: Write `tokens.css`**

Transcribe every value from `design_handoff_homesiem/README.md`'s "Design tokens" section exactly — do not round, approximate, or substitute a "close enough" value.

`siem-web/src/lib/styles/tokens.css`:
```css
:root {
  /* Color */
  --color-bg: #131523;
  --color-bg-alt: #161826;
  --color-surface: #232532;
  --color-surface-2: #1b1d2c;
  --color-surface-3: #191b29;
  --color-text: #e9e9ed;
  --color-text-2: #cfd3e5;
  --color-text-3: #b2b6ca;
  --color-muted: #9397ab;
  --color-muted-2: #75798c;
  --color-line: #292b31;
  --color-line-2: #3f424d;
  --color-accent: #968ae0;
  --color-accent-light: #b5abfc;
  --color-accent-lighter: #d2cefd;
  --color-accent-deep: #5d5294;
  --color-accent-tint: #2b2741;
  --color-accent-tint-2: #423a6a;

  /* Severity (OKLCH, low-chroma) */
  --color-severity-critical: oklch(0.68 0.16 22);
  --color-severity-error: oklch(0.68 0.16 22);
  --color-severity-warning: oklch(0.78 0.13 78);
  --color-severity-ok: oklch(0.74 0.11 158);
  --color-severity-healthy: oklch(0.74 0.11 158);
  --color-severity-notice: #796cbf;
  --color-severity-info: #5d5294;

  /* Type */
  --font-ui: Inter, sans-serif;
  --font-mono: ui-monospace, 'SF Mono', Menlo, monospace;
  --text-page-title: 19px;
  --text-big-stat: 46px;
  --text-section-head: 13px;
  --text-body: 13px;
  --text-table: 12.5px;
  --text-log-row: 11.5px;
  --text-label: 11px;
  --text-eyebrow: 10px;
  --line-height-log: 2;
  --line-height-dense-table: 1.55;

  /* Spacing (0.7x scale) */
  --space-1: 2.8px;
  --space-2: 5.6px;
  --space-3: 8.4px;
  --space-4: 11.2px;
  --space-5: 16.8px;
  --space-6: 22.4px;

  /* Radius */
  --radius-sm: 4px;
  --radius-default: 8px;
  --radius-lg: 14px;

  /* Elevation */
  --shadow-flat: 0 0 0 1px var(--color-line-2);
  --shadow-raised: 0 0 0 1px #595d6c, 0 6px 18px rgba(0, 0, 0, 0.55);

  /* Interaction */
  --row-hover-bg: rgba(255, 255, 255, 0.035);
  --row-selected-bg: var(--color-accent-tint);
  --focus-outline: 2px solid var(--color-accent);
  --focus-outline-offset: 2px;
}

body {
  background: var(--color-bg);
  color: var(--color-text);
  font-family: var(--font-ui);
  font-weight: 400;
  margin: 0;
}

*:focus-visible {
  outline: var(--focus-outline);
  outline-offset: var(--focus-outline-offset);
}
```

- [ ] **Step 3: Write `Nav.svelte`**

`siem-web/src/lib/components/Nav.svelte`:
```svelte
<script lang="ts">
  let {
    activeRoute,
    alertCount,
    ingestRate,
    userDisplayName,
    userRole
  }: {
    activeRoute: string;
    alertCount: number;
    ingestRate: number;
    userDisplayName: string;
    userRole: string;
  } = $props();

  const navItems = [
    { label: 'Wall', href: '/' },
    { label: 'Search', href: '/search' },
    { label: 'Live tail', href: '/tail' },
    { label: 'Alerts', href: '/alerts' },
    { label: 'Sources', href: '/sources' },
    { label: 'Settings', href: '/settings' }
  ];
</script>

<header class="nav">
  <div class="brand">
    <span class="brand-icon"><i class="ph ph-shield-check"></i></span>
    <span class="brand-name">homeSIEM</span>
  </div>

  <nav class="links">
    {#each navItems as item (item.href)}
      <a href={item.href} class:active={activeRoute === item.href}>
        {item.label}
        {#if item.label === 'Alerts' && alertCount > 0}
          <span class="pill">{alertCount}</span>
        {/if}
      </a>
    {/each}
  </nav>

  <div class="status">
    <span class="ingest-dot"></span>
    <span class="ingest-text">ingest live · {ingestRate}/min</span>
    <span class="user">
      {userDisplayName}
      <span class="role">{userRole}</span>
    </span>
    <span class="avatar"></span>
  </div>
</header>

<style>
  .nav {
    display: flex;
    align-items: center;
    padding: var(--space-5) var(--space-6);
    gap: var(--space-6);
    background: linear-gradient(
        to right,
        transparent,
        var(--color-line) 48px,
        var(--color-line) calc(100% - 48px),
        transparent
      )
      bottom / 100% 1px no-repeat;
  }

  .brand {
    display: flex;
    align-items: center;
    gap: var(--space-3);
  }

  .brand-icon {
    width: 24px;
    height: 24px;
    border-radius: var(--radius-sm) calc(var(--radius-sm) + 3px);
    background: var(--color-accent-tint);
    color: var(--color-accent-light);
    display: flex;
    align-items: center;
    justify-content: center;
  }

  .brand-name {
    font-size: 20px;
    font-weight: 500;
    letter-spacing: -0.02em;
  }

  .links {
    display: flex;
    gap: var(--space-5);
    font-size: var(--text-table);
  }

  .links a {
    color: var(--color-muted);
    text-decoration: none;
    padding-bottom: var(--space-2);
    border-bottom: 2px solid transparent;
  }

  .links a.active {
    color: var(--color-text);
    border-bottom-color: var(--color-accent);
  }

  .pill {
    font-size: var(--text-eyebrow);
    background: var(--color-accent-light);
    color: var(--color-bg-alt);
    border-radius: var(--radius-sm);
    padding: 0 var(--space-2);
    margin-left: var(--space-1);
  }

  .status {
    margin-left: auto;
    display: flex;
    align-items: center;
    gap: var(--space-4);
    font-size: var(--text-table);
    color: var(--color-muted-2);
  }

  .ingest-dot {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: var(--color-severity-healthy);
    display: inline-block;
  }

  .user .role {
    color: var(--color-muted-2);
    margin-left: var(--space-2);
  }

  .avatar {
    width: 28px;
    height: 28px;
    border-radius: 50%;
    background: var(--color-line-2);
    display: inline-block;
  }
</style>
```

- [ ] **Step 4: Write `+layout.server.ts` and `+layout.svelte`**

`siem-web/src/routes/+layout.server.ts`:
```ts
import type { LayoutServerLoad } from './$types';

export const load: LayoutServerLoad = ({ locals, url }) => {
  return {
    user: locals.user,
    activeRoute: url.pathname
  };
};
```

`siem-web/src/routes/+layout.svelte`:
```svelte
<script lang="ts">
  import '@phosphor-icons/web/regular';
  import '$lib/styles/tokens.css';
  import Nav from '$lib/components/Nav.svelte';
  import type { LayoutData } from './$types';

  let { data, children }: { data: LayoutData; children: () => unknown } = $props();
</script>

<Nav
  activeRoute={data.activeRoute}
  alertCount={0}
  ingestRate={0}
  userDisplayName={data.user?.displayName ?? ''}
  userRole={data.user?.role ?? ''}
/>

<main>
  {@render children()}
</main>
```

`alertCount`/`ingestRate` are hardcoded to `0` here — Task 10 wires the Wall screen's own data, but the nav's live counts belong to a shared layout data source that doesn't exist yet in this sub-project (no `/alerts` or ingest-rate polling at the layout level). Flagging as a known simplification for this pass, not a bug: the design's nav chrome numbers will read `0`/quiet until a later sub-project wires a proper shared source (e.g. a layout-level SSE subscription).

- [ ] **Step 5: Verify it builds**

Run: `cd siem-web && pnpm check && pnpm build`
Expected: both succeed.

- [ ] **Step 6: Commit**

```bash
git add siem-web/src/lib/styles/tokens.css siem-web/src/lib/components/Nav.svelte siem-web/src/routes/+layout.svelte siem-web/src/routes/+layout.server.ts siem-web/package.json siem-web/pnpm-lock.yaml
git commit -m "Add siem-web design tokens and global nav chrome"
```

---

### Task 9: `src/lib/wall.ts` — Wall screen data-shaping helpers

**Files:**
- Create: `siem-web/src/lib/wall.ts`
- Test: `siem-web/src/lib/wall.test.ts`

**Interfaces:**
- Consumes: `AlertResponse`, `LogEntry` types (Task 3).
- Produces: `heatTierColor`, `topTriageAlerts`, `deriveCountryBreakdown`, `CountryCount` — consumed by Task 10 (Wall's `load`, for triage/country data) and Task 11 (`HeatGrid.svelte`, for tier→color).

```ts
export function heatTierColor(tier: string): string;
export function topTriageAlerts(alerts: AlertResponse[], count?: number): AlertResponse[];
export interface CountryCount { country: string; count: number; }
export function deriveCountryBreakdown(entries: LogEntry[]): CountryCount[];
```

`heatTierColor` maps the six tier strings `/events/stats` returns (`critical`/`warning`/`busy`/`light`/`quiet`/`none`) to the exact token values from the design's heat-grid legend — `var(--color-severity-critical)`, `var(--color-severity-warning)`, `var(--color-accent-deep)` (`#5d5294` busy), `var(--color-accent-tint-2)` (`#423a6a` light), `var(--color-accent-tint)` (`#2b2741` quiet), `var(--color-surface)` (`#232532` no data). `topTriageAlerts` sorts by severity rank (`critical > high > medium > low`, matching the schema's severity vocabulary) then by recency, returning the top N (default 3, per the design's three-card triage lane). `deriveCountryBreakdown` is the best-effort country panel described in the design spec — parses each entry's `Line` as JSON, reads `geoip.cc` when present, counts occurrences, sorts descending; malformed lines or entries with no geoip data are silently skipped, not errors.

- [ ] **Step 1: Write the failing test**

`siem-web/src/lib/wall.test.ts`:
```ts
import { describe, it, expect } from 'vitest';
import { heatTierColor, topTriageAlerts, deriveCountryBreakdown } from './wall';
import type { AlertResponse, LogEntry } from './server/siemApiClient';

describe('heatTierColor', () => {
  it('maps every known tier to its token', () => {
    expect(heatTierColor('critical')).toBe('var(--color-severity-critical)');
    expect(heatTierColor('warning')).toBe('var(--color-severity-warning)');
    expect(heatTierColor('busy')).toBe('var(--color-accent-deep)');
    expect(heatTierColor('light')).toBe('var(--color-accent-tint-2)');
    expect(heatTierColor('quiet')).toBe('var(--color-accent-tint)');
    expect(heatTierColor('none')).toBe('var(--color-surface)');
  });

  it('falls back to the "none" color for an unrecognized tier', () => {
    expect(heatTierColor('bogus')).toBe('var(--color-surface)');
  });
});

function alert(overrides: Partial<AlertResponse>): AlertResponse {
  return {
    id: 1,
    rule_id: 1,
    group_key: 'a',
    severity: 'low',
    title: 't',
    body: 'b',
    event_count: 1,
    state: 'open',
    first_seen_at: '2026-08-02T00:00:00Z',
    last_seen_at: '2026-08-02T00:00:00Z',
    ...overrides
  };
}

describe('topTriageAlerts', () => {
  it('sorts by severity rank descending, then recency descending', () => {
    const alerts = [
      alert({ id: 1, severity: 'low', last_seen_at: '2026-08-02T03:00:00Z' }),
      alert({ id: 2, severity: 'critical', last_seen_at: '2026-08-02T01:00:00Z' }),
      alert({ id: 3, severity: 'critical', last_seen_at: '2026-08-02T02:00:00Z' }),
      alert({ id: 4, severity: 'medium', last_seen_at: '2026-08-02T04:00:00Z' })
    ];

    const top = topTriageAlerts(alerts, 3);

    expect(top.map((a) => a.id)).toEqual([3, 2, 4]);
  });

  it('defaults to the top 3', () => {
    const alerts = [1, 2, 3, 4, 5].map((id) => alert({ id, severity: 'critical' }));
    expect(topTriageAlerts(alerts)).toHaveLength(3);
  });
});

describe('deriveCountryBreakdown', () => {
  function entry(line: string): LogEntry {
    return { Timestamp: '2026-08-02T00:00:00Z', Labels: {}, Line: line };
  }

  it('counts geoip.cc occurrences and sorts descending', () => {
    const entries = [
      entry('{"geoip":{"cc":"US"}}'),
      entry('{"geoip":{"cc":"US"}}'),
      entry('{"geoip":{"cc":"DE"}}')
    ];

    expect(deriveCountryBreakdown(entries)).toEqual([
      { country: 'US', count: 2 },
      { country: 'DE', count: 1 }
    ]);
  });

  it('skips entries with no geoip data or malformed JSON', () => {
    const entries = [entry('not json'), entry('{}'), entry('{"geoip":{}}'), entry('{"geoip":{"cc":"US"}}')];
    expect(deriveCountryBreakdown(entries)).toEqual([{ country: 'US', count: 1 }]);
  });
});
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd siem-web && pnpm vitest run src/lib/wall.test.ts`
Expected: FAIL — `wall.ts` doesn't exist yet.

- [ ] **Step 3: Write the implementation**

`siem-web/src/lib/wall.ts`:
```ts
import type { AlertResponse, LogEntry } from './server/siemApiClient';

const HEAT_TIER_COLORS: Record<string, string> = {
  critical: 'var(--color-severity-critical)',
  warning: 'var(--color-severity-warning)',
  busy: 'var(--color-accent-deep)',
  light: 'var(--color-accent-tint-2)',
  quiet: 'var(--color-accent-tint)',
  none: 'var(--color-surface)'
};

export function heatTierColor(tier: string): string {
  return HEAT_TIER_COLORS[tier] ?? HEAT_TIER_COLORS.none;
}

const SEVERITY_RANK: Record<string, number> = { critical: 4, high: 3, medium: 2, low: 1 };

export function topTriageAlerts(alerts: AlertResponse[], count = 3): AlertResponse[] {
  return [...alerts]
    .sort((a, b) => {
      const rankDiff = (SEVERITY_RANK[b.severity] ?? 0) - (SEVERITY_RANK[a.severity] ?? 0);
      if (rankDiff !== 0) return rankDiff;
      return new Date(b.last_seen_at).getTime() - new Date(a.last_seen_at).getTime();
    })
    .slice(0, count);
}

export interface CountryCount {
  country: string;
  count: number;
}

export function deriveCountryBreakdown(entries: LogEntry[]): CountryCount[] {
  const counts = new Map<string, number>();

  for (const entry of entries) {
    let parsed: unknown;
    try {
      parsed = JSON.parse(entry.Line);
    } catch {
      continue;
    }
    if (typeof parsed !== 'object' || parsed === null) continue;

    const geoip = (parsed as Record<string, unknown>).geoip;
    if (typeof geoip !== 'object' || geoip === null) continue;

    const country = (geoip as Record<string, unknown>).cc;
    if (typeof country !== 'string' || country === '') continue;

    counts.set(country, (counts.get(country) ?? 0) + 1);
  }

  return [...counts.entries()]
    .map(([country, count]) => ({ country, count }))
    .sort((a, b) => b.count - a.count);
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd siem-web && pnpm vitest run src/lib/wall.test.ts`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add siem-web/src/lib/wall.ts siem-web/src/lib/wall.test.ts
git commit -m "Add siem-web wall: heat-grid coloring, triage sort, country breakdown helpers"
```

---

### Task 10: Wall screen `+page.server.ts` — data load

**Files:**
- Create: `siem-web/src/routes/+page.server.ts`
- Test: `siem-web/src/routes/page.server.test.ts`

**Interfaces:**
- Consumes: `SiemApiClient` (Task 3), `locals.sessionToken` (Task 7), `topTriageAlerts`/`deriveCountryBreakdown` (Task 9).
- Produces: the `load` function's return shape — consumed by Task 11 (`+page.svelte`).

```ts
interface WallPageData {
  eventCount24h: number;
  heatGrid: { source: string; hours: string[] }[];
  openAlertCount: number;
  triageAlerts: AlertResponse[];
  countryBreakdown: CountryCount[];
}
```

- [ ] **Step 1: Write the failing test**

`siem-web/src/routes/page.server.test.ts`:
```ts
import { describe, it, expect, vi } from 'vitest';
import { load } from './+page.server';
import * as siemApiClientModule from '$lib/server/siemApiClient';

vi.mock('$env/dynamic/private', () => ({ env: { API_URL: 'http://siem-api:8080' } }));

vi.mock('$lib/server/siemApiClient', async (importOriginal) => {
  const actual = await importOriginal<typeof import('$lib/server/siemApiClient')>();
  return { ...actual, SiemApiClient: vi.fn() };
});

describe('Wall load', () => {
  it('shapes siem-api responses into WallPageData', async () => {
    vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(
      () =>
        ({
          getEventsStats: vi.fn().mockResolvedValue({
            event_count_24h: 1240000,
            heat_grid: [{ source: 'udm-ultra', hours: ['critical', 'none'] }]
          }),
          getAlerts: vi.fn().mockResolvedValue([
            {
              id: 1,
              rule_id: 1,
              group_key: 'a',
              severity: 'critical',
              title: 't',
              body: 'b',
              event_count: 1,
              state: 'open',
              first_seen_at: '2026-08-02T00:00:00Z',
              last_seen_at: '2026-08-02T00:00:00Z'
            }
          ]),
          search: vi.fn().mockResolvedValue({
            logql: '{job="siem"}',
            count: 1,
            entries: [{ Timestamp: '2026-08-02T00:00:00Z', Labels: {}, Line: '{"geoip":{"cc":"US"}}' }]
          })
        }) as never
    );

    const result = await load({ locals: { sessionToken: 'token-123' } } as never);

    expect(result.eventCount24h).toBe(1240000);
    expect(result.heatGrid).toEqual([{ source: 'udm-ultra', hours: ['critical', 'none'] }]);
    expect(result.openAlertCount).toBe(1);
    expect(result.triageAlerts).toHaveLength(1);
    expect(result.countryBreakdown).toEqual([{ country: 'US', count: 1 }]);
  });
});
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd siem-web && pnpm vitest run src/routes/page.server.test.ts`
Expected: FAIL — `+page.server.ts` doesn't exist yet.

- [ ] **Step 3: Write the implementation**

`siem-web/src/routes/+page.server.ts`:
```ts
import { env } from '$env/dynamic/private';
import type { PageServerLoad } from './$types';
import { SiemApiClient } from '$lib/server/siemApiClient';
import { topTriageAlerts, deriveCountryBreakdown } from '$lib/wall';

export const load: PageServerLoad = async ({ locals }) => {
  const client = new SiemApiClient({ baseUrl: env.API_URL });
  const token = locals.sessionToken as string;

  const [stats, openAlerts, sample] = await Promise.all([
    client.getEventsStats(token),
    client.getAlerts(token, 'open'),
    client.search(token, {})
  ]);

  return {
    eventCount24h: stats.event_count_24h,
    heatGrid: stats.heat_grid,
    openAlertCount: openAlerts.length,
    triageAlerts: topTriageAlerts(openAlerts),
    countryBreakdown: deriveCountryBreakdown(sample.entries)
  };
};
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd siem-web && pnpm vitest run src/routes/page.server.test.ts`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add siem-web/src/routes/+page.server.ts siem-web/src/routes/page.server.test.ts
git commit -m "Add siem-web Wall screen data load"
```

---

### Task 11: Wall screen components + `+page.svelte`

**Files:**
- Create: `siem-web/src/lib/components/StatRow.svelte`
- Create: `siem-web/src/lib/components/HeatGrid.svelte`
- Create: `siem-web/src/lib/components/TriageCard.svelte`
- Create: `siem-web/src/lib/components/CountryBar.svelte`
- Create: `siem-web/src/routes/+page.svelte`

**Interfaces:**
- Consumes: `heatTierColor` (Task 9), Task 10's `load` return shape.
- Produces: the rendered Wall screen.

Presentational, no unit tests, per the Global Constraints testing split. `StatRow` renders only the two stats this sub-project actually has data for (24h event count, open alert count) — the design's "Blocked at WAN" and "Retention" figures need data sources not built in this pass (no WAN-block metric exists yet; retention is static/placeholder per the design spec) and are represented as static placeholder markup with a code comment, not invented numbers.

- [ ] **Step 1: Write `StatRow.svelte`**

```svelte
<script lang="ts">
  let { eventCount24h, openAlertCount }: { eventCount24h: number; openAlertCount: number } = $props();

  function formatCount(n: number): { value: string; unit: string } {
    if (n >= 1_000_000) return { value: (n / 1_000_000).toFixed(2), unit: 'M' };
    if (n >= 1_000) return { value: (n / 1_000).toFixed(1), unit: 'K' };
    return { value: String(n), unit: '' };
  }

  let events = $derived(formatCount(eventCount24h));
</script>

<div class="stat-row">
  <div class="stat">
    <div class="eyebrow">Events 24h</div>
    <div class="value">{events.value}<span class="unit">{events.unit}</span></div>
  </div>
  <div class="stat">
    <div class="eyebrow">Open alerts</div>
    <div class="value critical">{openAlertCount}</div>
  </div>
  <div class="stat placeholder">
    <!-- No data source for retention figures in this sub-project yet — see design spec. -->
    <div class="eyebrow">Retention</div>
    <div class="value-small">not yet available</div>
  </div>
</div>

<style>
  .stat-row {
    display: flex;
    gap: var(--space-6);
    flex-wrap: wrap;
    padding: var(--space-5) var(--space-6);
  }
  .eyebrow {
    font-size: var(--text-eyebrow);
    text-transform: uppercase;
    letter-spacing: 0.1em;
    color: var(--color-muted-2);
  }
  .value {
    font-size: var(--text-big-stat);
    font-weight: 500;
    letter-spacing: -0.03em;
  }
  .value.critical {
    color: var(--color-severity-critical);
  }
  .unit {
    font-size: 22px;
    color: var(--color-muted);
  }
  .placeholder {
    margin-left: auto;
    text-align: right;
  }
  .value-small {
    font-size: var(--text-table);
    color: var(--color-muted-2);
  }
</style>
```

- [ ] **Step 2: Write `HeatGrid.svelte`**

```svelte
<script lang="ts">
  import { heatTierColor } from '$lib/wall';

  let { rows }: { rows: { source: string; hours: string[] }[] } = $props();
</script>

<div class="heat-grid">
  {#each rows as row (row.source)}
    <div class="row">
      <span class="label">{row.source}</span>
      <div class="cells">
        {#each row.hours as tier, i (i)}
          <span class="cell" style="background: {heatTierColor(tier)}" title={tier}></span>
        {/each}
      </div>
    </div>
  {/each}
</div>

<style>
  .heat-grid {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }
  .row {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }
  .label {
    width: 96px;
    font-family: var(--font-mono);
    font-size: var(--text-label);
    color: var(--color-muted);
    flex-shrink: 0;
  }
  .cells {
    display: flex;
    gap: 3px;
    flex: 1;
  }
  .cell {
    flex: 1;
    height: 19px;
    border-radius: 3px;
  }
</style>
```

- [ ] **Step 3: Write `TriageCard.svelte`**

```svelte
<script lang="ts">
  import type { AlertResponse } from '$lib/server/siemApiClient';

  let { alert }: { alert: AlertResponse } = $props();

  function ageLabel(iso: string): string {
    const ms = Date.now() - new Date(iso).getTime();
    const minutes = Math.floor(ms / 60000);
    if (minutes < 60) return `${minutes}m`;
    return `${Math.floor(minutes / 60)}h`;
  }
</script>

<div class="card severity-{alert.severity}">
  <div class="header">
    <span class="eyebrow">{alert.severity}</span>
    <span class="age">{ageLabel(alert.first_seen_at)}</span>
  </div>
  <div class="title">{alert.title}</div>
  <div class="body">{alert.body}</div>
  <div class="actions">
    <button class="primary">Investigate</button>
    <button class="ghost">Mute 1h</button>
  </div>
</div>

<style>
  .card {
    background: var(--color-surface-2);
    border-radius: var(--radius-default);
    padding: var(--space-4);
    box-shadow: inset 0 2px 0 var(--color-severity-critical);
  }
  .card.severity-warning {
    box-shadow: inset 0 2px 0 var(--color-severity-warning);
  }
  .card.severity-low,
  .card.severity-medium {
    box-shadow: inset 0 2px 0 var(--color-severity-info);
  }
  .header {
    display: flex;
    justify-content: space-between;
    font-size: var(--text-eyebrow);
    text-transform: uppercase;
    color: var(--color-severity-critical);
  }
  .age {
    color: var(--color-muted);
    text-transform: none;
  }
  .title {
    font-size: 14px;
    font-weight: 500;
    margin-top: var(--space-2);
  }
  .body {
    font-size: 11.5px;
    color: var(--color-muted);
    margin-top: var(--space-1);
  }
  .actions {
    margin-top: var(--space-3);
    display: flex;
    gap: var(--space-3);
  }
  .primary {
    background: transparent;
    border: 1px solid var(--color-accent);
    color: var(--color-text);
    border-radius: var(--radius-sm);
    padding: var(--space-1) var(--space-3);
    font-size: 11px;
  }
  .ghost {
    background: none;
    border: none;
    color: var(--color-accent-light);
    font-size: 11px;
  }
</style>
```

- [ ] **Step 4: Write `CountryBar.svelte`**

```svelte
<script lang="ts">
  import type { CountryCount } from '$lib/wall';

  let { countries }: { countries: CountryCount[] } = $props();
  let max = $derived(Math.max(1, ...countries.map((c) => c.count)));
</script>

<div class="country-bar">
  <div class="eyebrow">Where it's coming from</div>
  {#each countries as c (c.country)}
    <div class="row">
      <span class="name">{c.country}</span>
      <div class="track">
        <div class="fill" style="width: {(c.count / max) * 100}%"></div>
      </div>
      <span class="count">{c.count}</span>
    </div>
  {/each}
</div>

<style>
  .country-bar {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }
  .eyebrow {
    font-size: var(--text-eyebrow);
    text-transform: uppercase;
    color: var(--color-muted-2);
    margin-bottom: var(--space-2);
  }
  .row {
    display: flex;
    align-items: center;
    gap: var(--space-3);
  }
  .name {
    width: 112px;
    font-size: var(--text-table);
  }
  .track {
    flex: 1;
    height: 6px;
    background: var(--color-surface);
    border-radius: var(--radius-sm);
    overflow: hidden;
  }
  .fill {
    height: 100%;
    background: var(--color-accent);
  }
  .count {
    font-family: var(--font-mono);
    font-size: var(--text-label);
    color: var(--color-muted);
  }
</style>
```

- [ ] **Step 5: Write `+page.svelte`**

```svelte
<script lang="ts">
  import StatRow from '$lib/components/StatRow.svelte';
  import HeatGrid from '$lib/components/HeatGrid.svelte';
  import TriageCard from '$lib/components/TriageCard.svelte';
  import CountryBar from '$lib/components/CountryBar.svelte';
  import type { PageData } from './$types';

  let { data }: { data: PageData } = $props();
</script>

<div class="wall">
  <div class="col-main">
    <StatRow eventCount24h={data.eventCount24h} openAlertCount={data.openAlertCount} />
    <HeatGrid rows={data.heatGrid} />
    <div class="triage-lane">
      {#each data.triageAlerts as alert (alert.id)}
        <TriageCard {alert} />
      {/each}
    </div>
  </div>
  <div class="col-side">
    <CountryBar countries={data.countryBreakdown} />
  </div>
</div>

<style>
  .wall {
    display: grid;
    grid-template-columns: 1.62fr 1fr;
    gap: var(--space-6);
    padding: var(--space-5) var(--space-6);
  }
  .col-main,
  .col-side {
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: var(--space-5);
  }
  .triage-lane {
    display: grid;
    grid-template-columns: repeat(3, 1fr);
    gap: var(--space-4);
    align-items: start;
  }
</style>
```

- [ ] **Step 6: Verify it builds, then run the dev server for a visual check**

Run: `cd siem-web && pnpm check && pnpm build`
Expected: both succeed.

Then run: `pnpm dev`, log in via the real flow (Task 13 automates part of this later, but a manual pass now is worthwhile), and compare the rendered Wall screen against `design_handoff_homesiem/designs/homeSIEM Console - Build.dc.html`'s Wall screen. Note any visible discrepancies in your report — fixing them is in-scope for this task if small (spacing, color), but don't gold-plate beyond the handoff's spec.

- [ ] **Step 7: Commit**

```bash
git add siem-web/src/lib/components/StatRow.svelte siem-web/src/lib/components/HeatGrid.svelte siem-web/src/lib/components/TriageCard.svelte siem-web/src/lib/components/CountryBar.svelte siem-web/src/routes/+page.svelte
git commit -m "Add siem-web Wall screen components and page"
```

---

### Task 12: SSE tail proxy + `Ticker.svelte`

**Files:**
- Create: `siem-web/src/routes/api/tail-proxy/+server.ts`
- Test: `siem-web/src/routes/api/tail-proxy/server.test.ts`
- Create: `siem-web/src/lib/components/Ticker.svelte`
- Modify: `siem-web/src/routes/+page.svelte` (add `Ticker` to the side column)

**Interfaces:**
- Consumes: `locals.sessionToken` (Task 7).
- Produces: a same-origin SSE endpoint the browser's `EventSource` can hit without ever holding the siem-api bearer token — per the design spec's SSE proxying section.

The proxy route's auth-forwarding is TDD'd (it's real logic: attach the right header, forward the right URL, preserve streaming semantics). `Ticker.svelte`'s `EventSource` subscription logic is not unit-tested, per the design's testing split for presentational components — it's exercised visually and, indirectly, by Task 13's e2e test reaching a logged-in page where it mounts without erroring.

- [ ] **Step 1: Write the failing test for the proxy route**

`siem-web/src/routes/api/tail-proxy/server.test.ts`:
```ts
import { describe, it, expect, vi } from 'vitest';
import { GET } from './+server';

vi.mock('$env/dynamic/private', () => ({ env: { API_URL: 'http://siem-api:8080' } }));

describe('GET /api/tail-proxy', () => {
  it('forwards the Authorization header to siem-api and streams the response', async () => {
    const fetchFn = vi.fn().mockResolvedValue(new Response(new ReadableStream(), { status: 200 }));

    const response = await GET({ locals: { sessionToken: 'token-123' }, fetch: fetchFn } as never);

    expect(fetchFn).toHaveBeenCalledWith(
      'http://siem-api:8080/events/tail',
      expect.objectContaining({ headers: { Authorization: 'Bearer token-123' } })
    );
    expect(response.headers.get('Content-Type')).toBe('text/event-stream');
    expect(response.status).toBe(200);
  });
});
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd siem-web && pnpm vitest run src/routes/api/tail-proxy/server.test.ts`
Expected: FAIL — `+server.ts` doesn't exist yet.

- [ ] **Step 3: Write the proxy route**

`siem-web/src/routes/api/tail-proxy/+server.ts`:
```ts
import { env } from '$env/dynamic/private';
import type { RequestHandler } from './$types';

export const GET: RequestHandler = async ({ locals, fetch }) => {
  const token = locals.sessionToken as string;

  const upstream = await fetch(`${env.API_URL}/events/tail`, {
    headers: { Authorization: `Bearer ${token}` }
  });

  return new Response(upstream.body, {
    status: upstream.status,
    headers: {
      'Content-Type': 'text/event-stream',
      'Cache-Control': 'no-cache',
      Connection: 'keep-alive'
    }
  });
};
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd siem-web && pnpm vitest run src/routes/api/tail-proxy/server.test.ts`
Expected: PASS.

- [ ] **Step 5: Write `Ticker.svelte`**

`siem-web/src/lib/components/Ticker.svelte`:
```svelte
<script lang="ts">
  interface TickerEntry {
    time: string;
    severity: string;
    host: string;
    program: string;
    message: string;
  }

  let entries = $state<TickerEntry[]>([]);

  $effect(() => {
    const source = new EventSource('/api/tail-proxy');
    source.onmessage = (event) => {
      try {
        const raw = JSON.parse(event.data);
        entries = [
          {
            time: raw.Timestamp ?? '',
            severity: raw.Labels?.severity ?? 'info',
            host: raw.Labels?.host ?? '',
            program: raw.Labels?.program ?? '',
            message: raw.Line ?? ''
          },
          ...entries
        ].slice(0, 50);
      } catch {
        // malformed SSE payload — skip this line rather than breaking the ticker
      }
    };
    return () => source.close();
  });
</script>

<div class="ticker">
  <div class="eyebrow">Ticker</div>
  {#each entries as entry, i (i)}
    <div class="row">
      <span class="time">{entry.time}</span>
      <span class="dot severity-{entry.severity}"></span>
      <span class="line">{entry.host} {entry.program}: {entry.message}</span>
    </div>
  {/each}
</div>

<style>
  .ticker {
    background: var(--color-bg-alt);
    box-shadow: inset var(--shadow-flat);
    border-radius: var(--radius-default);
    font-family: var(--font-mono);
    font-size: 11px;
    line-height: 1.9;
    padding: var(--space-3);
    max-height: 400px;
    overflow: hidden;
  }
  .eyebrow {
    font-size: var(--text-eyebrow);
    text-transform: uppercase;
    color: var(--color-muted-2);
    margin-bottom: var(--space-2);
  }
  .row {
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    display: flex;
    gap: var(--space-2);
    align-items: center;
  }
  .time {
    color: var(--color-muted);
  }
  .dot {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: var(--color-severity-info);
    flex-shrink: 0;
  }
  .dot.severity-critical {
    background: var(--color-severity-critical);
  }
  .dot.severity-warning {
    background: var(--color-severity-warning);
  }
</style>
```

- [ ] **Step 6: Wire `Ticker` into the Wall page's side column**

In `siem-web/src/routes/+page.svelte`, add the import and mount it below `CountryBar` in `.col-side`:
```svelte
  import Ticker from '$lib/components/Ticker.svelte';
```
```svelte
  <div class="col-side">
    <CountryBar countries={data.countryBreakdown} />
    <Ticker />
  </div>
```

- [ ] **Step 7: Verify it builds**

Run: `cd siem-web && pnpm check && pnpm build`
Expected: both succeed.

- [ ] **Step 8: Commit**

```bash
git add siem-web/src/routes/api/tail-proxy siem-web/src/lib/components/Ticker.svelte siem-web/src/routes/+page.svelte
git commit -m "Add siem-web SSE tail proxy and Ticker component"
```

---

### Task 13: Playwright e2e — full login flow

**Files:**
- Create: `siem-web/e2e/login.spec.ts` (adjust the path to match wherever Task 1's scaffold put Playwright's test directory — `e2e/` or `tests/`, check `playwright.config.ts`)

**Interfaces:**
- Consumes: the whole app, running against the real Pocket ID instance and a locally-run real `siem-api` binary, per the design spec's dev/test environment decision.

**This task has a real unknown this plan cannot resolve in advance:** Pocket ID authenticates via passkey (per the design handoff), and WebAuthn/passkey flows are not scriptable the way a plain username/password form is — there's no way to know the exact interaction (which UI elements, whether a hardware key or platform authenticator is expected) without having the real instance in front of you. Two ways to handle it, in order of preference:

1. **If Pocket ID's admin console can register a WebAuthn credential backed by Playwright/Chrome DevTools Protocol's virtual authenticator** (`page.context().newCDPSession()` → `WebAuthn.enable` → `WebAuthn.addVirtualAuthenticator`), use that — it lets the whole flow run headlessly and repeatably. This is the standard way to e2e-test WebAuthn login flows.
2. **If that's not feasible** (e.g. Pocket ID requires a specific authenticator type the virtual one can't emulate, or there's no test account set up for it), write the test up through the redirect to Pocket ID's `/authorize` page and assert that redirect happens correctly, then stop — leave a clear comment explaining that full login automation needs a WebAuthn test credential set up first, and treat manual login (already done once in Task 11 Step 6) as the verification for this pass. Report this limitation clearly rather than faking a passing assertion past a step that wasn't actually exercised.

Do not invent believable-looking Pocket ID login steps without having verified them against the real instance — a test that "passes" by clicking through UI elements that don't match reality is worse than an honest partial test.

- [ ] **Step 1: Write the redirect-to-Pocket-ID portion (this part has no uncertainty)**

`siem-web/e2e/login.spec.ts`:
```ts
import { test, expect } from '@playwright/test';

test('unauthenticated visitors are redirected to Pocket ID', async ({ page }) => {
  await page.goto('/');
  await page.waitForURL(/pocketid\.townsville\.cc\/authorize/);
});
```

- [ ] **Step 2: Run it against the locally-running app**

With `siem-api` running locally (per the design's dev-environment decision) and `pnpm dev` running for `siem-web`, run: `cd siem-web && pnpm exec playwright test e2e/login.spec.ts`
Expected: PASS — confirms the auth gate (Task 7) and login redirect (Task 5) work end-to-end against the real Pocket ID discovery document.

- [ ] **Step 3: Attempt the full login flow per the guidance above**

Investigate Pocket ID's actual login UI (navigate to `https://pocketid.townsville.cc/authorize?...` manually in a browser first, or check its documentation/admin console for WebAuthn test-credential support) and extend the test to complete login and assert on the post-login Wall screen, OR stop at Step 1's redirect assertion with a clear comment per the guidance above — whichever is actually achievable. Report which path you took and why.

- [ ] **Step 4: Commit**

```bash
git add siem-web/e2e/login.spec.ts
git commit -m "Add siem-web e2e: login flow"
```

---

### Task 14: Final verification and dev-run documentation

**Files:**
- Create: `siem-web/README.md`

**Interfaces:**
- Consumes: everything from Tasks 1-13.

This is the final checkpoint for this sub-project — no new application code, just running the whole thing together and documenting how to run it.

- [ ] **Step 1: Run the full test suite**

Run: `cd siem-web && pnpm check && pnpm test:unit run && pnpm build`
(`test:unit` or just `test` depending on what Task 1's scaffold named the Vitest script — check `package.json`.)
Expected: all clean, every test from Tasks 2-12 passing together.

- [ ] **Step 2: Write `siem-web/README.md`**

```markdown
# siem-web

The homeSIEM console: OIDC login through Pocket ID, session/BFF layer, and
(so far) the Wall screen. See `docs/superpowers/specs/2026-08-02-siem-web-auth-shell-wall-design.md`
for the design.

## Running locally

1. Copy `.env.example` to `.env` and fill in the values — `SESSION_SECRET`
   must match whatever `siem-api` is using for `SIEM_SESSION_SECRET`.
2. Run `siem-api` locally (see the `siem-api-implementation` worktree's
   own README/smoke-test instructions) so `API_URL` has something to talk to.
3. `pnpm install`
4. `pnpm dev`

## Testing

- `pnpm test:unit` — Vitest, TDD coverage for session/cookie handling, the
  siem-api client, claims extraction, the auth gate, and Wall's data-shaping
  helpers.
- `pnpm exec playwright test` — the login-flow e2e test (see its own file
  for what is/isn't automated, depending on Pocket ID's WebAuthn testability).

## What's built so far

OIDC login, session cookie, global nav chrome, Screen 1 (Wall). The other
five screens (Search, Live tail, Alerts, Sources, Settings) are separate
future sub-projects.

## Known gaps in this pass

- Nav chrome's alert count and ingest-rate figures are hardcoded to 0 — no
  shared layout-level data source for them yet.
- Wall's country breakdown is a best-effort client-side derivation from a
  bounded `/events/search` call, not a real aggregation endpoint.
- Wall's retention figures have no data source at all yet.
- Break-glass local admin login isn't wired up (belongs with the Settings
  screen sub-project).
```

- [ ] **Step 3: Manual smoke test against the real stack**

With `siem-api` running locally and `.env` filled in with real values:
```bash
pnpm dev
```
Visit the app in a browser, confirm the redirect to Pocket ID happens, log in, confirm landing on the Wall screen with the nav chrome, stat row, heat grid, triage cards (if any open alerts exist), country panel, and ticker all rendering without console errors. Compare visually against `design_handoff_homesiem/designs/homeSIEM Console - Build.dc.html`'s Wall screen one more time now that everything is wired together, not just Task 11's component-level check.

- [ ] **Step 4: Commit**

```bash
git add siem-web/README.md
git commit -m "Add siem-web README with dev-run instructions and known gaps"
```
