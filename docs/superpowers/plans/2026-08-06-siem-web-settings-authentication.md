# siem-web: Settings → Authentication Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the static-mockup `/settings` page's Authentication section with a real
Group → role mapping panel backed by the existing `GET`/`PUT /settings/auth` endpoints —
per `docs/superpowers/specs/2026-08-06-siem-web-settings-authentication-design.md`.

**Architecture:** Zero siem-api changes — `GET /settings/auth` and `PUT /settings/auth`
already exist, already tested, and already do everything this pass needs (`PUT` upserts
each mapping in its request body by `group_claim`, it does not replace the full list —
this is load-bearing for Task 5/6, see the note there). This is a pure siem-web addition:
new client methods, a new proxy route, a new load function, two new components, and a
rewrite of the existing page to use them instead of hardcoded mock data.

**Tech Stack:** SvelteKit 5 + TypeScript + Vitest, matching every prior siem-web
sub-project. No new dependencies.

## Global Constraints

- Response/request field JSON names are snake_case (`group_claim`, `oidc_issuer`,
  `role_mappings`, etc.), matching `authSettingsResponse`/`updateAuthSettingsRequest` in
  `siem-api/internal/api/settings_auth.go` exactly.
- **`PUT /settings/auth` upserts, it does not replace.** `handleUpdateAuthSettings` loops
  over the request's `role_mappings` and calls `store.UpsertRoleMapping` (upsert-by-
  `group_claim`) for each — it never deletes a mapping absent from the request. The
  add/edit form must send **only the single mapping being added or edited**, not the
  full existing list — sending the full list would still be correct but is unnecessary
  complexity; sending only the changed mapping is both simpler and matches what the
  endpoint is actually designed for.
- Editing an existing mapping must keep `group_claim` **read-only** in the form — since
  the backend upserts by `group_claim`, an edited `group_claim` would create a new
  mapping rather than rename the existing one.
- New mappings get priority `max(existing priorities) + 1` (or `1` if none exist) —
  not an editable field.
- No delete UI, no OIDC provider panel, no Session & break-glass panel — all three are
  explicitly out of scope per the design spec's Decisions section.
- No unit tests for Svelte components — matching every other screen's convention.

---

### Task 1: siem-web — `settings.ts` role→capability lookup

**Files:**
- Create: `siem-web/src/lib/settings.ts`
- Create: `siem-web/src/lib/settings.test.ts`

**Interfaces:**
- Produces: `roleCapabilityLabel(role: string): string` — consumed by Task 5's
  `RoleMappingTable.svelte`.

- [ ] **Step 1: Write the failing tests**

Create `siem-web/src/lib/settings.test.ts`:

```ts
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
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd siem-web && npm run test:unit -- --run settings.test`
Expected: FAIL — `Cannot find module './settings'`.

- [ ] **Step 3: Implement the helper**

Create `siem-web/src/lib/settings.ts`:

```ts
const ROLE_CAPABILITY_LABELS: Record<string, string> = {
	admin: 'read/write/manage',
	analyst: 'read/search/triage',
	viewer: 'read only'
};

export function roleCapabilityLabel(role: string): string {
	return ROLE_CAPABILITY_LABELS[role] ?? 'unknown';
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd siem-web && npm run test:unit -- --run settings.test`
Expected: PASS (2 tests).

- [ ] **Step 5: Commit**

```bash
git add siem-web/src/lib/settings.ts siem-web/src/lib/settings.test.ts
git commit -m "Add settings.ts: role-to-capability-label lookup"
```

---

### Task 2: siem-web — `SiemApiClient` additions

**Files:**
- Modify: `siem-web/src/lib/server/siemApiClient.ts`
- Modify: `siem-web/src/lib/server/siemApiClient.test.ts`

**Interfaces:**
- Produces: `RoleMappingResponse`, `AuthSettingsResponse`, `UpdateRoleMappingsRequest`
  types; `SiemApiClient.getAuthSettings`, `updateRoleMappings` methods — consumed by
  Task 4's load function and Task 6's form component.

- [ ] **Step 1: Write the failing tests**

Add to `siem-web/src/lib/server/siemApiClient.test.ts`:

```ts
it('getAuthSettings attaches Authorization and parses the response', async () => {
	const fetchFn = fakeFetch({
		oidc_issuer: 'https://pocketid.townsville.cc',
		oidc_client_id: 'homeSIEM',
		oidc_groups_scope: 'groups',
		role_mappings: [{ id: 1, group_claim: 'admins', role: 'admin', priority: 10 }]
	});
	const client = new SiemApiClient({ baseUrl: 'http://siem-api:8080' }, fetchFn);

	const result = await client.getAuthSettings('token-123');

	expect(result.role_mappings).toHaveLength(1);
	expect(result.role_mappings[0].group_claim).toBe('admins');
	const [url, init] = fetchFn.mock.calls[0];
	expect(url).toBe('http://siem-api:8080/settings/auth');
	expect((init?.headers as Record<string, string>).Authorization).toBe('Bearer token-123');
});

it('updateRoleMappings PUTs to /settings/auth with Authorization and a JSON body', async () => {
	const fetchFn = fakeFetch(null, 204);
	const client = new SiemApiClient({ baseUrl: 'http://siem-api:8080' }, fetchFn);

	await client.updateRoleMappings('token-123', {
		role_mappings: [{ group_claim: 'homelab', role: 'viewer', priority: 100 }]
	});

	const [url, init] = fetchFn.mock.calls[0];
	expect(url).toBe('http://siem-api:8080/settings/auth');
	expect(init?.method).toBe('PUT');
	expect((init?.headers as Record<string, string>).Authorization).toBe('Bearer token-123');
	expect(JSON.parse(init?.body as string)).toEqual({
		role_mappings: [{ group_claim: 'homelab', role: 'viewer', priority: 100 }]
	});
});
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd siem-web && npm run test:unit -- --run siemApiClient`
Expected: FAIL — `client.getAuthSettings is not a function` (and similarly for
`updateRoleMappings`).

- [ ] **Step 3: Implement the client additions**

In `siem-web/src/lib/server/siemApiClient.ts`, add near the other response interfaces:

```ts
export interface RoleMappingResponse {
	id: number;
	group_claim: string;
	role: string;
	priority: number;
}

export interface AuthSettingsResponse {
	oidc_issuer: string;
	oidc_client_id: string;
	oidc_groups_scope: string;
	role_mappings: RoleMappingResponse[];
}

export interface UpdateRoleMappingsRequest {
	role_mappings: { group_claim: string; role: string; priority: number }[];
}
```

Add to the `SiemApiClient` class (after `getRules`):

```ts
	async getAuthSettings(sessionToken: string): Promise<AuthSettingsResponse> {
		return this.request<AuthSettingsResponse>('/settings/auth', this.authInit(sessionToken));
	}

	async updateRoleMappings(sessionToken: string, req: UpdateRoleMappingsRequest): Promise<void> {
		return this.requestNoContent('/settings/auth', {
			method: 'PUT',
			headers: { 'Content-Type': 'application/json', ...this.authInit(sessionToken).headers },
			body: JSON.stringify(req)
		});
	}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd siem-web && npm run test:unit -- --run siemApiClient`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add siem-web/src/lib/server/siemApiClient.ts siem-web/src/lib/server/siemApiClient.test.ts
git commit -m "Add getAuthSettings and updateRoleMappings to SiemApiClient"
```

---

### Task 3: siem-web — `/api/settings/auth` proxy route

**Files:**
- Create: `siem-web/src/routes/api/settings/auth/+server.ts`
- Create: `siem-web/src/routes/api/settings/auth/server.test.ts`

**Interfaces:**
- Consumes: `Task 2`'s `SiemApiClient.updateRoleMappings`.
- Produces: `PUT /api/settings/auth`, consumed by `Task 6`'s `RoleMappingForm.svelte`.

- [ ] **Step 1: Write the failing tests**

Create `siem-web/src/routes/api/settings/auth/server.test.ts` (mirrors the Search
plan's `routes/api/search/rules/server.test.ts` exactly, swapping `createRule`/POST for
`updateRoleMappings`/PUT):

```ts
import { describe, it, expect, vi } from 'vitest';
import { PUT } from './+server';
import * as siemApiClientModule from '$lib/server/siemApiClient';

vi.mock('$env/dynamic/private', () => ({ env: { API_URL: 'http://siem-api:8080' } }));

vi.mock('$lib/server/siemApiClient', async (importOriginal) => {
	const actual = await importOriginal<typeof import('$lib/server/siemApiClient')>();
	return { ...actual, SiemApiClient: vi.fn() };
});

function fakeUpdateRequest() {
	return { role_mappings: [{ group_claim: 'homelab', role: 'viewer', priority: 100 }] };
}

describe('PUT /api/settings/auth', () => {
	it('calls updateRoleMappings with the session token and returns 204', async () => {
		const updateRoleMappingsMock = vi.fn().mockResolvedValue(undefined);
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return { updateRoleMappings: updateRoleMappingsMock };
		});

		const response = await PUT({
			request: new Request('http://x/api/settings/auth', {
				method: 'PUT',
				body: JSON.stringify(fakeUpdateRequest())
			}),
			locals: { sessionToken: 'token-123' }
		} as never);

		expect(updateRoleMappingsMock).toHaveBeenCalledWith('token-123', fakeUpdateRequest());
		expect(response.status).toBe(204);
	});

	it('propagates a SiemApiError status code as a JSON error response', async () => {
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return {
				updateRoleMappings: vi
					.fn()
					.mockRejectedValue(new siemApiClientModule.SiemApiError(403, 'denied'))
			};
		});

		const response = await PUT({
			request: new Request('http://x/api/settings/auth', {
				method: 'PUT',
				body: JSON.stringify(fakeUpdateRequest())
			}),
			locals: { sessionToken: 'token-123' }
		} as never);

		expect(response.status).toBe(403);
	});
});
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd siem-web && npm run test:unit -- --run routes/api/settings`
Expected: FAIL — `Cannot find module './+server'`.

- [ ] **Step 3: Implement the route**

Create `siem-web/src/routes/api/settings/auth/+server.ts`:

```ts
import { json } from '@sveltejs/kit';
import { env } from '$env/dynamic/private';
import type { RequestHandler } from './$types';
import { SiemApiClient, SiemApiError } from '$lib/server/siemApiClient';
import type { UpdateRoleMappingsRequest } from '$lib/server/siemApiClient';

export const PUT: RequestHandler = async ({ request, locals }) => {
	const client = new SiemApiClient({ baseUrl: env.API_URL as string });
	const token = locals.sessionToken as string;
	const body = (await request.json()) as UpdateRoleMappingsRequest;

	try {
		await client.updateRoleMappings(token, body);
	} catch (err) {
		if (err instanceof SiemApiError) {
			return json({ error: err.message }, { status: err.status });
		}
		throw err;
	}

	return new Response(null, { status: 204 });
};
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd siem-web && npm run test:unit -- --run routes/api/settings`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add siem-web/src/routes/api/settings
git commit -m "Add PUT /api/settings/auth proxy route"
```

---

### Task 4: siem-web — `/settings` load function

**Files:**
- Create: `siem-web/src/routes/settings/+page.server.ts`
- Create: `siem-web/src/routes/settings/page.server.test.ts`

**Interfaces:**
- Consumes: `Task 2`'s `SiemApiClient.getAuthSettings`.
- Produces: the `PageData` shape `Task 6`'s `+page.svelte` renders:
  `{ roleMappings: RoleMappingResponse[] }`.

- [ ] **Step 1: Write the failing tests**

Create `siem-web/src/routes/settings/page.server.test.ts`:

```ts
import { describe, it, expect, vi } from 'vitest';
import { load } from './+page.server';
import * as siemApiClientModule from '$lib/server/siemApiClient';
import { SiemApiError } from '$lib/server/siemApiClient';

vi.mock('$env/dynamic/private', () => ({ env: { API_URL: 'http://siem-api:8080' } }));

vi.mock('$lib/server/siemApiClient', async (importOriginal) => {
	const actual = await importOriginal<typeof import('$lib/server/siemApiClient')>();
	return { ...actual, SiemApiClient: vi.fn() };
});

describe('Settings load', () => {
	it('returns the real role mappings from siem-api', async () => {
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return {
				getAuthSettings: vi.fn().mockResolvedValue({
					oidc_issuer: 'https://pocketid.townsville.cc',
					oidc_client_id: 'homeSIEM',
					oidc_groups_scope: 'groups',
					role_mappings: [{ id: 1, group_claim: 'admins', role: 'admin', priority: 10 }]
				})
			};
		});

		const result = (await load({
			locals: { sessionToken: 'token-123' }
		} as never)) as Exclude<Awaited<ReturnType<typeof load>>, void>;

		expect(result.roleMappings).toEqual([
			{ id: 1, group_claim: 'admins', role: 'admin', priority: 10 }
		]);
	});

	it('redirects to /auth/logout on a 401/403 from siem-api', async () => {
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return {
				getAuthSettings: vi.fn().mockRejectedValue(new SiemApiError(401, 'invalid session'))
			};
		});

		await expect(
			load({ locals: { sessionToken: 'stale-token' } } as never)
		).rejects.toMatchObject({ status: 302, location: '/auth/logout' });
	});

	it('surfaces a 502 when siem-api fails for a reason other than auth', async () => {
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return {
				getAuthSettings: vi.fn().mockRejectedValue(new SiemApiError(500, 'boom'))
			};
		});

		await expect(
			load({ locals: { sessionToken: 'token-123' } } as never)
		).rejects.toMatchObject({ status: 502 });
	});
});
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd siem-web && npm run test:unit -- --run routes/settings`
Expected: FAIL — `Cannot find module './+page.server'`.

- [ ] **Step 3: Implement the load function**

Create `siem-web/src/routes/settings/+page.server.ts`:

```ts
import { error, redirect } from '@sveltejs/kit';
import { env } from '$env/dynamic/private';
import type { PageServerLoad } from './$types';
import { SiemApiClient, SiemApiError } from '$lib/server/siemApiClient';

export const load: PageServerLoad = async ({ locals }) => {
	const client = new SiemApiClient({ baseUrl: env.API_URL as string });
	const token = locals.sessionToken as string;

	let settings;
	try {
		settings = await client.getAuthSettings(token);
	} catch (err) {
		if (err instanceof SiemApiError) {
			if (err.status === 401 || err.status === 403) {
				redirect(302, '/auth/logout');
			}
			error(502, 'siem-api unavailable');
		}
		throw err;
	}

	return { roleMappings: settings.role_mappings };
};
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd siem-web && npm run test:unit -- --run routes/settings`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add siem-web/src/routes/settings/+page.server.ts siem-web/src/routes/settings/page.server.test.ts
git commit -m "Add /settings load function"
```

---

### Task 5: siem-web — `RoleMappingTable.svelte` and `RoleMappingForm.svelte`

**Files:**
- Create: `siem-web/src/lib/components/RoleMappingTable.svelte`
- Create: `siem-web/src/lib/components/RoleMappingForm.svelte`

**Interfaces:**
- Consumes: `RoleMappingResponse` (Task 2), `roleCapabilityLabel` (Task 1).
- Produces: both consumed by `Task 6`'s `+page.svelte`. No unit tests, per convention.

**Read this before implementing `RoleMappingForm.svelte`:** per this plan's Global
Constraints, the form must PUT only the single mapping being added or edited — never
the full `existingMappings` list — because `PUT /settings/auth` upserts each mapping in
its request body without deleting anything absent from it. `existingMappings` is passed
in only so the form can compute the next priority for a *new* mapping; it is never
included in the request body itself.

- [ ] **Step 1: Implement `RoleMappingTable.svelte`**

Create `siem-web/src/lib/components/RoleMappingTable.svelte`:

```svelte
<script lang="ts">
	import { roleCapabilityLabel } from '$lib/settings';
	import type { RoleMappingResponse } from '$lib/server/siemApiClient';

	let {
		mappings,
		onEdit
	}: {
		mappings: RoleMappingResponse[];
		onEdit: (mapping: RoleMappingResponse) => void;
	} = $props();
</script>

<table class="table">
	<thead>
		<tr>
			<th>OIDC group claim</th>
			<th>Role</th>
			<th>Can</th>
			<th></th>
		</tr>
	</thead>
	<tbody>
		{#each mappings as mapping (mapping.id)}
			<tr>
				<td class="mono">{mapping.group_claim}</td>
				<td><span class="pill accent">{mapping.role}</span></td>
				<td>{roleCapabilityLabel(mapping.role)}</td>
				<td class="edit-cell">
					<button class="btn ghost" type="button" onclick={() => onEdit(mapping)}>Edit</button>
				</td>
			</tr>
		{/each}
	</tbody>
</table>

<style>
	.table {
		width: 100%;
		border-collapse: collapse;
		font-size: var(--text-table);
	}
	.table th,
	.table td {
		text-align: left;
		padding: 8px 0;
		border-bottom: 1px solid var(--color-line);
	}
	.table th {
		color: var(--color-muted);
		font-weight: 500;
	}
	.mono {
		font-family: var(--font-mono);
	}
	.pill {
		display: inline-flex;
		align-items: center;
		gap: 4px;
		font-size: var(--text-eyebrow);
		padding: 2px 6px;
		border-radius: 999px;
	}
	.pill.accent {
		background: var(--color-accent-tint);
		color: var(--color-accent-lighter);
	}
	.edit-cell {
		text-align: right;
	}
	.btn {
		border: 1px solid transparent;
		border-radius: var(--radius-sm);
		padding: 5px 11px;
		font-size: var(--text-table);
		cursor: pointer;
	}
	.btn.ghost {
		background: transparent;
		color: var(--color-accent-light);
		border-color: transparent;
		padding: 2px 7px;
	}
</style>
```

- [ ] **Step 2: Implement `RoleMappingForm.svelte`**

Create `siem-web/src/lib/components/RoleMappingForm.svelte`:

```svelte
<script lang="ts">
	import { invalidateAll } from '$app/navigation';
	import type { RoleMappingResponse } from '$lib/server/siemApiClient';

	let {
		mode,
		initial,
		existingMappings,
		onClose
	}: {
		mode: 'add' | 'edit';
		initial: RoleMappingResponse | null;
		existingMappings: RoleMappingResponse[];
		onClose: () => void;
	} = $props();

	let groupClaim = $state(initial?.group_claim ?? '');
	let role = $state(initial?.role ?? 'viewer');
	let submitting = $state(false);
	let error = $state<string | null>(null);

	function nextPriority(): number {
		if (existingMappings.length === 0) return 1;
		return Math.max(...existingMappings.map((m) => m.priority)) + 1;
	}

	async function submit(event: SubmitEvent) {
		event.preventDefault();
		submitting = true;
		error = null;
		try {
			const priority = mode === 'edit' && initial ? initial.priority : nextPriority();
			const response = await fetch('/api/settings/auth', {
				method: 'PUT',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({
					role_mappings: [{ group_claim: groupClaim, role, priority }]
				})
			});
			if (!response.ok) {
				error = 'Failed to save role mapping.';
				return;
			}
			await invalidateAll();
			onClose();
		} finally {
			submitting = false;
		}
	}
</script>

<div class="overlay">
	<form class="mapping-form" onsubmit={submit}>
		<h2>{mode === 'add' ? 'Add mapping' : 'Edit mapping'}</h2>
		<label>
			OIDC group claim
			<input bind:value={groupClaim} required disabled={mode === 'edit'} />
		</label>
		<label>
			Role
			<select bind:value={role}>
				<option value="viewer">viewer</option>
				<option value="analyst">analyst</option>
				<option value="admin">admin</option>
			</select>
		</label>
		{#if error}
			<p class="error">{error}</p>
		{/if}
		<div class="actions">
			<button type="button" onclick={onClose}>Cancel</button>
			<button type="submit" disabled={submitting}>
				{submitting ? 'Saving…' : 'Save'}
			</button>
		</div>
	</form>
</div>

<style>
	.overlay {
		position: fixed;
		inset: 0;
		background: rgba(0, 0, 0, 0.5);
		display: flex;
		align-items: center;
		justify-content: center;
		z-index: 10;
	}
	.mapping-form {
		background: var(--color-surface);
		border-radius: var(--radius-lg);
		box-shadow: var(--shadow-raised);
		padding: var(--space-6);
		width: 340px;
		display: flex;
		flex-direction: column;
		gap: var(--space-3);
	}
	h2 {
		margin: 0;
		font-size: var(--text-section-head);
	}
	label {
		display: flex;
		flex-direction: column;
		gap: var(--space-1);
		font-size: var(--text-label);
		color: var(--color-muted);
	}
	input,
	select {
		background: var(--color-surface-2);
		border: none;
		border-radius: var(--radius-sm);
		color: var(--color-text);
		padding: var(--space-2);
		font-size: var(--text-table);
		font-family: inherit;
	}
	input:disabled {
		opacity: 0.6;
	}
	.error {
		color: var(--color-severity-critical);
		font-size: var(--text-label);
		margin: 0;
	}
	.actions {
		display: flex;
		justify-content: flex-end;
		gap: var(--space-2);
		margin-top: var(--space-2);
	}
	.actions button {
		border: none;
		border-radius: var(--radius-sm);
		padding: var(--space-1) var(--space-3);
		font-size: var(--text-label);
		cursor: pointer;
	}
	.actions button[type='submit'] {
		background: var(--color-accent-tint-2);
		color: var(--color-accent-lighter);
	}
	.actions button[type='button'] {
		background: var(--color-surface-2);
		color: var(--color-text);
	}
	.actions button:disabled {
		opacity: 0.6;
		cursor: default;
	}
</style>
```

- [ ] **Step 3: Typecheck**

Run: `cd siem-web && npm run check && npm run lint`
Expected: no new errors from these two files.

- [ ] **Step 4: Commit**

```bash
git add siem-web/src/lib/components/RoleMappingTable.svelte siem-web/src/lib/components/RoleMappingForm.svelte
git commit -m "Add RoleMappingTable and RoleMappingForm components"
```

---

### Task 6: siem-web — `/settings` page assembly

**Files:**
- Modify: `siem-web/src/routes/settings/+page.svelte` (full rewrite of the file)
- Delete: `siem-web/src/routes/settings.test.ts`
- Modify: `siem-web/src/lib/components/Nav.svelte`

**Interfaces:**
- Consumes: `Task 4`'s `PageData`, `Task 5`'s two components.

- [ ] **Step 1: Delete the obsolete top-level settings test**

`siem-web/src/routes/settings.test.ts` asserts on static strings from the OIDC provider
panel ("Continue with PocketID", "Test connection") that this task removes entirely.
Delete this file — matching this project's established convention of no unit tests for
`+page.svelte` files; the real coverage for this page now lives in Task 4's load-function
test.

```bash
git rm siem-web/src/routes/settings.test.ts
```

- [ ] **Step 2: Rewrite the page**

Replace the full contents of `siem-web/src/routes/settings/+page.svelte` with:

```svelte
<script lang="ts">
	import RoleMappingTable from '$lib/components/RoleMappingTable.svelte';
	import RoleMappingForm from '$lib/components/RoleMappingForm.svelte';
	import type { RoleMappingResponse } from '$lib/server/siemApiClient';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	type SectionKey = 'authentication' | 'retention' | 'notifications' | 'parsers' | 'backups' | 'about';

	let selectedSection = $state<SectionKey>('authentication');

	const sections: { key: SectionKey; label: string }[] = [
		{ key: 'authentication', label: 'Authentication' },
		{ key: 'retention', label: 'Retention & storage' },
		{ key: 'notifications', label: 'Notifications' },
		{ key: 'parsers', label: 'Parsers' },
		{ key: 'backups', label: 'Backups' },
		{ key: 'about', label: 'About' }
	];

	function selectSection(key: SectionKey) {
		selectedSection = key;
	}

	let formMode = $state<'add' | 'edit' | null>(null);
	let formSeed = $state<RoleMappingResponse | null>(null);

	function openAddForm() {
		formSeed = null;
		formMode = 'add';
	}

	function openEditForm(mapping: RoleMappingResponse) {
		formSeed = mapping;
		formMode = 'edit';
	}

	function closeForm() {
		formMode = null;
		formSeed = null;
	}
</script>

<main class="settings-shell">
	<nav class="sidebar" aria-label="Settings sections">
		{#each sections as section (section.key)}
			<button
				type="button"
				class:selected={selectedSection === section.key}
				onclick={() => selectSection(section.key)}
			>
				{section.label}
			</button>
		{/each}
	</nav>

	<section class="content">
		{#if selectedSection === 'authentication'}
			<div class="hero">
				<h1>Authentication</h1>
				<p>
					homeSIEM delegates identity to your OIDC provider. Local accounts stay available as a
					break-glass path only.
				</p>
			</div>

			<div class="panel">
				<div class="panel-head">
					<span class="panel-title">Group → role mapping</span>
					<span class="muted">first match wins; users in no listed group are denied</span>
					<button class="btn ghost" type="button" onclick={openAddForm}>+ Add mapping</button>
				</div>

				<RoleMappingTable mappings={data.roleMappings} onEdit={openEditForm} />
			</div>
		{:else}
			<div class="hero">
				<h1>{sections.find((section) => section.key === selectedSection)?.label}</h1>
				<p>This section is ready for the next set of settings content.</p>
			</div>
		{/if}
	</section>
</main>

{#if formMode}
	<RoleMappingForm
		mode={formMode}
		initial={formSeed}
		existingMappings={data.roleMappings}
		onClose={closeForm}
	/>
{/if}

<style>
	.settings-shell {
		display: flex;
		gap: var(--space-6);
		padding: var(--space-5) var(--space-6);
		min-height: 0;
	}

	.sidebar {
		width: 168px;
		flex: none;
		display: flex;
		flex-direction: column;
		gap: 2.8px;
	}

	.sidebar button {
		border: 0;
		background: transparent;
		color: var(--color-muted);
		text-align: left;
		padding: 6px 8.4px;
		border-radius: var(--radius-sm);
		cursor: pointer;
		font-size: var(--text-table);
	}

	.sidebar button.selected {
		background: var(--color-surface);
		color: var(--color-text);
		box-shadow: inset 2px 0 0 var(--color-accent);
	}

	.content {
		flex: 1;
		min-width: 0;
		max-width: 840px;
		display: flex;
		flex-direction: column;
		gap: var(--space-6);
	}

	.hero h1 {
		font-size: var(--text-page-title);
		margin: 0 0 4px;
	}

	.hero p {
		margin: 0;
		font-size: var(--text-body);
		color: var(--color-text-3);
		max-width: 70ch;
		line-height: 1.6;
	}

	.panel {
		display: flex;
		flex-direction: column;
		gap: var(--space-4);
		padding: var(--space-4);
		border-radius: var(--radius-default);
		background: var(--color-surface-2);
		box-shadow: var(--shadow-flat);
	}

	.panel-head {
		display: flex;
		align-items: center;
		gap: var(--space-3);
		flex-wrap: wrap;
	}

	.panel-title {
		font-size: var(--text-section-head);
		font-weight: 500;
	}

	.muted {
		font-size: var(--text-label);
		color: var(--color-muted);
	}

	.btn {
		border: 1px solid transparent;
		border-radius: var(--radius-sm);
		padding: 5px 11px;
		font-size: var(--text-table);
		cursor: pointer;
	}

	.btn.ghost {
		background: transparent;
		color: var(--color-accent-light);
		border-color: transparent;
		margin-left: auto;
		padding: 2px 7px;
	}
</style>
```

This removes the OIDC provider panel, the Session & break-glass panel, the hardcoded
`oidc`/`roleMappings` mock state, and every CSS rule that only those two panels used
(`.form-grid`, `.field`, `.status`, `.dot`, `.cards`, `.card*`, `.actions`,
`.btn.primary`, `.btn.secondary`, `.table` — the last one moves into
`RoleMappingTable.svelte`, per Task 5).

- [ ] **Step 3: Drop the now-unnecessary `Pathname` assertion in `Nav.svelte`**

`/settings` has been a real route since before this plan (the static mockup page already
existed at this path), so `Nav.svelte`'s cast was already unnecessary — this is the last
of the six screens, so this is also the last cleanup of this kind. In
`siem-web/src/lib/components/Nav.svelte`, the current content is:

```ts
	// Only `/settings` remains a future sub-project whose path isn't in SvelteKit's
	// generated `Pathname` union yet; it's asserted to `Pathname` so `resolve()`
	// (which applies `base`) can still be used uniformly — drop the assertion once
	// that route lands.
	const navItems: { label: string; href: Pathname }[] = [
		{ label: 'Wall', href: '/' },
		{ label: 'Search', href: '/search' },
		{ label: 'Live tail', href: '/tail' },
		{ label: 'Alerts', href: '/alerts' },
		{ label: 'Sources', href: '/sources' },
		{ label: 'Settings', href: '/settings' as Pathname }
	];
```

Replace it with (dropping the now-stale comment entirely — every route is real now, so
nothing in this file needs the assertion the comment was explaining):

```ts
	const navItems: { label: string; href: Pathname }[] = [
		{ label: 'Wall', href: '/' },
		{ label: 'Search', href: '/search' },
		{ label: 'Live tail', href: '/tail' },
		{ label: 'Alerts', href: '/alerts' },
		{ label: 'Sources', href: '/sources' },
		{ label: 'Settings', href: '/settings' }
	];
```

- [ ] **Step 4: Typecheck, lint, and run the full test suite**

Run: `cd siem-web && npm run check && npm run lint && npm run test:unit -- --run`
Expected: no new type errors, no lint errors, all tests (existing + this plan's) pass.
Note: this plan's Global Constraints don't require fixing the pre-existing Prettier
formatting issue in the old `settings/+page.svelte` — Step 2 replaces the file's content
entirely, so run `npx prettier --write siem-web/src/routes/settings/+page.svelte` (or
rely on `npm run lint`'s failure output) to ensure the new content is itself
correctly formatted from the start.

- [ ] **Step 5: Manual verification**

Run `cd siem-web && npm run dev`. Since this environment likely has no real siem-api
running, confirm at minimum: `/settings` fails gracefully (redirect to login, or a clean
502 error page) rather than crashing with an unhandled exception or a Svelte compile
error. Note what you actually observed.

- [ ] **Step 6: Commit**

```bash
git add siem-web/src/routes/settings/+page.svelte siem-web/src/lib/components/Nav.svelte
git commit -m "Wire the Settings Authentication page to real role-mapping data"
```
