# siem-ingest Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the Vector syslog-ingest pipeline (UniFi + generic hosts,
parsing, geo/threat-intel enrichment, fast-path to siem-api), a new
source-heartbeat mechanism that closes siem-api's deferred absence-rule gap,
and the deployment/setup deliverables.

**Architecture:** Vector receives syslog on three ports, parses per device
family, enriches, and fans out to Loki (everything), siem-api's fast-path
endpoint (high-signal events only), and a new heartbeat endpoint (throttled
to one event per source per interval). A local Docker-based test harness
(Vector + Loki + a stub HTTP receiver) verifies each pipeline stage against
synthetic syslog traffic, since there's no compiler or unit-test framework
for a TOML pipeline config.

**Tech Stack:** Vector 0.49 (TOML config), Go (the one siem-api addition),
Docker Compose (test harness + deployment stack).

## Global Constraints

- **Two worktrees.** Task 1 runs in
  `/Users/hibikipr/Documents/GitHub/homeSIEM/.claude/worktrees/siem-api-implementation/siem-api`
  (Go, branch `worktree-siem-api-implementation`, the still-open PR #1).
  Tasks 2–8 run in
  `/Users/hibikipr/Documents/GitHub/homeSIEM/.claude/worktrees/siem-ingest-implementation`
  (new branch `worktree-siem-ingest-implementation`, no PR yet). Every task
  states which one.
- The reference `vector.toml`/`docker-compose.yml` under
  `design_handoff_homesiem/reference/` are authoritative starting points —
  implement at high fidelity, deviating only where something doesn't
  actually work or where this plan explicitly says to extend them (the
  heartbeat mechanism).
- Label discipline is non-negotiable: only `job`, `source`, `host`,
  `program`, `severity`, `facility` may ever become a Loki label. `src_ip`,
  `dst_port`, `rule`, `geoip.*`, `threat_intel` are structured fields on the
  event, never labels.
- No real GeoIP or threat-intel data is fetched or embedded in this repo.
  Local verification uses MaxMind's own publicly-published test-only
  database (`test-data/GeoLite2-City-Test.mmdb` from
  `github.com/maxmind/MaxMind-DB`, fetched fresh by the test harness script,
  never committed) plus a header-only placeholder `threatlist.csv` — neither
  is real geolocation/threat data.
- The deployment stack (`docker-compose.yml`) stays inside the homeSIEM repo
  as a deliverable. Nothing in this plan writes to the separate, live
  `homelab` repo.
- After Task 1 (the only siem-api change), push to `origin` to update the
  still-open PR #1 — same precedent as every prior siem-api addition in this
  project.
- No AI attribution in commit messages.
- Most of this plan's VRL/TOML syntax and field names (the `throttle`
  transform, the `http` sink's custom request headers, `.source_ip` on a
  `syslog` source, the `parse_key_value` bug in `parse_unifi` and its
  regex-based fix, the `exists()`-vs-`is_null()` bug in the `fast_path`
  condition and its fix, the `--config-toml` flag requirement) were verified
  directly against the real `timberio/vector:0.49.0-alpine` binary before
  this plan was written, not assumed from documentation. Two things remain
  genuinely open, each with a concrete verification step rather than a
  guess: which field distinguishes `hosts_tcp` from `hosts_tls` traffic at
  runtime (Task 3, Step 3); and an intermittent-seeming fast-path leak
  reproduced only with the full `geolite`+`threatlist`+`heartbeat_throttle`+
  `heartbeat_shape` topology together, whose root cause substantial direct
  testing did not pin down (Task 4 Step 2 and Task 5 Step 3a) — flagged
  honestly as unresolved rather than papered over, with a bisection method
  handed forward instead of a fix.

---

### Task 1: siem-api — `POST /sources/heartbeat`

**Worktree:** `siem-api-implementation/siem-api`

**Files:**
- Modify: `internal/api/sources.go`
- Modify: `internal/api/server.go`
- Test: `internal/api/sources_test.go` (add to existing file)

**Interfaces:**
- Consumes: `Store.UpsertSource(ctx, Source) (Source, error)` and
  `Store.TouchSourceLastSeen(ctx, name string, at time.Time) error` — both
  already exist and are already tested; this task only adds the HTTP layer.
- Produces: `POST /sources/heartbeat` (`202 Accepted` on success) —
  consumed by Task 5's Vector heartbeat sink.

Mirrors `handleFastpath`'s existing unauthenticated-but-token-gated pattern
exactly (`internal/api/fastpath.go:22-26`): checked directly in the handler
via `X-Fastpath-Token`, not through the RBAC `protect()` middleware, and
registered with a plain `s.mux.HandleFunc` the same way
`POST /ingest/fastpath` is (`internal/api/server.go:51`).

- [ ] **Step 1: Write the failing tests**

Add to `internal/api/sources_test.go`:
```go
func TestSourceHeartbeat_InvalidToken(t *testing.T) {
	s, _ := newTestServer(t)
	body := strings.NewReader(`{"name":"udm-ultra","address":"10.0.0.1","transport":"udp/514","parser":"unifi-os"}`)
	req := httptest.NewRequest(http.MethodPost, "/sources/heartbeat", body)
	req.Header.Set("X-Fastpath-Token", "wrong-token")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestSourceHeartbeat_RegistersNewSourceAndBumpsLastSeen(t *testing.T) {
	s, st := newTestServer(t)
	ctx := context.Background()

	body := strings.NewReader(`{"name":"udm-ultra","address":"10.0.0.1","transport":"udp/514","parser":"unifi-os"}`)
	req := httptest.NewRequest(http.MethodPost, "/sources/heartbeat", body)
	req.Header.Set("X-Fastpath-Token", "test-fastpath-token")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202, body=%s", rec.Code, rec.Body.String())
	}

	sources, err := st.ListSources(ctx)
	if err != nil {
		t.Fatalf("ListSources() error = %v", err)
	}
	if len(sources) != 1 {
		t.Fatalf("len(sources) = %d, want 1", len(sources))
	}
	got := sources[0]
	if got.Name != "udm-ultra" || got.Address != "10.0.0.1" || got.Transport != "udp/514" || got.Parser != "unifi-os" {
		t.Errorf("source = %+v, want name/address/transport/parser to match the heartbeat body", got)
	}
	if got.Claimed {
		t.Error("Claimed = true, want a newly-registered source to start unclaimed")
	}
	if got.LastSeenAt == nil {
		t.Error("LastSeenAt = nil, want it set by the heartbeat")
	}
}

func TestSourceHeartbeat_ExistingSourceUpdatesLastSeen(t *testing.T) {
	s, st := newTestServer(t)
	ctx := context.Background()
	created, err := st.UpsertSource(ctx, store.Source{
		Name: "udm-ultra", Address: "10.0.0.1", Transport: "udp/514", Parser: "unifi-os", HeartbeatSec: 900,
	})
	if err != nil {
		t.Fatalf("UpsertSource() error = %v", err)
	}
	if created.LastSeenAt != nil {
		t.Fatal("test setup: expected LastSeenAt nil before any heartbeat")
	}

	body := strings.NewReader(`{"name":"udm-ultra","address":"10.0.0.1","transport":"udp/514","parser":"unifi-os"}`)
	req := httptest.NewRequest(http.MethodPost, "/sources/heartbeat", body)
	req.Header.Set("X-Fastpath-Token", "test-fastpath-token")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202, body=%s", rec.Code, rec.Body.String())
	}

	sources, err := st.ListSources(ctx)
	if err != nil {
		t.Fatalf("ListSources() error = %v", err)
	}
	if len(sources) != 1 {
		t.Fatalf("len(sources) = %d, want 1 (heartbeat must not duplicate an existing source)", len(sources))
	}
	if sources[0].LastSeenAt == nil {
		t.Error("LastSeenAt = nil, want the heartbeat to have bumped it")
	}
}

func TestSourceHeartbeat_InvalidJSON(t *testing.T) {
	s, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/sources/heartbeat", strings.NewReader("not json"))
	req.Header.Set("X-Fastpath-Token", "test-fastpath-token")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}
```

This will need `"strings"` added to the test file's imports if not already
present — check the existing import block in `internal/api/sources_test.go`
before adding.

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./internal/api/... -run TestSourceHeartbeat -v`
Expected: FAIL — the route and handler don't exist yet.

- [ ] **Step 3: Write the handler**

Add to `internal/api/sources.go`:
```go
type sourceHeartbeatRequest struct {
	Name      string `json:"name"`
	Address   string `json:"address"`
	Transport string `json:"transport"`
	Parser    string `json:"parser"`
}

func (s *Server) handleSourceHeartbeat(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("X-Fastpath-Token") != s.deps.FastpathToken || s.deps.FastpathToken == "" {
		http.Error(w, "invalid fastpath token", http.StatusUnauthorized)
		return
	}

	var req sourceHeartbeatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	// heartbeat_sec: no UI exists yet to let an admin customize this per
	// source, so every heartbeat call passes the schema's own default.
	// UpsertSource always overwrites it — harmless today since nothing sets
	// it to anything else, but whoever builds the Sources screen's "edit
	// heartbeat interval" feature will need to read-then-preserve here
	// instead of always passing this constant.
	const defaultHeartbeatSec = 900

	if _, err := s.deps.Store.UpsertSource(ctx, store.Source{
		Name: req.Name, Address: req.Address, Transport: req.Transport,
		Parser: req.Parser, HeartbeatSec: defaultHeartbeatSec,
	}); err != nil {
		s.deps.Logger.Error("source heartbeat: upsert failed", "name", req.Name, "error", err)
		http.Error(w, "heartbeat failed", http.StatusInternalServerError)
		return
	}

	if err := s.deps.Store.TouchSourceLastSeen(ctx, req.Name, time.Now().UTC()); err != nil {
		s.deps.Logger.Error("source heartbeat: touch last_seen failed", "name", req.Name, "error", err)
		http.Error(w, "heartbeat failed", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusAccepted)
}
```

Add `"time"` to `internal/api/sources.go`'s imports if not already present
(check first — `sourceResponse`'s `LastSeenAt *time.Time` field means it
likely already imports `time`).

- [ ] **Step 4: Register the route**

In `internal/api/server.go`'s `routes()`, add right after the existing
`POST /ingest/fastpath` line:
```go
	s.mux.HandleFunc("POST /sources/heartbeat", s.handleSourceHeartbeat)
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/api/... -v`
Expected: PASS, including all 4 new tests, no regressions.

- [ ] **Step 6: Run the full suite**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: clean build, no vet issues, all tests passing across every
package.

- [ ] **Step 7: Commit and push**

```bash
git add internal/api/sources.go internal/api/server.go internal/api/sources_test.go
git commit -m "Add POST /sources/heartbeat: register/touch sources from Vector"
git push
```

---

### Task 2: siem-ingest — local test harness scaffold

**Worktree:** `siem-ingest-implementation` (repo root)

**Files:**
- Create: `siem-ingest/test/docker-compose.yml`
- Create: `siem-ingest/test/stub-receiver.py`
- Create: `siem-ingest/test/fetch-test-fixtures.sh`
- Create: `siem-ingest/test/threatlist.csv`
- Create: `siem-ingest/test/send-test-traffic.sh`

**Interfaces:**
- Produces: a runnable local stack (`docker compose -f
  siem-ingest/test/docker-compose.yml up`) with real Vector, real Loki, and
  a stub HTTP receiver standing in for siem-api's `/ingest/fastpath` and
  `/sources/heartbeat` — consumed by every later task's verification steps.

No TDD here (this task builds the test harness itself, not application
logic) — verification is running the harness and confirming its pieces come
up healthy.

- [ ] **Step 1: Write the stub receiver**

`siem-ingest/test/stub-receiver.py` (stdlib only, no dependencies — this
just needs to run inside a plain `python:3-alpine` container):
```python
#!/usr/bin/env python3
"""Stub HTTP receiver standing in for siem-api's /ingest/fastpath and
/sources/heartbeat during local pipeline verification. Logs every request's
path, headers, and JSON body to stdout as one line of JSON, so a test script
can grep the container's logs to assert on what Vector actually sent."""
import json
import sys
from http.server import BaseHTTPRequestHandler, HTTPServer


class Handler(BaseHTTPRequestHandler):
    def do_POST(self):
        length = int(self.headers.get("Content-Length", 0))
        body = self.rfile.read(length) if length else b""
        try:
            parsed_body = json.loads(body) if body else None
        except json.JSONDecodeError:
            parsed_body = None
        record = {
            "path": self.path,
            "fastpath_token": self.headers.get("X-Fastpath-Token"),
            "body": parsed_body,
        }
        print(json.dumps(record), flush=True)
        self.send_response(202)
        self.end_headers()

    def log_message(self, format, *args):
        pass  # suppress default request logging; the JSON line above is what matters


if __name__ == "__main__":
    port = int(sys.argv[1]) if len(sys.argv) > 1 else 8080
    HTTPServer(("0.0.0.0", port), Handler).serve_forever()
```

- [ ] **Step 2: Write the fixture-fetch script**

`siem-ingest/test/fetch-test-fixtures.sh`:
```bash
#!/usr/bin/env bash
set -euo pipefail
# Fetches MaxMind's own publicly-published TEST-ONLY database (not real
# geolocation data — explicitly published by MaxMind for integration
# testing without a license) into a gitignored local fixtures directory.
# Never commit the downloaded file; re-run this script whenever the
# fixtures directory is missing.
cd "$(dirname "$0")"
mkdir -p fixtures
curl -fsSL \
  "https://raw.githubusercontent.com/maxmind/MaxMind-DB/main/test-data/GeoLite2-City-Test.mmdb" \
  -o fixtures/GeoLite2-City-Test.mmdb
echo "Fetched fixtures/GeoLite2-City-Test.mmdb ($(wc -c < fixtures/GeoLite2-City-Test.mmdb) bytes)"
```

- [ ] **Step 3: Write the placeholder threat list**

`siem-ingest/test/threatlist.csv` (header-only — schema-valid, zero real
threat-intel entries, matching the reference config's
`enrichment_tables.threatlist.schema`):
```csv
ip,tag
```

- [ ] **Step 4: Write the docker-compose test harness**

`siem-ingest/test/docker-compose.yml`:
```yaml
version: "3.9"

services:
  loki:
    image: grafana/loki:3.0.0
    command: -config.file=/etc/loki/local-config.yaml
    ports:
      - "3100:3100"

  stub-receiver:
    image: python:3-alpine
    working_dir: /app
    volumes:
      - ./stub-receiver.py:/app/stub-receiver.py:ro
    command: ["python3", "stub-receiver.py", "8080"]
    ports:
      - "8081:8080"

  vector:
    image: timberio/vector:0.49.0-alpine
    # Without this, the image silently falls back to its own bundled demo
    # config at /etc/vector/vector.yaml (verified empirically: it starts up
    # cleanly and streams fake movie-quote log lines, giving no indication
    # the mounted vector.toml below was never read). Confirmed this exact
    # flag against the real image before writing this task.
    command: ["--config-toml", "/etc/vector/vector.toml"]
    depends_on:
      - loki
      - stub-receiver
    environment:
      - LOKI_ENDPOINT=http://loki:3100
      - SIEM_API_URL=http://stub-receiver:8080
      - SIEM_FASTPATH_TOKEN=test-fastpath-token
    volumes:
      - ../vector.toml:/etc/vector/vector.toml:ro
      - ./fixtures:/geoip:ro
      - ./threatlist.csv:/geoip/threatlist.csv:ro
    ports:
      - "514:514/udp"
      - "601:601/tcp"
      - "6514:6514/tcp"
      - "8686:8686"
```

Note: this mounts `../vector.toml` — the real production config Tasks 3-5
build — directly into the test harness, so verification always runs against
the actual deliverable, not a separate test-only copy that could drift.
`fixtures/GeoLite2-City-Test.mmdb` is mounted at the same path
(`/geoip/GeoLite2-City-Test.mmdb`, matching the fetch script's output
filename) — `vector.toml`'s enrichment table path must reference
`GeoLite2-City-Test.mmdb` during local testing; Task 4 documents pointing
this at the real `GeoLite2-City.mmdb` filename for production via the
setup docs.

- [ ] **Step 5: Write the synthetic-traffic sender**

`siem-ingest/test/send-test-traffic.sh`:
```bash
#!/usr/bin/env bash
set -euo pipefail
# Sends synthetic syslog lines shaped like real UniFi/RFC5424 output at the
# harness's three ports. Requires `logger` (util-linux/bsdutils, present on
# both macOS and Linux) and `ncat` or `nc`.
UNIFI_HOST="${1:-127.0.0.1}"
UNIFI_PORT="${2:-514}"
HOSTS_TCP_PORT="${3:-601}"

# OUT= is deliberately empty here — the real, common shape for a blocked
# inbound connection (no egress interface). Task 3 found and fixed a real
# parsing bug specifically triggered by this empty-value case; keep this
# line shaped exactly this way so it continues to exercise that fix.
echo '<134>Jan 1 00:00:00 UDM-Ultra kernel: [WAN_LOCAL-1000-D] IN=eth0 OUT= SRC=203.0.113.7 DST=10.0.0.1 PROTO=TCP SPT=443 DPT=22' \
  | ncat -u -w1 "$UNIFI_HOST" "$UNIFI_PORT"

echo '<134>1 2026-08-03T00:00:00Z test-host-1 sshd 1234 - - Failed password for root from 198.51.100.5 port 4242 ssh2' \
  | ncat -w1 "$UNIFI_HOST" "$HOSTS_TCP_PORT"

echo "Sent 2 synthetic syslog lines (UniFi UDP/$UNIFI_PORT, host TCP/$HOSTS_TCP_PORT)."
```

`203.0.113.7` and `198.51.100.5` are both IANA-reserved TEST-NET addresses
(RFC 5737) — safe to use in synthetic traffic without resembling any real
address, and specifically chosen so a later task's threat-intel test case
can distinguish "flagged" vs "not flagged" IPs without ambiguity.

- [ ] **Step 6: Make the shell scripts executable and verify the harness boots**

Run:
```bash
chmod +x siem-ingest/test/fetch-test-fixtures.sh siem-ingest/test/send-test-traffic.sh
siem-ingest/test/fetch-test-fixtures.sh
touch siem-ingest/vector.toml  # placeholder so compose can mount it; Task 3 writes real content
cd siem-ingest/test && docker compose up -d loki stub-receiver
docker compose ps
```
Expected: both `loki` and `stub-receiver` show as running/healthy. (Don't
start `vector` yet — its config is still empty; Task 3 does that.) Then:
```bash
docker compose down
```

- [ ] **Step 7: Gitignore the fetched fixture and commit everything else**

Add to `siem-ingest/.gitignore` (create the file):
```
test/fixtures/
```

```bash
cd siem-ingest-implementation  # repo root
git add siem-ingest/test/docker-compose.yml siem-ingest/test/stub-receiver.py \
  siem-ingest/test/fetch-test-fixtures.sh siem-ingest/test/threatlist.csv \
  siem-ingest/test/send-test-traffic.sh siem-ingest/.gitignore
git commit -m "Add siem-ingest local test harness: Vector + Loki + stub receiver"
```

Do not commit the placeholder empty `siem-ingest/vector.toml` from Step 6 —
`git status` should show it as untracked; leave it in place (Task 3
overwrites it with real content) but unstaged.

---

### Task 3: siem-ingest — sources, parsing, Loki sink

**Worktree:** `siem-ingest-implementation` (repo root)

**Files:**
- Create/overwrite: `siem-ingest/vector.toml`

**Interfaces:**
- Produces: the `unifi`/`hosts_tcp`/`hosts_tls` sources and
  `parse_unifi`/`parse_hosts` transforms — consumed by Task 4's enrichment
  and Task 5's heartbeat, both of which read the `.transport`/`.parser`
  fields this task adds.

This is the first of three tasks that incrementally build one `vector.toml`.
Verification is real: bring up the harness, send synthetic traffic, query
Loki directly.

**A real bug already found and fixed here, ahead of writing the task** (via
direct testing against the real `timberio/vector:0.49.0-alpine` binary
before drafting this plan, not assumed): the reference config's
`parse_unifi` extracts fields with `parse_key_value!(.message,
field_delimiter: " ", key_value_delimiter: "=")`. Tested against a real
UniFi-shaped log line with `OUT=` empty (the normal, common case for
WAN-inbound-blocked traffic — there's no egress interface for a dropped
inbound packet), `parse_key_value` swallows the *next* space-delimited
token into the empty value instead of stopping at the delimiter — producing
`{"OUT": "SRC=203.0.113.7", ...}` with **no `SRC` key at all**. `src_ip`
then resolves to `null` silently (no VRL error at that transform), which
breaks `enrich_geo` downstream (confirmed: `ip_cidr_contains` errors on a
null input) — a total, silent loss of geo/threat-intel enrichment for
exactly the traffic class (blocked inbound connections) this pipeline
exists to catch. The plan below uses field-by-field regex extraction
instead, verified against real blocked (`OUT=` empty) and accepted
(`OUT=` non-empty) UniFi log shapes.

- [ ] **Step 1: Write the sources, transforms, and Loki sink**

`siem-ingest/vector.toml`:
```toml
# ${MY_DOCKER_DATA_DIR}/homesiem/vector/vector.toml
#
# One source + transform pair per device family; the sink and label set
# are shared. The job label is what keeps this feed distinct from the
# alloy and unpoller feeds already in the same Loki tenant.

[api]
enabled = true
address = "0.0.0.0:8686"

# ---------------------------------------------------------------- sources

[sources.unifi]
type = "syslog"
address = "0.0.0.0:514"
mode = "udp"
receive_buffer_bytes = 8388608   # needs net.core.rmem_max raised on the host

[sources.hosts_tcp]
type = "syslog"
address = "0.0.0.0:601"
mode = "tcp"

[sources.hosts_tls]
type = "syslog"
address = "0.0.0.0:6514"
mode = "tcp"
[sources.hosts_tls.tls]
enabled = true
crt_file = "/etc/vector/tls/server.crt"
key_file = "/etc/vector/tls/server.key"
verify_certificate = true

# ------------------------------------------------------------- transforms

[transforms.parse_unifi]
type = "remap"
inputs = ["unifi"]
source = '''
  .job = "siem"
  .source = "udm-ultra"
  .transport = "udp/514"
  .parser = "unifi-os"
  if match(string!(.message), r'\[(?P<rule>[A-Z_]+-\w+-[ADR])\]') {
    .rule     = parse_regex!(.message, r'\[(?P<rule>[A-Z_]+-\w+-[ADR])\]').rule
    .src_ip   = parse_regex(.message, r'(?:^| )SRC=(?P<val>[^ ]+)').val ?? null
    .dst_ip   = parse_regex(.message, r'(?:^| )DST=(?P<val>[^ ]+)').val ?? null
    dpt       = parse_regex(.message, r'(?:^| )DPT=(?P<val>[^ ]+)').val ?? null
    .dst_port = to_int(dpt) ?? null
    proto     = parse_regex(.message, r'(?:^| )PROTO=(?P<val>[^ ]+)').val ?? ""
    .proto    = downcase(proto)
    .action   = if ends_with(.rule, "-D") { "drop" } else { "accept" }
  }
'''

[transforms.parse_hosts]
type = "remap"
inputs = ["hosts_tcp", "hosts_tls"]
source = '''
  .job = "siem"
  .source = downcase(string!(.host))
  .transport = "tcp/601"
  .parser = "rfc5424"
'''

# ------------------------------------------------------------------ sinks

[sinks.loki]
type = "loki"
inputs = ["parse_unifi", "parse_hosts"]
endpoint = "${LOKI_ENDPOINT}"
out_of_order_action = "accept"

[sinks.loki.labels]
job = "siem"
source = "{{ source }}"
host = "{{ host }}"
program = "{{ appname }}"
severity = "{{ severity }}"

[sinks.loki.buffer]
type = "disk"
max_size = 536870912
```

Deviation from the reference worth noting: `.transport` is hardcoded
`"tcp/601"` in `parse_hosts` even though this transform also handles the
`hosts_tls` source (port 6514) — fix this in Step 3 below once you've
confirmed which field actually distinguishes the two inputs at runtime (see
the note there). This step intentionally ships that known gap so Step 2's
verification can demonstrate it, which is the point of the empirical check.

- [ ] **Step 2: Bring up the harness and send synthetic traffic**

```bash
cd siem-ingest/test
docker compose up -d
sleep 3
./send-test-traffic.sh
sleep 2
curl -s "http://localhost:3100/loki/api/v1/query_range?query={job=\"siem\"}" | python3 -m json.tool
```
Expected: two log streams, one with `source="udm-ultra"` (from the UniFi
UDP line) and one with `source="test-host-1"` (from the TCP host line),
both carrying only the five labels `job`/`source`/`host`/`program`/`severity`
— confirm no `src_ip`/`dst_port`/`transport`/`parser`/`rule` ever appears as
a Loki *label* (they should still be present in the structured log line's
JSON body if Vector's syslog source emits structured fields into the message
— check the actual query response to see how they surface, not just assume).

- [ ] **Step 3: Discover the real transport-distinguishing field, fix `parse_hosts`**

Temporarily add a debug sink to inspect the raw parsed event for a
`hosts_tls`-sourced connection specifically (TLS requires a cert Task 3
hasn't set up yet, so this can't be tested against port 6514 directly in
this harness pass — instead, inspect Vector's own source documentation/
`vector top` output, or check whether `hosts_tcp` vs `hosts_tls` inputs can
be distinguished via a Vector-provided source-identifying field such as
`.source_type` — add this to `parse_hosts`'s remap logic once confirmed:
```vrl
  .transport = if .source_type == "hosts_tls" { "tls/6514" } else { "tcp/601" }
```
If `.source_type` isn't populated the way expected, split `parse_hosts` into
two separate transforms (`parse_hosts_tcp` inputting `["hosts_tcp"]` with
`.transport = "tcp/601"`, `parse_hosts_tls` inputting `["hosts_tls"]` with
`.transport = "tls/6514"`) feeding a shared downstream transform — this is
the safe fallback if Vector doesn't expose a distinguishing field on the
event itself. Update the Loki sink's `inputs` list accordingly if you split
it. Document in the commit message which approach you used and why.

- [ ] **Step 4: Run `vector validate` and re-verify with traffic**

```bash
cd siem-ingest/test
docker compose run --rm vector validate --config-toml /etc/vector/vector.toml
docker compose up -d
sleep 3
./send-test-traffic.sh
sleep 2
curl -s "http://localhost:3100/loki/api/v1/query_range?query={job=\"siem\"}" | python3 -m json.tool
docker compose down
```
Expected: `vector validate` reports the config valid; the Loki query still
shows both streams correctly labeled after the Step 3 fix.

- [ ] **Step 5: Commit**

```bash
cd siem-ingest-implementation  # repo root
git add siem-ingest/vector.toml
git commit -m "Add siem-ingest Vector sources, parsing, and Loki sink"
```

---

### Task 4: siem-ingest — enrichment and fast-path

**Worktree:** `siem-ingest-implementation` (repo root)

**Files:**
- Modify: `siem-ingest/vector.toml`

**Interfaces:**
- Consumes: `parse_unifi`/`parse_hosts` outputs (Task 3).
- Produces: `enrich_geo` (adds `.geoip`/`.threat_intel`) — consumed by
  Task 5's heartbeat transform, which reads from this same enriched stream.

**A second real bug already found and fixed here** (same direct-testing
process as Task 3's `parse_key_value` finding): the reference's `fast_path`
condition uses `exists(.threat_intel)`. `enrich_geo` always assigns the
`.threat_intel` key (even to literal `null`, via `?? null`) for every event
with a public `src_ip` — and `exists()` checks key *presence*, not whether
the value is non-null. So `exists(.threat_intel)` is `true` for every
enriched event, public IP or not, threat hit or not — meaning the
reference's fast-path filter would forward **every single enriched
event** to siem-api, not just real threat-intel hits, defeating the entire
point of a "fast path" and likely flooding siem-api with false alerts for
ordinary traffic. Fixed below with `!is_null(.threat_intel)`, which
correctly distinguishes "key present with a real tag" from "key present but
null."

- [ ] **Step 1: Add enrichment tables, the enrichment transform, the fast-path filter, and the siem-api fastpath sink**

Add to `siem-ingest/vector.toml`, after the transforms and before the Loki
sink:
```toml
[transforms.enrich_geo]
type = "remap"
inputs = ["parse_unifi", "parse_hosts"]
source = '''
  if exists(.src_ip) && !ip_cidr_contains!("10.0.0.0/8", .src_ip) {
    .geoip = get_enrichment_table_record("geolite", { "ip": .src_ip }) ?? {}
    .threat_intel = get_enrichment_table_record("threatlist", { "ip": .src_ip }).tag ?? null
  }
'''

[transforms.fast_path]
type = "filter"
inputs = ["enrich_geo"]
condition = '''
  !is_null(.threat_intel) || (.action == "drop" && !is_null(.dst_port))
'''

[enrichment_tables.geolite]
type = "geoip"
path = "/geoip/GeoLite2-City-Test.mmdb"

[enrichment_tables.threatlist]
type = "file"
[enrichment_tables.threatlist.file]
path = "/geoip/threatlist.csv"
encoding = { type = "csv" }
[enrichment_tables.threatlist.schema]
ip = "string"
tag = "string"

[sinks.siem_api]
type = "http"
inputs = ["fast_path"]
uri = "${SIEM_API_URL}/ingest/fastpath"
method = "post"
encoding.codec = "json"
[sinks.siem_api.request.headers]
X-Fastpath-Token = "${SIEM_FASTPATH_TOKEN}"
[sinks.siem_api.buffer]
type = "memory"
max_events = 1000
when_full = "drop_newest"
```

Change the Loki sink's `inputs` from `["parse_unifi", "parse_hosts"]` to
`["enrich_geo"]`, so Loki also receives the enriched fields (still only as
structured data on the event, not as new labels — the `[sinks.loki.labels]`
block from Task 3 is unchanged and doesn't reference `geoip`/`threat_intel`
anywhere).

`enrichment_tables.geolite.path` points at the TEST fixture filename
(`GeoLite2-City-Test.mmdb`) for this local-harness pass — Task 7's setup
docs tell the real deployment to point this at the real
`GeoLite2-City.mmdb` instead.

- [ ] **Step 2: Bring up the harness and verify fast-path filtering**

```bash
cd siem-ingest/test
docker compose up -d
sleep 3
./send-test-traffic.sh
sleep 2
docker compose logs stub-receiver
```
Expected: the UniFi line (`SRC=203.0.113.7 ... [WAN_LOCAL-1000-D]`, a `-D`
suffix meaning `action=drop`, with a numeric `dst_port`) produces a
`POST /ingest/fastpath` line in the stub receiver's log, with the
`X-Fastpath-Token: test-fastpath-token` header present and `body.action ==
"drop"`. The host-syslog SSH line should NOT produce any fastpath POST
(no drop action, no threat-intel hit, since `198.51.100.5` isn't in the
empty `threatlist.csv`).

**Important — also explicitly test an accepted (non-dropped) UniFi event,
not just the two cases above.** Send a second, accept-shape UniFi line —
same rule-name pattern but ending in `-A` instead of `-D` (e.g.
`[LAN_IN-1000-A] IN=eth1 OUT=eth0 SRC=192.168.1.50 DST=8.8.8.8 PROTO=UDP
SPT=51000 DPT=53`) — through the same harness and confirm it produces **no**
fastpath POST. This specific check matters more than it looks: while
building this plan, sending an accept-shape event through a config
structurally identical to this task's (enrich_geo with both `geolite` and
`threatlist` tables, feeding both `fast_path` and, once Task 5 adds it,
`heartbeat_throttle`/`heartbeat_shape`) sometimes let the accept-case event
through anyway — reproducible with that exact combination of components
present, but NOT reproducible in any simpler sub-combination tried (fewer
enrichment tables, or `fast_path` alone without a sibling
`heartbeat_throttle` consumer). The root cause was not pinned down despite
substantial isolation effort. If this check fails for you: bisect by
temporarily removing sibling consumers of `enrich_geo` one at a time
(comment out `heartbeat_throttle`/`heartbeat_shape` if Task 5's additions
are already present, or the `geolite` enrichment table) and re-testing
after each removal to find what specifically triggers it in your build,
rather than assuming the condition logic itself (verified correct in
isolation, multiple times) is wrong. Do not consider this task done until
this specific accept-case check passes reliably (run it twice).

- [ ] **Step 3: Verify a threat-intel hit reaches fast-path**

Temporarily add a row to `siem-ingest/test/threatlist.csv`:
```csv
ip,tag
203.0.113.7,test-known-scanner
```
Re-run:
```bash
cd siem-ingest/test
docker compose restart vector
sleep 2
./send-test-traffic.sh
sleep 2
docker compose logs stub-receiver
docker compose down
```
Expected: the fastpath POST for the UniFi line now shows
`body.threat_intel` present (not just the drop-action path). Revert
`threatlist.csv` back to header-only afterward — Step 3 of Task 2 already
established that as the committed placeholder state; don't leave the test
row committed.

- [ ] **Step 4: Run `vector validate` and confirm the label discipline holds**

```bash
cd siem-ingest/test
docker compose run --rm vector validate --config-toml /etc/vector/vector.toml
docker compose up -d
sleep 3
./send-test-traffic.sh
sleep 2
curl -s "http://localhost:3100/loki/api/v1/query_range?query={job=\"siem\"}" | python3 -m json.tool
docker compose down
```
Expected: config valid; Loki entries still carry only the five allowed
labels even though the underlying events now carry `geoip`/`threat_intel`
structured fields.

- [ ] **Step 5: Commit**

```bash
cd siem-ingest-implementation  # repo root
git add siem-ingest/vector.toml siem-ingest/test/threatlist.csv
git commit -m "Add siem-ingest geo/threat-intel enrichment and fast-path sink"
```

---

### Task 5: siem-ingest — heartbeat

**Worktree:** `siem-ingest-implementation` (repo root)

**Files:**
- Modify: `siem-ingest/vector.toml`

**Interfaces:**
- Consumes: `enrich_geo`'s output (Task 4) — heartbeats fire from the full
  enriched stream, not just fast-path, so a quiet-but-alive source still
  proves it's alive.
- Produces: `POST /sources/heartbeat` calls — consumed by Task 1's
  siem-api endpoint (already built and pushed).

- [ ] **Step 1: Add the throttle transform, reshape transform, and heartbeat sink**

Add to `siem-ingest/vector.toml`:
```toml
[transforms.heartbeat_throttle]
type = "throttle"
inputs = ["enrich_geo"]
threshold = 1
window_secs = 900
key_field = "{{ source }}"

[transforms.heartbeat_shape]
type = "remap"
inputs = ["heartbeat_throttle"]
source = '''
  . = {
    "name": .source,
    "address": .source_ip,
    "transport": .transport,
    "parser": .parser
  }
'''

[sinks.siem_heartbeat]
type = "http"
inputs = ["heartbeat_shape"]
uri = "${SIEM_API_URL}/sources/heartbeat"
method = "post"
encoding.codec = "json"
[sinks.siem_heartbeat.request.headers]
X-Fastpath-Token = "${SIEM_FASTPATH_TOKEN}"
```

`.source_ip` in `heartbeat_shape` is **confirmed correct, not a guess** —
verified directly against the real `timberio/vector:0.49.0-alpine` binary
before writing this task: a real syslog UDP packet sent to a minimal
`syslog` source produces an event containing `"source_ip":"<peer address>"`
(the actual peer/gateway address Vector saw the connection from). Step 2
re-confirms this in the full pipeline context (after enrichment/throttling
are layered in) rather than re-deriving it from scratch.

- [ ] **Step 2: Verify the heartbeat body in the full pipeline**

```bash
cd siem-ingest/test
docker compose up -d
sleep 3
./send-test-traffic.sh
sleep 2
docker compose logs stub-receiver | grep sources/heartbeat
```
Expected: `address` is populated (non-null) in the logged heartbeat body
for both device families. If it's ever `null` for a specific source in
practice (e.g. a device family reached through a proxy/relay that changes
what Vector sees as the peer address), that's a real environment-specific
finding to note in the report, not a sign the field name itself is wrong.

- [ ] **Step 3: Verify throttling caps at one heartbeat per source per window**

```bash
cd siem-ingest/test
./send-test-traffic.sh
./send-test-traffic.sh
./send-test-traffic.sh
sleep 2
docker compose logs stub-receiver | grep -c sources/heartbeat
```
Expected: exactly 2 heartbeat POSTs total (one per distinct `source` value
across all three `send-test-traffic.sh` runs within the same 900s window),
not 6 — confirming `threshold = 1` per `window_secs` per `key_field` is
working as intended.

- [ ] **Step 3a: Re-verify Task 4's accept-case fast-path check now that the full topology exists**

This exact combination of components (`enrich_geo` with both `geolite` and
`threatlist` enrichment tables, feeding both `fast_path` and
`heartbeat_throttle`/`heartbeat_shape`) is the one flagged in Task 4, Step 2
as sometimes letting an accepted (non-dropped) event leak through
`fast_path` incorrectly — reproduced during this plan's own research with
this precise topology, not with any simpler sub-combination. Now that
Task 5 completes that topology, re-run Task 4's accept-case check
one more time:
```bash
cd siem-ingest/test
echo '<134>Jan 1 00:00:03 UDM-Ultra kernel: [LAN_IN-1000-A] IN=eth1 OUT=eth0 SRC=192.168.1.50 DST=8.8.8.8 PROTO=UDP SPT=51000 DPT=53' | ncat -u -w1 127.0.0.1 514
sleep 2
docker compose logs stub-receiver | grep ingest/fastpath
```
Expected: no output (this accept-shape event must not reach
`/ingest/fastpath`). If it does, bisect per Task 4 Step 2's instructions —
temporarily remove `heartbeat_throttle`/`heartbeat_shape` or the `geolite`
enrichment table one at a time and re-test after each removal — before
considering this task done. Do not ship with this leak present: it would
flood siem-api with a fastpath-priority alert for every accepted connection,
not just genuine threats.

- [ ] **Step 4: Verify the heartbeat actually reaches siem-api's real endpoint**

This uses the REAL siem-api binary (from Task 1, already built and merged)
instead of the stub, to confirm true end-to-end integration, not just that
Vector sends a well-shaped request:
```bash
# In a separate terminal, from siem-api-implementation/siem-api:
SIEM_FASTPATH_TOKEN=test-fastpath-token DATABASE_URL=sqlite:///tmp/siem-ingest-e2e.db \
  LOKI_URL=http://localhost:3100 go run ./cmd/siem-api &

# Back in siem-ingest/test, point Vector at the real siem-api instead of the stub:
docker compose stop vector
SIEM_API_URL_OVERRIDE=http://host.docker.internal:8080 docker compose up -d vector
./send-test-traffic.sh
sleep 2
sqlite3 /tmp/siem-ingest-e2e.db "SELECT name, address, transport, parser, last_seen_at FROM sources;"
```
Expected: two rows, `udm-ultra` and `test-host-1`, each with a non-null
`last_seen_at`. Kill the background `siem-api` process and remove
`/tmp/siem-ingest-e2e.db` afterward. (This step needs `SIEM_API_URL` in
`docker-compose.yml` overridden to reach the host's real siem-api rather
than the stub container — if the compose file doesn't support an env
override this way, temporarily edit `test/docker-compose.yml`'s `vector`
service `SIEM_API_URL` value directly for this one verification run, then
revert it back to `http://stub-receiver:8080` before committing.)

- [ ] **Step 5: Run `vector validate`, tear down, commit**

```bash
cd siem-ingest/test
docker compose run --rm vector validate --config-toml /etc/vector/vector.toml
docker compose down
git -C ../.. diff --stat siem-ingest/test/docker-compose.yml  # confirm no leftover SIEM_API_URL override
```
```bash
cd siem-ingest-implementation  # repo root
git add siem-ingest/vector.toml
git commit -m "Add siem-ingest source-heartbeat mechanism"
```

---

### Task 6: siem-ingest — deployment stack

**Worktree:** `siem-ingest-implementation` (repo root)

**Files:**
- Create: `siem-ingest/docker-compose.yml`
- Create: `siem-ingest/.env.example`

**Interfaces:**
- Consumes: nothing from earlier tasks directly — this is the production
  deployment artifact, independent of the local test harness in
  `siem-ingest/test/`.

No TDD (deployment artifact, not application logic) — verification is
`docker compose config` (validates YAML + variable interpolation) plus a
manual diff against the reference and `stacks/homelab-monitoring`'s
conventions.

- [ ] **Step 1: Write the deployment compose file**

`siem-ingest/docker-compose.yml` (the `siem-ingest` service block from the
reference `design_handoff_homesiem/reference/docker-compose.yml`, on its
own — the `siem-api`/`siem-web` service blocks belong to those services'
own deployment, not repeated here):
```yaml
version: "3.9"

networks:
  # Created by the root Portainer stack - reference, never re-create.
  backend:
    name: backend
    external: true

volumes:
  siem_buffer:
    driver: local
    name: siem_buffer

services:
  # Vector - syslog receiver, parser and enricher. The only service in the
  # homeSIEM stack that publishes host ports, because devices dial it directly.
  siem-ingest:
    container_name: siem-ingest
    image: timberio/vector:0.49.0-alpine
    # Required: without this, the image silently falls back to its own
    # bundled demo config instead of the mounted vector.toml below
    # (verified empirically — it starts up cleanly with no error, so this
    # is easy to miss without checking).
    command: ["--config-toml", "/etc/vector/vector.toml"]
    restart: unless-stopped
    networks:
      - backend
    ports:
      - "514:514/udp"
      - "601:601/tcp"
      - "6514:6514/tcp"
    expose:
      - "8686"
    environment:
      - TZ=${DOCKER_TZ:-America/New_York}
      - LOKI_ENDPOINT=http://loki:3100
      - SIEM_API_URL=http://siem-api:8080
      - SIEM_FASTPATH_TOKEN=${SIEM_FASTPATH_TOKEN}
    volumes:
      - ${MY_DOCKER_DATA_DIR}/homesiem/vector:/etc/vector:ro
      - ${MY_DOCKER_DATA_DIR}/homesiem/geoip:/geoip:ro
      - siem_buffer:/var/lib/vector
    tmpfs: /tmp
    security_opt:
      - no-new-privileges:true
    healthcheck:
      test: ["CMD-SHELL", "wget -q -O- http://127.0.0.1:8686/health || exit 1"]
      interval: 30s
      timeout: 5s
      retries: 3
    deploy:
      resources:
        limits:
          memory: 384m
          cpus: "1.5"

# End siem-ingest stack. Deploy alongside stacks/homelab-siem's siem-api and
# siem-web service blocks (see the design handoff's reference compose) and
# after stacks/homelab-monitoring (loki, ntfy must already exist).
```

Note the one deviation from the reference: `SIEM_FASTPATH_TOKEN` is added
as an explicit environment variable here (the reference config predates
this plan's heartbeat work and didn't wire it into the ingest service at
all — Vector needs it now to authenticate both the fastpath and heartbeat
sinks).

- [ ] **Step 2: Write the env example**

`siem-ingest/.env.example`:
```bash
# New variables for the siem-ingest deployment.
# DOCKER_TZ and MY_DOCKER_DATA_DIR are already defined by the
# homelab-monitoring stack.

SIEM_FASTPATH_TOKEN=          # shared secret between Vector and siem-api; generate with: openssl rand -hex 32
```

- [ ] **Step 3: Validate**

```bash
cd siem-ingest
DOCKER_TZ=America/New_York MY_DOCKER_DATA_DIR=/tmp SIEM_FASTPATH_TOKEN=x docker compose config > /dev/null
echo "docker compose config: OK"
```
Expected: no errors (confirms valid YAML and that every `${VAR}`
interpolation resolves).

- [ ] **Step 4: Commit**

```bash
git add siem-ingest/docker-compose.yml siem-ingest/.env.example
git commit -m "Add siem-ingest deployment stack definition"
```

---

### Task 7: siem-ingest — GeoIP and TLS setup docs

**Worktree:** `siem-ingest-implementation` (repo root)

**Files:**
- Create: `siem-ingest/docs/geoip-setup.md`
- Create: `siem-ingest/docs/tls-setup.md`

**Interfaces:** none — pure documentation.

- [ ] **Step 1: Write the GeoIP setup doc**

`siem-ingest/docs/geoip-setup.md`:
```markdown
# GeoIP and threat-intel data setup

Neither `GeoLite2-City.mmdb` nor `threatlist.csv` ships in this repo — no
real geolocation or threat-intelligence data is embedded anywhere in
homeSIEM. Provision both on the real deployment host.

## GeoLite2-City.mmdb

1. Create a free MaxMind account: https://www.maxmind.com/en/geolite2/signup
2. Generate a license key: account portal → "Manage License Keys" → "Generate new license key".
3. Download the GeoLite2 City database:
   ```bash
   curl -fsSL "https://download.maxmind.com/app/geoip_download?edition_id=GeoLite2-City&license_key=YOUR_LICENSE_KEY&suffix=tar.gz" \
     -o GeoLite2-City.tar.gz
   tar xzf GeoLite2-City.tar.gz --strip-components=1 --wildcards '*/GeoLite2-City.mmdb'
   ```
4. Place the resulting `GeoLite2-City.mmdb` at
   `${MY_DOCKER_DATA_DIR}/homesiem/geoip/GeoLite2-City.mmdb` on the
   deployment host — matching `vector.toml`'s
   `enrichment_tables.geolite.path` (update that path from the local test
   harness's `GeoLite2-City-Test.mmdb` filename to the real one when
   deploying).
5. MaxMind updates GeoLite2 databases roughly weekly. Consider a small cron
   job re-running steps 3-4 periodically — out of scope for this pass.

## threatlist.csv

Format (matches `vector.toml`'s `enrichment_tables.threatlist.schema`):
```csv
ip,tag
198.51.100.23,known-scanner
203.0.113.44,c2-server
```

No specific feed is prescribed here — pick a threat-intel source you trust
and are licensed to use (many free/open feeds exist; check each one's terms
before ingesting it), and write a small script to transform it into this
two-column CSV shape. Place the result at
`${MY_DOCKER_DATA_DIR}/homesiem/geoip/threatlist.csv`.
```

- [ ] **Step 2: Write the TLS setup doc**

`siem-ingest/docs/tls-setup.md`:
```markdown
# TLS setup for the syslog-TLS source (port 6514)

`hosts_tls` (Vector's TCP+TLS syslog source, port 6514) needs a server
certificate and key. No existing homelab certificate is directly reusable
here — nginx-proxy-manager's Let's Encrypt certificates are issued for
HTTP-routed hostnames, not exposed for arbitrary raw TCP services like this
one.

A self-signed certificate is appropriate: this is host-to-host syslog
forwarding on the internal `backend` Docker network, not a
publicly-verified TLS endpoint.

```bash
mkdir -p ${MY_DOCKER_DATA_DIR}/homesiem/vector/tls
openssl req -x509 -newkey rsa:2048 -nodes -days 3650 \
  -keyout ${MY_DOCKER_DATA_DIR}/homesiem/vector/tls/server.key \
  -out ${MY_DOCKER_DATA_DIR}/homesiem/vector/tls/server.crt \
  -subj "/CN=siem-ingest.internal"
```

`vector.toml`'s `sources.hosts_tls.tls` block expects these at
`/etc/vector/tls/server.crt` and `/etc/vector/tls/server.key` inside the
container — mount `${MY_DOCKER_DATA_DIR}/homesiem/vector/tls` there (or
place the cert/key directly under the existing
`${MY_DOCKER_DATA_DIR}/homesiem/vector` mount, in a `tls/` subdirectory, so
one volume mount covers both `vector.toml` and the TLS material).

Any host configured to forward syslog to port 6514 over TLS needs
`verify_certificate = true` (already set in `vector.toml`) satisfied on its
end — either trust this specific self-signed cert explicitly on the sending
host, or switch that host to the unencrypted TCP/601 source instead if its
syslog client can't be configured to trust a custom CA/cert.
```

- [ ] **Step 3: Commit**

```bash
git add siem-ingest/docs/geoip-setup.md siem-ingest/docs/tls-setup.md
git commit -m "Add siem-ingest GeoIP and TLS setup docs"
```

---

### Task 8: Final verification and README

**Worktree:** `siem-ingest-implementation` (repo root), Step 2 also touches
`siem-api-implementation/siem-api`

**Files:**
- Create: `siem-ingest/README.md`

No new application code — run the full pipeline end-to-end one more time
and document what shipped.

- [ ] **Step 1: Full end-to-end harness run**

```bash
cd siem-ingest/test
./fetch-test-fixtures.sh   # in case fixtures/ was cleaned between tasks
docker compose up -d
sleep 3
./send-test-traffic.sh
sleep 2
echo "--- Loki entries ---"
curl -s "http://localhost:3100/loki/api/v1/query_range?query={job=\"siem\"}" | python3 -m json.tool
echo "--- Fastpath/heartbeat calls received ---"
docker compose logs stub-receiver
docker compose down
```
Expected: both log streams present and correctly labeled, the UniFi drop
event reached `/ingest/fastpath`, and heartbeat calls reached
`/sources/heartbeat` for both sources — the same behavior each prior task
verified individually, now confirmed together in one run.

- [ ] **Step 2: Run the siem-api suite**

From `siem-api-implementation/siem-api`:
```bash
go build ./... && go vet ./... && go test ./...
```
Expected: clean — confirms Task 1 didn't regress anything else.

- [ ] **Step 3: Write the README**

`siem-ingest/README.md`:
```markdown
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
- `heartbeat_sec` (how long before a source is considered "silent") is
  hardcoded to the schema default on every heartbeat call — there's no UI
  yet to customize it per source (that belongs to the not-yet-built Sources
  screen).
- This pass verifies the pipeline against synthetic traffic in a local
  Docker harness, not against the real UDM-Ultra or real hosts — that's the
  next real-world verification step once deployed.
```

- [ ] **Step 4: Commit**

```bash
git add siem-ingest/README.md
git commit -m "Add siem-ingest README with pipeline overview and setup pointers"
```
