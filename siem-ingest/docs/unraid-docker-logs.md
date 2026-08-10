# Forwarding Unraid Community App logs

Unraid isn't a systemd/journald host, so the journald-driver +
host-rsyslog-bridge approach used for this deployment's own Debian-based
Docker host doesn't apply there. Two options exist instead.

## Option A: a Vector sidecar (recommended)

Run a small [Vector](https://vector.dev) container on the Unraid box itself,
using its `docker_logs` source to read every other container's stdout/stderr
straight from the Docker Engine API and forward it as syslog. This is the
same approach `siem-ingest` itself uses internally, just running on Unraid
instead of the SIEM host.

**Why this over the alternative:** it doesn't touch any other container's
own log driver, so every app's native "Logs" button in Unraid's Docker UI
keeps working. The tradeoff is running one extra container, and everything
funnels through a single `host` value (see below) rather than being
per-app.

### Setup

1. Install **Vector** from the Community Applications store.
2. Save [`unraid-vector.toml`](unraid-vector.toml) (this directory) to the
   array, e.g. `/mnt/user/appdata/vector/vector.toml`, and fill in the two
   placeholders:
   - `HOSTNAME` — whatever you want this box to show up as in Search/Sources
     (doesn't need to match Unraid's real hostname).
   - `SIEM_INGEST_HOST` — `siem-ingest`'s LAN IP (the same host `514`/`601`/
     `6514` are already published on for other syslog senders).
3. In the container's **Edit** page (toggle **Advanced View**), add two path
   mappings:
   - Container Path `/etc/vector/vector.toml` → Host Path (the file from
     step 2), **Access Mode: Read Only**.
   - Container Path `/var/run/docker.sock` → Host Path `/var/run/docker.sock`
     — required, this is how `docker_logs` reads every container's logs
     without touching their own log drivers.
4. Set **Extra Parameters** (or the template's Post Arguments field, if it
   exposes one) to:
   ```
   --config-toml /etc/vector/vector.toml
   ```
5. Start the container. Every other container's stdout/stderr on this box
   now forwards automatically — no per-app configuration, nothing to redo
   when you add a new Community App later.

### What the config does

- `sources.docker` (`docker_logs`) — tails every container via the Docker
  API.
- `transforms.to_syslog` — hand-builds an RFC5424 syslog line
  (`<PRI>VERSION TIMESTAMP HOSTNAME APP-NAME PROCID MSGID STRUCTURED-DATA
  MSG`) from each event. `stream` (stdout/stderr) picks the PRI's severity
  default (info/err) exactly like every other source in this pipeline —
  it's not the real severity, just the fallback `enrich_geo` uses if it
  can't detect a self-reported level in the message text (see the next
  section). `container_name` becomes the syslog APP-NAME, which lands as
  `program` in Loki — this is what tells apps apart in Search, since they
  all share one `host` value. Also replaces any real ANSI escape (ESC,
  `0x1B`) control bytes in the message with the literal text `#033` before
  framing it — `docker_logs` reads raw stdout/stderr straight from the
  Docker API, so a color-coded logger's real ESC bytes survive intact,
  unlike the journald+rsyslog bridge used for this deployment's own Docker
  host, where rsyslog's own RFC5424 output already re-escapes that byte
  into `#033` text. Every ANSI-aware branch in `enrich_geo`'s
  severity-detection cascade is built against that rsyslog-escaped form —
  without this, a color-coded logger's real errors and warnings land as
  whatever the stdout/stderr default implies, not their real level.
  Confirmed in production: manyfold's red-colored `ERROR` lines (real
  Redis connection failures) were landing as `severity=info` before this
  was added.
- `sinks.siem` — a `socket` sink in TCP mode, sending each formatted line
  newline-delimited to `siem-ingest`'s existing TCP/601 source
  (`sources.hosts_tcp` in `vector.toml`). Plaintext, not TLS — this is
  internal LAN traffic, and per `tls-setup.md`, anything that can't be
  configured to trust a custom CA (Docker/Vector's own syslog tooling can't,
  easily) should use 601 rather than fight port 6514's TLS.

Verified end-to-end against a real deployment: events land in Loki with
`transport="tcp/601"`, `source_ip` set to the Unraid box's real LAN IP,
`host` set to the configured `HOSTNAME`, and `program` set to each
container's name.

### New sources may need a new severity-detection branch

`enrich_geo`'s severity cascade in `vector.toml` only knows the log formats
it's already seen. A Community App logging in a format nothing else in this
pipeline uses yet will fall through to the stdout/stderr PRI default above —
usually fine for stdout (`info`), but every stderr line lands as `err`
regardless of its real level until a matching branch is added. This
happened during the first real deployment against Unraid: one app logged
via Python's `logging` module with its default asctime formatter (`YYYY-MM-DD
HH:MM:SS,mmm LEVEL [logger] message`), which none of the existing
Python-logging branches matched. Fixed by adding a new branch — see the
`## Python's logging module, asctime-with-millis formatter` comment in
`vector.toml` for the pattern to copy if you hit a similar gap with a
different app.

## Option B: Docker's built-in syslog driver, per container

Skip the sidecar and point individual containers straight at `siem-ingest`
via **Extra Parameters** on each one:

```
--log-driver=syslog --log-opt syslog-address=tcp://SIEM_INGEST_HOST:601 --log-opt tag="{{.Name}}" --log-opt syslog-format=rfc5424
```

Simpler for a single app, but replaces that container's log driver
entirely — Unraid's own "Logs" button for it stops working, since Unraid
reads the default json-file driver, not syslog. Fine for one or two apps
you specifically care about; Option A scales better if you want everything
centralized without losing per-app log viewing.
