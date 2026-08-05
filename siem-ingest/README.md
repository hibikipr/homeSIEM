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
- **UniFi OS's "SIEM Server" integration** (Settings → System Logging /
  SIEM — a distinct feature from the classic "Remote Logging" toggle this
  pipeline was originally built against) sends **CEF-formatted** messages
  wrapped in a syslog envelope with no `<PRI>` header, confirmed against a
  real UDM device's "Send Test Event" (raw packet captured via `tcpdump`).
  Two consequences, both handled but not ideal:
  - No `<PRI>` header means Vector's syslog decoder can't derive
    `.severity` — `enrich_geo` now defaults it to `"info"` when missing
    (see the comment in `vector.toml`) so the event isn't dropped
    outright, but the event's *real* CEF severity (present in the CEF
    header itself, e.g. `CEF:0|Ubiquiti|UniFi OS|...|<severity>|...`) is
    currently ignored — it's just always `"info"` for CEF-shaped events.
  - Nothing here parses CEF's pipe-delimited structure, so `host`/
    `hostname`/`appname` end up populated with whatever Vector's lenient
    RFC3164-ish decoder guessed at (observed: the CEF payload's own
    embedded timestamp landing in the `host` field) rather than real
    values. A proper fix would need a CEF-aware parse transform,
    conditioned on detecting the `CEF:` prefix — not implemented here.
  - The same malformed shape also confused Vector's timestamp decoder
    into producing a value 4 hours in the future (confirmed: matches the
    process's `TZ=America/New_York` offset exactly), which made Loki
    reject the event outright with "timestamp too new" — a second,
    independent way this one integration's malformed input caused total
    data loss beyond just the missing severity. `enrich_geo` now clamps
    any implausibly-future timestamp back to real receipt time. Verified
    this is specific to malformed input, not a blanket TZ bug: a
    well-formed RFC5424 message with an explicit `Z` timestamp parses
    correctly even with the same `TZ` set.
