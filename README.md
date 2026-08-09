# homeSIEM

A self-hosted syslog collector and security console for a homelab. homeSIEM
receives syslog from network gear (a UniFi gateway to start) and servers,
parses and enriches it, stores it in Loki, evaluates correlation rules, and
raises alerts an operator triages in a browser console authenticated over
OIDC.

It is not a metrics platform — Prometheus/Grafana/etc. already cover
infrastructure health. homeSIEM covers triage of security-relevant log
events: alerts, acknowledgements, rules, a source registry.

## Architecture

| Service | What it is |
| --- | --- |
| [`siem-ingest`](siem-ingest/) | Vector, configured: syslog sources for UniFi (UDP/514) and generic hosts (TCP/601, TLS/6514), parsing, geo/threat-intel enrichment, a fast path to siem-api, and the full stream to Loki. |
| [`siem-api`](siem-api/) | Go service: rule scheduler (threshold / first-seen / absence rule shapes), alert lifecycle, Loki client, OIDC token verification + RBAC, SQLite state, rich ntfy delivery (severity emoji tags, Markdown body, click-through link, action button, icon). |
| [`siem-web`](siem-web/) | SvelteKit console + a thin BFF holding the session cookie: Wall, Search, Live tail, Alerts, Sources, Settings. |

Two pieces of shared infra it depends on but doesn't own:

- **Loki** — the only place log events live; homeSIEM never puts raw events in SQLite.
- **ntfy** — alert delivery.

And one thing it deliberately does not provide: **an OIDC identity
provider.** siem-web/siem-api authenticate against whatever OIDC issuer you
point them at — [Pocket ID](https://github.com/pocket-id/pocket-id) is what
this project was built against and is recommended, but any provider that
does authorization-code+PKCE and can send a `groups` claim works.

## Quickstart (`docker-compose.yml`)

The root [`docker-compose.yml`](docker-compose.yml) is a self-contained
example stack — unlike the three services' own repos, it bundles Loki and
ntfy too, so `docker compose up` gives you a fully working homeSIEM with no
pre-existing homelab infrastructure required. It's a starting point for
your own deployment, not a prescription — see each service's `docker-compose.yml`
(`siem-ingest/docker-compose.yml`, and the design handoff's reference
compose under `design_handoff_homesiem/reference/`) for the conventions a
real homelab deployment alongside existing Loki/ntfy/nginx-proxy-manager
infra would follow instead.

### 1. Configure

```bash
cp .env.example .env
```

Generate the two secrets:

```bash
# SIEM_SESSION_SECRET — signs the session cookie, shared between siem-api and siem-web
openssl rand -base64 32

# SIEM_FASTPATH_TOKEN — shared secret between Vector and siem-api's ingest endpoints
openssl rand -hex 32
```

Register an OIDC client with your provider: a **public** client (no secret),
authorization-code + PKCE, redirect URI `${SIEM_APP_URL}/auth/callback`
(`http://localhost:8080/auth/callback` if you're keeping the default
`SIEM_APP_URL`), scopes `openid profile email groups`. Fill
`SIEM_OIDC_ISSUER` / `SIEM_OIDC_CLIENT_ID` / `SIEM_OIDC_LOGOUT_URL` in `.env`
with what your provider gives you.

`SIEM_APP_URL` defaults to `http://localhost:8080`, which works without TLS
because browsers treat `localhost` as a secure context — the session cookie
is `Secure`. Anything else (a LAN IP, a real hostname) needs HTTPS in front
(nginx-proxy-manager, Caddy, etc.) or login will silently fail to persist a
session.

### 2. Bring up the core stack

```bash
docker compose up -d --build
```

This starts Loki, ntfy, siem-api and siem-web. `siem-ingest` is deliberately
**not** included yet — see step 4.

### 3. First login: bootstrap the initial admin role mapping

homeSIEM maps OIDC `groups` claims to roles (`viewer` / `analyst` / `admin`)
via a table that starts **empty** on a fresh database — nobody can be
assigned a role, so the very first login has nothing to match against and
is rejected. Seed one mapping before you try to log in, using whatever
group name your OIDC provider actually sends for your account:

```bash
docker run --rm -v siem_data:/data alpine:3.19 sh -c \
  "apk add --no-cache sqlite && sqlite3 /data/siem.db \
   \"INSERT INTO role_mappings (group_claim, role, priority) VALUES ('admins', 'admin', 1);\""
```

This runs a throwaway container on Docker's default network rather than
`docker compose run` on `siem-api`'s own — `apk add` needs internet access,
and a real deployment (e.g. a stack sharing this repo's existing Loki/ntfy
over an internal-only Docker network) may well have `siem-api` on a network
with no route out. Mounting the `siem_data` volume directly sidesteps that
regardless of topology. It doesn't need to run from any particular
directory, only that the `siem_data` volume already exists (i.e. `siem-api`
has started at least once).

(This only needs to run once — after that, manage additional mappings from
the console's Settings → Authentication screen as an admin.)

Then open `http://localhost:8080` and sign in.

### 4. (Optional) turn on live ingest

`siem-ingest` sits behind the `ingest` Compose profile because Vector
refuses to start without a provisioned GeoIP database — a missing file
takes down the whole pipeline, not just enrichment (see
`siem-ingest/docs/geoip-setup.md`). Before enabling it:

1. Follow `siem-ingest/docs/geoip-setup.md` to place `GeoLite2-City.mmdb`
   under `./siem-ingest/geoip/` — this one still needs your own MaxMind
   account, nothing can automate it. `threatlist.csv` in the same
   directory, by contrast, is handled for you: the `ingest` profile also
   brings up `siem-threatlist-updater`, which fetches real threat-intel
   data on its own and keeps it refreshed daily — `siem-ingest` won't even
   start until that file actually has content.
2. Follow `siem-ingest/docs/tls-setup.md` to generate a self-signed
   cert/key pair under `./siem-ingest/tls/` (needed for the syslog-TLS
   source on port 6514 even if you don't plan to use it).
3. `docker compose --profile ingest up -d`

Point your UniFi gateway or hosts' syslog at this machine — UDP/514,
TCP/601, or TCP+TLS/6514 (see `siem-ingest/docs/tls-setup.md` for what
sending hosts need to trust).

### Verifying it's up

```bash
curl http://localhost:8080/healthz   # siem-web
curl http://localhost:3100/ready     # loki
curl http://localhost:8081/v1/health # ntfy
```

Subscribe to alert delivery at `http://localhost:8081/<SIEM_NTFY_TOPIC>`
(web UI, or the ntfy mobile app pointed at this host).

## Building and publishing images

[`.github/workflows/docker-build.yml`](.github/workflows/docker-build.yml)
builds `siem-api`, `siem-web`, and `siem-threatlist-updater` for
`linux/amd64,linux/arm64` (the Raspberry Pi 5 target the design handoff
calls out) whenever a GitHub Release is published (or on a manual
`workflow_dispatch` run), and publishes to `ghcr.io/hibikipr/<service>`,
tagged `:latest` (stable releases only — a prerelease/beta never becomes
`:latest`), `:<version>` (e.g. `:0.1.0`), and `:<major>.<minor>`. Cut a
release (`git tag vX.Y.Z && git push origin vX.Y.Z`, then
`gh release create vX.Y.Z`) to publish new images — pushes to `main`
alone no longer trigger a build. `siem-ingest` itself still has no image
of its own — it runs the upstream `timberio/vector` image with a mounted
config.

To build locally instead of pulling from GHCR:

```bash
docker compose build
```

`docker-compose.yml` declares both `image:` and `build:` for siem-api and
siem-web, so `docker compose up --build` always uses what you just built
rather than the published tag.

## Known gaps

Each service's own README has the full list for that service
(`siem-api` doesn't have one yet — see its Go source and
`docs/superpowers/specs/2026-08-01-siem-api-design.md`
for its API surface). Deployment-level gaps worth knowing about before you
rely on this:

- **No local user database** — if your OIDC provider is down, you cannot
  sign in. siem-api does have a break-glass local-admin login path
  (`SIEM_LOCAL_ADMIN_USERNAME`/`SIEM_LOCAL_ADMIN_PASSWORD_HASH`), but
  siem-web has no UI wired up to use it yet.
- **The role-mapping bootstrap above is a manual SQL step** — there's no
  first-run wizard; see step 3.
- **The example `docker-compose.yml`'s `ntfy` has no persistent cache** —
  messages delivered while no client is subscribed are not replayed.
- **GeoIP/threat-intel data is never embedded in this repo** — you provision
  your own MaxMind account and threat-intel feed; see
  `siem-ingest/docs/geoip-setup.md`.
- **Multi-arch images** are built by CI for `linux/amd64,linux/arm64` and are
  verified in practice on real arm64 hardware — `siem-api`, `siem-web`, and
  `siem-threatlist-updater` all run as the published `ghcr.io` images on a
  Raspberry Pi in the reference deployment.

## Development

Each service has its own local dev instructions:

- [`siem-web/README.md`](siem-web/README.md)
- [`siem-ingest/README.md`](siem-ingest/README.md)
- `siem-api`: `cd siem-api && go test ./... && go run ./cmd/siem-api`
  (needs the same env vars as the compose file's `siem-api` service).
