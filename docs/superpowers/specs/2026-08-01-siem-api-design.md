# siem-api design

Status: approved
Scope: first sub-project of the homeSIEM handoff (`design_handoff_homesiem/README.md`).
The other two services (`siem-ingest` config deploy, `siem-web` console) are separate
sub-projects, out of scope here.

## Context

`design_handoff_homesiem/README.md` is a high-fidelity handoff: full design tokens and
six console screens for `siem-web`, and complete reference configs (`docker-compose.yml`,
`.env.example`, `vector.toml`, `schema.sql`) for the system. `siem-api` is the one service
described as "the real work" — no reference implementation exists for it, just a
responsibilities list and an API surface. This spec covers its internal design.

The repo is currently greenfield: only `README.md` and the handoff bundle exist.

## Goals

Build the full v1 `siem-api` surface as scoped by the handoff (not a trimmed slice):

- Rule scheduler evaluating threshold / first-seen / absence rules on staggered intervals
- Fast-path ingest endpoint for pre-filtered events from Vector
- Alert lifecycle: dedupe by `(rule, group_key)`, cooldown, event_count accrual, SSE +
  ntfy notification
- Auth: OIDC-derived identity trusted from `siem-web`'s BFF, groups→role RBAC
- Audit log for every ack, mute, rule change, and login
- API surface: `/healthz`, `/events/search`, `/events/tail` (SSE), `/alerts`,
  `/alerts/:id/ack`, `/rules`, `/sources`, `/settings/auth`, `/ingest/fastpath`

Explicitly out of scope for this pass: `siem-web`, `siem-ingest` deployment, Dockerfile
CI/multi-arch publishing (a Dockerfile is included; GitHub Actions CI is deferred to the
later deployment-scaffold sub-project), and any changes to the reference `schema.sql`
beyond the one addition noted below.

## Stack

- **Go**, per the handoff's suggestion (single binary, arm64 target, Pi 5 memory limits).
- **stdlib-first**: `net/http` with Go 1.22+ pattern-based routing (no router library),
  `log/slog` for structured JSON logs, stdlib goroutines + `time.Ticker` for scheduling.
- **`modernc.org/sqlite`** — pure-Go, CGO-free SQLite driver. Chosen specifically because
  the eventual multi-arch build (`linux/amd64` + `linux/arm64` via `docker buildx`) is far
  simpler without a C toolchain per architecture. `mattn/go-sqlite3` (CGO) was considered
  and rejected for this reason.
- **`golang-jwt/jwt/v5`** for verifying the internal HMAC-signed session token forwarded by
  `siem-web`'s BFF (see Auth & RBAC below). `siem-api` itself never talks OIDC/JWKS — that
  happens once, in the BFF, which is out of scope for this spec. `coreos/go-oidc` is
  therefore a `siem-web` dependency, not a `siem-api` one.
- **`golang.org/x/crypto/bcrypt`** for the break-glass local admin password check.
- No ORM, no query builder: `database/sql` directly against `modernc.org/sqlite`.

## Package layout

```text
siem-api/
  cmd/siem-api/main.go        # wiring: config -> store -> clients -> scheduler -> HTTP server
  internal/
    config/                   # env var parsing into one struct, validated at startup
    store/                    # SQLite access; only package touching *sql.DB directly
    auth/                     # internal-JWT verification, groups->role mapping, RBAC middleware
    loki/                     # Loki HTTP client: query_range, label-safe query building
    ntfy/                     # ntfy publish client
    rules/                    # scheduler + threshold/first_seen/absence evaluators
    alerts/                   # lifecycle: dedupe, cooldown, event_count accrual
    sse/                      # broadcast hub for /alerts and /events/tail
    api/                      # HTTP handlers, routing, middleware chain
  schema.sql                  # copy of the handoff's reference schema, plus seen_values (below)
  Dockerfile
  go.mod
```

Each `internal/` package has one responsibility and depends on others through small
interfaces — e.g. `rules` depends on a `LokiQuerier` interface, not the concrete
`loki.Client`, so evaluators are unit-testable without a real Loki. `store` is the sole
owner of the database handle.

## Auth & RBAC

`siem-api` sits on the `backend` network only, reachable exclusively through `siem-web`'s
BFF. Rather than verifying the OIDC ID token itself on every request, `siem-api` trusts a
short-lived **internal JWT** minted by the BFF at login (claims: `sub`, `email`, `groups`),
signed with the shared `SIEM_SESSION_SECRET`. OIDC/JWKS verification happens exactly once,
in the BFF, at the OIDC callback — `siem-api` only verifies the internal signature.

**Config additions beyond the reference `.env.example`/`docker-compose.yml`** (both are
deployment-scaffold concerns, out of scope here, but `siem-api` needs these env vars once
that scaffold is built): `SIEM_SESSION_SECRET` already exists in `.env.example` and is
reused as-is; `SIEM_FASTPATH_TOKEN` is new, a static shared token `/ingest/fastpath`
checks directly (Vector isn't an OIDC client, so it can't carry the internal JWT);
`SIEM_LOCAL_ADMIN_USERNAME`/`SIEM_LOCAL_ADMIN_PASSWORD_HASH` seed the one break-glass row
in `users` on first startup (idempotent — a pre-existing row is left alone).

- `auth.Middleware` decodes the internal JWT, maps `groups` → role via `role_mappings`
  (first match by `priority`, deny if unmapped), attaches `(userID, role)` to context.
- Per-route role requirements are declared alongside route registration in `api/routes.go`.
- Break-glass local admin: `POST /auth/local` (username + bcrypt against
  `users.local_hash`), bypassing OIDC entirely. This is an addition beyond the handoff's
  listed API surface, needed to fulfil its "restricted to LAN source addresses" break-glass
  requirement. The LAN restriction itself is enforced by `siem-web`'s BFF, which sees the
  real client IP via `X-Forwarded-For` from nginx-proxy-manager — `siem-api` only ever sees
  traffic arriving from the BFF on the `backend` network, so a redundant IP check on its
  side would be checking the BFF's address, not the browser's, and is deliberately omitted.
- `POST /ingest/fastpath` is called by Vector, not a browser session — authenticated by a
  separate static shared token (env var), checked directly, not via the JWT path.

## Rule scheduler & evaluation

- On startup, load enabled rules from `store`; one goroutine per rule on
  `time.Ticker(interval_sec)` with a randomized startup jitter (0..interval) so rules
  don't all query Loki simultaneously.
- Rule CRUD starts/stops/restarts the relevant goroutine directly — no polling for changes.
- `rules.Evaluator` interface, `Evaluate(ctx, rule) ([]Candidate, error)`, three
  implementations:
  - `ThresholdEvaluator` — LogQL over `window_sec`, grouped by `group_by`, compared to
    `threshold`.
  - `FirstSeenEvaluator` — LogQL results diffed against a local "seen" set.
  - `AbsenceEvaluator` — no Loki query; checks `sources.last_seen_at` vs `heartbeat_sec`.
- **Schema addition**: `schema.sql` has no table for first-seen tracking. Adding one:

  ```sql
  CREATE TABLE seen_values (
    id            INTEGER PRIMARY KEY,
    rule_id       INTEGER NOT NULL REFERENCES rules(id) ON DELETE CASCADE,
    value         TEXT NOT NULL,
    first_seen_at TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE (rule_id, value)
  );
  ```

  This is the only deviation from the reference schema; everything else is used as-is.
- Both the scheduler and `/ingest/fastpath` converge on one `alerts.Raise(candidate)` call
  — dedupe/cooldown logic lives in exactly one place, not duplicated per ingestion path.

## Alert lifecycle & fastpath ingest

`alerts.Raise(candidate)`:

1. Look up an existing open alert via `(rule_id, group_key, state='open')`.
2. If found within `cooldown_sec` of `last_seen_at`: increment `event_count`, update
   `last_seen_at`, append a capped sample (latest 10) to `alert_samples`. No notification.
3. If not found, or found with cooldown expired: insert/reopen, reset the cooldown clock,
   notify.
4. Notify = publish to `sse.Hub` + best-effort POST to ntfy. An ntfy failure is logged and
   never blocks the alert record itself.

`POST /ingest/fastpath` does in-process field matching (not full LogQL re-evaluation) —
the handoff's fast-path examples (threat-intel hits, WAN drops) are simple field checks —
then calls `alerts.Raise`. No Loki round-trip on this path.

`sse.Hub` is a plain fan-out: one goroutine per connected client over a per-client channel;
`alerts` and the tail endpoint both publish into it; disconnect cleans up the channel.

## Search & tail

- `loki.Client.QueryRange` backs both `/events/search` and rule evaluation.
- Loki has no native streaming tail; `/events/tail` is implemented as `siem-api` polling
  `query_range` on a short interval (~1s), filtered to `job="siem"` plus active filters,
  deduped by a watermark timestamp, pushed into `sse.Hub`.
- All queries scoped to `job="siem"`, built through one `loki.BuildQuery(filters)` helper
  that only ever emits the mandated label set (`source`, `host`, `program`, `severity`,
  `facility`) as LogQL labels — everything else (`src_ip`, `rule`, `geoip.cc`, ...) goes
  through line/JSON filters, never labels. This is the enforcement point for the handoff's
  "label discipline (non-negotiable)" rule.
- `/events/search` responses include the compiled LogQL string, per the handoff's "always
  show the compiled query" requirement.
- Range bounding (default 24h, warn past 7d) is a response-field/UI concern; `siem-api`
  passes range through as given. `split_queries_by_interval`/`max_query_parallelism` are
  Loki-server config, not `siem-api` logic.

## API surface & middleware

Single `api/routes.go` registering the full handoff surface on stdlib `http.ServeMux`.
Middleware chain: recover → log → auth → RBAC role check → handler, with `/healthz` and
`/ingest/fastpath` exempted from the OIDC-derived auth path (fastpath uses its own shared
token; healthz is unauthenticated).

## Testing

- Table-driven unit tests per evaluator against a fake `LokiQuerier`.
- `store` tests against a real `modernc.org/sqlite` file in a temp dir — fast enough that
  mocking SQLite isn't worth it.
- Handler tests via `httptest` with fake `alerts`/`store`.
- One integration test booting the real scheduler against an `httptest.Server` fake Loki,
  asserting an alert is raised end-to-end for a threshold rule.
- No automated tests against the real homelab Loki/Pocket ID/ntfy — CI has no network path
  to `townsville.cc`; those get exercised manually against real instances once deployed
  (per the "real homelab instances" dev-environment decision below).

## Observability

`log/slog` structured JSON logs with a request ID per request. Every audit-worthy action
(ack, mute, rule change, login) writes to the `audit` table synchronously in the same
transaction as the state change it records, not as a side-channel that could drift from
what actually happened.

## Decisions carried from brainstorming

- Build order: `siem-api` first, then `siem-web`, then `siem-ingest`/deployment scaffold.
- Dev/test against real homelab Loki/Pocket ID/ntfy instances (already running), not fakes
  — except in the automated test suite, which has no network access to them.
- v1 scope is the full API surface as specified in the handoff, not a trimmed slice.
- Dockerfile included in this pass; multi-arch CI (GitHub Actions → ghcr.io) deferred to
  the deployment-scaffold sub-project.
