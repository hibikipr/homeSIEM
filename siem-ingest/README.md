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
- `test/` — a local Docker-based verification harness (real Vector, real
  Loki, a stub HTTP receiver standing in for siem-api). Run
  `test/fetch-test-fixtures.sh` once, then `docker compose -f test/docker-compose.yml up`,
  then `test/send-test-traffic.sh` to send synthetic syslog and observe the
  pipeline behave. Not a substitute for testing against your real UniFi
  gateway and hosts once deployed.

## Known gaps in this pass

- No real GeoIP or threat-intel data is provisioned — see the setup docs.
- The TLS source (port 6514) needs a certificate generated per
  `docs/tls-setup.md` before any host can use it.
- Port 6514's TLS source may require mutual TLS (Vector requiring the
  sending host to present a client certificate Vector trusts, not just the
  reverse) — a bare test connection without a client cert failed the
  handshake during development. Not fully verified in production; see
  `docs/tls-setup.md` for detail before relying on this port.
- `heartbeat_sec` (how long before a source is considered "silent") is
  hardcoded to the schema default on every heartbeat call — there's no UI
  yet to customize it per source (that belongs to the not-yet-built Sources
  screen).
- This pass verifies the pipeline against synthetic traffic in a local
  Docker harness, not against the real UDM-Ultra or real hosts — that's the
  next real-world verification step once deployed.
