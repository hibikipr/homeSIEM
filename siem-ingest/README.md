# siem-ingest

Vector pipeline for homeSIEM: receives syslog from the UniFi gateway
(UDP/514) and generic hosts (TCP/601, TLS/6514), parses and enriches each
event, and forwards to Loki (everything), siem-api's fast-path endpoint
(high-signal events only), and siem-api's source-heartbeat endpoint (proof
of life for the absence-rule shape and the future Sources screen).

See `docs/superpowers/specs/2026-08-03-siem-ingest-design.md` for the design.

## Files

- `vector.toml` — the pipeline config, deployed to
  `${MY_DOCKER_DATA_DIR}/homesiem/vector/vector.toml`.
- `docker-compose.yml` — the deployment stack. Copy into your `homelab`
  repo's `stacks/homelab-siem/` alongside siem-api's and siem-web's own
  service blocks (see the design handoff's reference compose for how all
  three fit together) — this repo does not deploy anywhere itself.
- `docs/geoip-setup.md`, `docs/tls-setup.md` — provisioning steps for the
  two external dependencies nothing in this repo can supply (real GeoLite2
  data requires a MaxMind account; TLS needs a cert generated on the real
  host).
- `scripts/update-threatlist.py` — generates `threatlist.csv` from five
  free IP threat-intel feeds (stdlib-only, no `pip install`). Runs as the
  `siem-threatlist-updater` service in `docker-compose.yml` (built from
  `scripts/Dockerfile`, published to
  `ghcr.io/hibikipr/siem-threatlist-updater`) — see `docs/geoip-setup.md`'s
  threatlist.csv section for how that's wired up and why these particular
  feeds.
- `test/` — a local Docker-based verification harness (real Vector, real
  Loki, a stub HTTP receiver standing in for siem-api). Run
  `test/fetch-test-fixtures.sh` once, then `docker compose -f test/docker-compose.yml up`,
  then `test/send-test-traffic.sh` to send synthetic syslog and observe the
  pipeline behave. Not a substitute for testing against your real UniFi
  gateway and hosts once deployed.

## Known gaps in this pass

- No real GeoIP data is provisioned — see `docs/geoip-setup.md`. Nothing
  can automate this one (needs your own MaxMind account). Threat-intel is
  fully solved when deployed via Docker Compose: the
  `siem-threatlist-updater` service fetches and refreshes real data on its
  own. Running `scripts/update-threatlist.py` outside of Compose (bare
  metal) still needs your own cron/systemd timer per the doc's suggested
  crontab line.
- The TLS source (port 6514) needs a certificate generated per
  `docs/tls-setup.md` before any host can use it.
- Port 6514 runs **one-way** TLS (`verify_certificate = false`). On a Vector
  source that setting means client-certificate verification, and leaving it
  `true` without a `ca_file` made the port reject every connection — now
  confirmed and fixed, with the evidence and an opt-in mutual-TLS recipe in
  `docs/tls-setup.md`. Each sending host must trust the self-signed server
  certificate.
- Port 6514 has been exercised end-to-end only with `openssl s_client`, not
  with a real syslog daemon (rsyslog/syslog-ng) configured for TLS forwarding
  — confirm your sender's certificate-trust configuration when you deploy.
- `heartbeat_sec` (how long before a source is considered "silent") is
  hardcoded to the schema default on every heartbeat call — there's no UI
  yet to customize it per source (that belongs to the not-yet-built Sources
  screen).
- This pass verifies the pipeline against synthetic traffic in a local
  Docker harness, not against the real UDM-Ultra or real hosts — that's the
  next real-world verification step once deployed.
