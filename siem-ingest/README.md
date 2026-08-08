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
- **UniFi OS's "SIEM Server" integration** (Settings → System Logging / SIEM — a
  distinct feature from the classic "Remote Logging" toggle this pipeline was originally
  built against) sends **CEF-formatted** messages wrapped in a syslog envelope with no
  `<PRI>` header, confirmed against a real UDM device's "Send Test Event" (raw packet
  captured via `tcpdump`) and cross-referenced against Ubiquiti's and Graylog's own
  published documentation for the integration's wire format. `parse_unifi` now parses
  this directly:
  - Real CEF severity (the header's 7th pipe-delimited field, numeric 0-10) is mapped to
    this pipeline's text severity vocabulary (`0-3→info, 4-6→warning, 7-8→err,
    9-10→crit`) — no longer defaulted to `"info"` for every CEF event.
  - `host`/`hostname` are set from the CEF extension's `UNIFIdeviceName` field (falling
    back to `UNIFIhost` if absent), and `program`/`appname` from the CEF header's `Name`
    field — no longer whatever Vector's lenient RFC3164-ish decoder guessed at.
  - `src_ip`/`dst_ip`/`dst_port`/`proto` are extracted from the CEF extension's
    `src`/`dst`/`dpt`/`proto` keys, using the same field names the classic netfilter-style
    messages already use — GeoIP/threat-intel enrichment and `fast_path` forwarding work
    identically for both UniFi message formats with no further changes.
  - `enrich_geo`'s severity-default and timestamp-clamp fallbacks (see
    `docs/superpowers/specs/2026-08-07-siem-ingest-unifi-cef-parser-design.md` for the
    original bug reports) remain in place as the safety net for anything the new parsing
    branch doesn't cover — a CEF message with fewer than 7 pipe-delimited fields
    (malformed/truncated) still falls through to those defaults rather than being dropped.
  - **Not handled**: CEF's escape-sequence syntax (`\|` inside a field value) — a plain
    pipe-split is used, which matches every real example captured or documented so far,
    but isn't fully CEF-spec-compliant. CEF events also don't set `.action`
    (`"drop"`/`"accept"`), so they only reach `fast_path`'s forwarding via the
    threat-intel match path, not the drop-rule path the netfilter branch uses.
  - CEF detection also handles a real captured variant where Vector's own syslog
    decoder already consumed the leading envelope token *and* the literal `"CEF:"`
    text as an RFC3164 hostname+tag pair before `parse_unifi` ever runs (observed
    for a `"WiFi Client Roamed"` event from a real device) — in that case `.message`
    starts directly at the CEF version marker (`"0|Ubiquiti|..."`) with no `"CEF:0|"`
    substring left to find. Detected via the side effect Vector's decoder leaves
    behind (`.appname == "CEF"`) combined with the message still being CEF-shaped.
  - **Not handled**: CEF extension values containing a space (e.g. a device or
    client name like `"Townsville CGU"` or `"Victor's iPhone"`) get truncated at
    the first space, since `parse_key_value`'s space field-delimiter can't
    distinguish a space inside a value from the delimiter between key=value pairs.
    Confirmed against real captured examples. A future pass would need a
    different extension-parsing approach to handle this correctly.
