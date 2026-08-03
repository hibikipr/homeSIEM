# siem-ingest: Vector pipeline + heartbeat — design

Status: approved
Scope: third and final planned service of the homeSIEM handoff
(`design_handoff_homesiem/README.md`). Covers the Vector syslog-ingest pipeline
(UniFi + generic hosts, parsing, geo/threat-intel enrichment, fast-path to
siem-api), a new source-heartbeat mechanism that closes a gap left open since
siem-api's own final review, and the deployment stack definition. Building,
deploying, and configuring the real homelab host is out of scope — this
sub-project produces the config/code/docs; the user provisions the real
GeoIP/threat-intel data, TLS certs, and the live deployment themselves.

## Context

The design handoff bundle already ships a detailed reference `vector.toml` and
`docker-compose.yml` under `design_handoff_homesiem/reference/` — these are
authoritative starting points, not rough sketches (confirmed during
brainstorming), matching the "high fidelity, recreate faithfully" approach
already used for the Wall/Alerts screens' mockups.

siem-api's own final whole-branch review flagged a real, deliberately-deferred
gap: nothing populates the `sources` table or bumps `last_seen_at`, so the
absence rule shape (a source going silent) can never fire, and the Sources
screen's future "silent" health indicator has no data. That review explicitly
named this sub-project as where the gap would close. It does, here.

Convenient discovery made during store-layer research: siem-api **already**
has the store primitives this needs — `Store.UpsertSource` (insert-or-update
by `name`, unique-constrained) and `Store.TouchSourceLastSeen` (bump
`last_seen_at` by `name`). Only the HTTP handler and the Vector-side heartbeat
sink are missing.

## Goals

- Vector config (`vector.toml`): syslog sources for UniFi (UDP/514) and
  generic hosts (TCP/601, TLS/6514), parsing per device family, geo/threat-intel
  enrichment via Vector enrichment tables, a fast-path filter to siem-api for
  high-signal events, and the full stream to Loki — all per the reference
  config, adapted only where something doesn't actually work.
- **New**: a heartbeat mechanism. Vector throttles its full enriched stream
  down to at most one event per source per interval and POSTs a small
  `{name, address, transport, parser}` payload to a new siem-api endpoint,
  `POST /sources/heartbeat`, gated by the same `X-Fastpath-Token` shared
  secret `/ingest/fastpath` already uses (Vector isn't an OIDC-authenticated
  user; this matches the existing trust boundary rather than inventing a new
  one). The endpoint upserts the source (auto-registering unclaimed senders
  by address) and bumps `last_seen_at` — the exact plumbing the absence-rule
  gap needed.
- Deployment stack (`docker-compose.yml`) for `siem-ingest`, matching the
  reference and the existing `stacks/homelab-monitoring` conventions
  (container_name, restart policy, healthcheck, resource limits, `expose`
  over `ports` except where Vector must publish host ports directly). Kept
  inside the homeSIEM repo as a deliverable — **not** written into the
  separate, live `homelab` repo (explicit choice: that repo is real deployed
  infrastructure, out of scope for this session to touch).
- Setup docs for the two external dependencies nothing in this repo can
  provision: a MaxMind GeoLite2-City license key + download steps, and the
  TLS certificate the `hosts_tls` syslog-over-TLS source needs.

Out of scope for this pass: any other homeSIEM screen; the Sources screen
itself (siem-web, future sub-project — this just makes its data exist);
actually deploying to the real homelab host; embedding real GeoIP or
threat-intel data in this repo.

## Vector pipeline

Per the reference `vector.toml`, adapted with the heartbeat addition:

- **Sources**: `unifi` (syslog, UDP, `:514`), `hosts_tcp` (syslog, TCP,
  `:601`), `hosts_tls` (syslog, TCP+TLS, `:6514`, server cert/key mounted
  read-only).
- **Transforms**: `parse_unifi` extracts UniFi firewall-rule fields
  (`src_ip`/`dst_ip`/`dst_port`/`proto`/`action`) via key-value parsing on a
  bracketed rule-name match; `parse_hosts` tags generic host syslog with
  `job`/`source`. **Extended** (small, necessary deviation from the reference,
  which has no heartbeat concept at all): both transforms also set
  `.transport` (`"udp/514"`, `"tcp/601"`, or `"tls/6514"`, matching the
  `sources.transport` column's example values) and `.parser` (`"unifi-os"` or
  `"rfc5424"`) — the metadata the new heartbeat sink needs, sourced once at
  parse time rather than re-derived later.
- **Enrichment**: `enrich_geo` looks up `src_ip` (when non-private) against a
  MaxMind GeoLite2 enrichment table and a CSV threat-intel list, same as the
  reference.
- **Fast path**: unchanged from the reference — a `filter` transform passing
  only threat-intel hits or dropped/blocked connections straight to
  `POST /ingest/fastpath`, so alert-worthy events don't wait on a query
  interval.
- **New — heartbeat**: a `throttle` transform (`threshold = 1`,
  `window_secs` matching the source's configured heartbeat interval,
  `key_field` = the parsed `source`) fed from the *full* enriched stream
  (not just fast-path), so a heartbeat fires from ANY traffic — a quiet
  device that hasn't triggered a rule still proves it's alive. A small remap
  reshapes the throttled event down to exactly `{name, address, transport,
  parser}` before an `http` sink POSTs it to `${SIEM_API_URL}/sources/heartbeat`
  with the `X-Fastpath-Token` header set from `${SIEM_FASTPATH_TOKEN}`.
  **Verify empirically during implementation** (not assumed here): the exact
  Vector syslog-source field carrying the real sender IP address — needed
  for the `address` field so newly-seen senders can be auto-registered by
  address, per the Sources screen's future "unclaimed senders" requirement.
- **Sinks**: `loki` (everything, labels `job`/`source`/`host`/`program`/`severity`
  per the non-negotiable label-discipline list — nothing else becomes a
  label), `siem_api` (fast path), and the new `siem_heartbeat`.

## siem-api addition

New commit on the still-open siem-api PR #1, same precedent as `/events/stats`
and the mute/samples endpoints added during the Wall/Alerts sub-projects.

- `POST /sources/heartbeat` — no OIDC/RBAC (Vector isn't a logged-in user);
  gated by `X-Fastpath-Token == FastpathToken` exactly like the existing
  `handleFastpath` handler. Request body `{name, address, transport, parser}`.
  Handler calls `Store.UpsertSource` (auto-registers a new source as
  unclaimed if `name` hasn't been seen, updates `address`/`transport`/`parser`
  if it has) then `Store.TouchSourceLastSeen` to bump `last_seen_at`. Returns
  `204 No Content` on success, matching the fastpath endpoint's own response
  shape.
- **Known, accepted limitation**: `UpsertSource`'s existing SQL always
  overwrites `heartbeat_sec` with whatever the caller passes. Since there's no
  UI yet to let an admin customize a source's heartbeat interval (that
  belongs to the not-yet-built Sources screen), this handler passes the
  schema's own default (900s) on every heartbeat, which is a no-op today.
  Flagged here so whoever builds the Sources screen's "edit heartbeat
  interval" feature knows this handler will need to read-then-preserve the
  existing value rather than stomp it on every heartbeat call.

## File structure

```text
siem-ingest/
  vector.toml            # main pipeline config
  docker-compose.yml      # deployment stack (stays in homeSIEM per this pass's scope choice)
  .env.example
  docs/
    geoip-setup.md        # MaxMind GeoLite2 license-key steps; threatlist.csv format & provisioning
    tls-setup.md          # self-signed cert generation for the syslog-tls source (port 6514)
```

## GeoIP / threat-intel data

Neither `GeoLite2-City.mmdb` nor `threatlist.csv` exists anywhere in this
repo, and none will be fetched or embedded during this pass (decided during
brainstorming). `docs/geoip-setup.md` documents: creating a free MaxMind
account, generating a license key, the download command/URL for
GeoLite2-City, and the expected `threatlist.csv` schema (`ip,tag` columns,
matching the reference config's `enrichment_tables.threatlist.schema`) with a
pointer to a couple of realistic public threat-intel-list sources the user
can pull from — this is a provisioning decision for the user's own real host,
not something this session can generate authoritatively.

## TLS for the syslog-TLS source

Port 6514 (`hosts_tls`) needs a server certificate. No existing cert in the
homelab is directly reusable for a raw TCP+TLS listener (nginx-proxy-manager's
Let's Encrypt certs are HTTP-routed, not exposed for arbitrary TCP services).
`docs/tls-setup.md` documents generating a self-signed cert/key pair (a
one-line `openssl req` command) and where to place it for the compose
volume mount — self-signed is acceptable here since this is host-to-host
syslog forwarding on the internal `backend` network, not a
publicly-verified TLS endpoint.

## Deployment stack

`siem-ingest/docker-compose.yml` mirrors the reference's `siem-ingest`
service block: `timberio/vector` image, `backend` network only (external,
never created here), host ports published for 514/udp, 601/tcp, 6514/tcp
(the one service in this whole project that must publish host ports, since
devices dial it directly), `expose` for Vector's internal API/health port
8686, the same resource limits (384m/1.5cpu), and the same healthcheck
pattern as the reference. Matches `stacks/homelab-monitoring`'s conventions
(`container_name`, `restart: unless-stopped`, `security_opt:
no-new-privileges`) exactly, since that's the sibling stack this one deploys
alongside. This file is a deliverable for the user to copy into their
`homelab` repo's `stacks/homelab-siem/` themselves — this session does not
write to that repo.

## Testing

No compiler or test framework validates a Vector TOML file or its remap
logic. Verification (decided during brainstorming): stand up the real
`timberio/vector` binary locally via Docker with this exact config, plus a
local Loki (Docker) and a small stub HTTP server standing in for siem-api's
`/ingest/fastpath` and `/sources/heartbeat` endpoints. Send synthetic syslog
lines shaped like real UniFi firewall-rule log lines and generic RFC 5424
host syslog at ports 514/601/6514 (via `logger`/`ncat`), and confirm:

- `vector validate` passes against the config.
- Sources bind and accept traffic on all three ports.
- `parse_unifi`/`parse_hosts` produce the exact labels/fields the design
  requires (`job=siem`, correct `source`, extracted firewall-rule fields).
- The fast-path filter correctly passes through only threat-intel-hit/blocked
  events to the stub fastpath endpoint, and nothing else.
- The heartbeat throttle fires at most once per source per window and posts
  the correct `{name, address, transport, parser}` shape (with real observed
  values for whatever field carries the sender IP) to the stub heartbeat
  endpoint, including the `X-Fastpath-Token` header.
- The Loki sink correctly carries only the six non-negotiable labels — no
  structured metadata (`src_ip`, `dst_port`, `geoip`, `threat_intel`) ever
  becomes a label, per the design's label-discipline requirement.

## Decisions carried from brainstorming

- Reference `vector.toml`/`docker-compose.yml` are authoritative starting
  points, implemented at high fidelity — deviate only where something
  doesn't work (the heartbeat mechanism is the one deliberate, necessary
  addition beyond the reference).
- Heartbeat gap closed via Vector (`throttle` transform on the full stream,
  not just fast-path) rather than having siem-api infer liveness from Loki
  queries — preserves the ability to auto-register new senders' address
  metadata, which a Loki-side inference approach couldn't provide.
- Heartbeat endpoint reuses the existing `X-Fastpath-Token` shared secret
  rather than a separate token.
- GeoIP/threat-intel data: documented provisioning steps only, no real or
  stub data embedded in this repo.
- Deployment stack file stays inside the homeSIEM repo as a deliverable;
  this session does not write into the separate, live `homelab` repo.
- Verification: real Vector + real Loki + synthetic syslog traffic locally,
  not just `vector validate` alone.
