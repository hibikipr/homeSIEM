# siem-ingest: UniFi CEF Parser Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the documented CEF-parsing gap in `siem-ingest/vector.toml`'s
`parse_unifi` transform with real parsing — per
`docs/superpowers/specs/2026-08-07-siem-ingest-unifi-cef-parser-design.md`.

**Architecture:** One transform edit (`parse_unifi` gains a content-detected CEF branch
alongside its existing netfilter-bracket branch) plus one new `vector test` unit-test
file. No other files in the pipeline change — `enrich_geo`'s existing severity-default/
timestamp-clamp fallbacks stay as the safety net for anything the new branch's 7-field
guard doesn't cover.

**Tech Stack:** VRL (Vector Remap Language), `vector test` (Vector's built-in TOML-based
unit-test framework — newly adopted by this repo in this plan).

## Global Constraints

- The exact VRL in Task 1 has **already been verified against the real
  `timberio/vector:0.49.0-alpine` binary** during design — every test case in Task 1's
  brief passed against it. Transcribe it exactly; do not "improve" or restructure it
  without re-verifying against the real binary, since several lines look like they could
  be simplified but aren't (see the inline comments explaining why each `??`/`!`/`if`
  shape is what it is — VRL's error-coalescing (`??`) vs null-safety semantics, and
  `else`/`else if` needing to stay on the same line as the preceding `}`, both bit this
  exact code during verification).
- `.severity` for CEF events comes from the CEF header's own severity field (mapped
  `0-3→info, 4-6→warning, 7-8→err, 9-10→crit`), not `enrich_geo`'s default — that default
  now only applies when the new branch's field-count guard fails (malformed/truncated
  CEF) or for genuinely non-CEF, non-PRI messages.
- `.host`/`.hostname` for CEF events come from `UNIFIdeviceName`, falling back to
  `UNIFIhost` only when `UNIFIdeviceName` is absent.
- Structured fields (`.src_ip`, `.dst_ip`, `.dst_port`, `.proto`) reuse the exact same
  names the netfilter branch already sets — `fast_path`, `enrich_geo`'s GeoIP/threat-intel
  lookup, and the Loki/siem-api consumers of these fields do not change.
- No CEF escape-sequence handling (`\|`), no `.action` set for CEF events — both
  explicitly out of scope per the design spec.
- The pre-existing netfilter-bracket branch's behavior must not change.

---

### Task 1: siem-ingest — CEF parsing branch + `vector test` unit tests

**Files:**
- Modify: `siem-ingest/vector.toml`
- Create: `siem-ingest/test/parse_unifi.tests.toml`

**Interfaces:**
- Produces: for CEF-shaped events on the `unifi` source, `parse_unifi` now sets
  `.severity`, `.src_ip`, `.dst_ip`, `.dst_port`, `.proto`, `.host`, `.hostname`,
  `.appname`, `.program`, `.unifi_device_mac`, `.unifi_device_ip`, `.cef_vendor`,
  `.cef_product`, `.cef_version`, `.cef_signature_id`, `.cef_name` — consumed downstream
  unchanged (`enrich_geo`, `fast_path`, `sinks.loki`'s label templates, and everything in
  `siem-api`/`siem-web` that already reads `severity`/`host`/`program`/`src_ip`/`dst_ip`/
  `dst_port`/`proto` from existing sources).

- [ ] **Step 1: Write the failing tests**

Create `siem-ingest/test/parse_unifi.tests.toml`:

```toml
[[tests]]
name = "cef parsing extracts real fields"

  [tests.input]
  insert_at = "parse_unifi"
  type = "log"

    [tests.input.log_fields]
    message = "CGU CEF:0|Ubiquiti|UniFi OS|5.1.19|1000|Admin Accessed UniFi OS|7|proto=TCP src=81.181.129.172 spt=54321 dst=192.168.0.233 dpt=443 UNIFIhost=Host UNIFIdeviceName=Townsville UNIFIdeviceMac=84:78:48:80:0d:86 UNIFIdeviceIp=192.168.0.1"

  [[tests.outputs]]
  extract_from = "parse_unifi"

    [[tests.outputs.conditions]]
    type = "vrl"
    source = '''
      assert_eq!(.severity, "err")
      assert_eq!(.src_ip, "81.181.129.172")
      assert_eq!(.dst_ip, "192.168.0.233")
      assert_eq!(.dst_port, 443)
      assert_eq!(.proto, "tcp")
      assert_eq!(.host, "Townsville")
      assert_eq!(.hostname, "Townsville")
      assert_eq!(.program, "Admin Accessed UniFi OS")
      assert_eq!(.unifi_device_mac, "84:78:48:80:0d:86")
      assert_eq!(.unifi_device_ip, "192.168.0.1")
      assert_eq!(.parser, "unifi-cef")
    '''

[[tests]]
name = "malformed CEF (too few fields) falls through without setting CEF fields"

  [tests.input]
  insert_at = "parse_unifi"
  type = "log"

    [tests.input.log_fields]
    message = "CGU CEF:0|Ubiquiti|UniFi OS|5.1.19"

  [[tests.outputs]]
  extract_from = "parse_unifi"

    [[tests.outputs.conditions]]
    type = "vrl"
    source = '''
      assert_eq!(.parser, "unifi-cef")
      assert!(!exists(.severity))
      assert!(!exists(.src_ip))
    '''

[[tests]]
name = "cef falls back to UNIFIhost when UNIFIdeviceName is absent"

  [tests.input]
  insert_at = "parse_unifi"
  type = "log"

    [tests.input.log_fields]
    message = "CGU CEF:0|Ubiquiti|UniFi OS|5.1.19|1000|Admin Accessed UniFi OS|1|UNIFIhost=Office-UDM-Pro"

  [[tests.outputs]]
  extract_from = "parse_unifi"

    [[tests.outputs.conditions]]
    type = "vrl"
    source = '''
      assert_eq!(.host, "Office-UDM-Pro")
      assert_eq!(.hostname, "Office-UDM-Pro")
      assert_eq!(.severity, "info")
    '''

[[tests]]
name = "netfilter-bracket branch still works unaffected"

  [tests.input]
  insert_at = "parse_unifi"
  type = "log"

    [tests.input.log_fields]
    message = "[WAN_LOCAL-DEFAULT-D]IN=eth0 OUT= SRC=203.0.113.9 DST=192.168.1.1 PROTO=TCP DPT=22"

  [[tests.outputs]]
  extract_from = "parse_unifi"

    [[tests.outputs.conditions]]
    type = "vrl"
    source = '''
      assert_eq!(.rule, "WAN_LOCAL-DEFAULT-D")
      assert_eq!(.src_ip, "203.0.113.9")
      assert_eq!(.dst_ip, "192.168.1.1")
      assert_eq!(.dst_port, 22)
      assert_eq!(.proto, "tcp")
      assert_eq!(.action, "drop")
      assert!(!exists(.cef_vendor))
    '''
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `docker run --rm -v "$(pwd)/siem-ingest:/etc/vector" timberio/vector:0.49.0-alpine test /etc/vector/vector.toml /etc/vector/test/parse_unifi.tests.toml`
Expected: FAIL — `parse_unifi` doesn't yet set any CEF-derived fields, so the first,
second, and third test cases' assertions fail (the fourth, netfilter-branch test, will
already pass since that code isn't changing yet — that's fine, it's there to catch a
regression in the next step, not to currently fail).

- [ ] **Step 3: Implement the CEF branch**

In `siem-ingest/vector.toml`, replace the `[transforms.parse_unifi]` block's `source`
with:

```toml
[transforms.parse_unifi]
type = "remap"
inputs = ["unifi"]
source = '''
  .job = "siem"
  .source = "udm-ultra"
  .transport = "udp/514"
  .parser = "unifi-os"

  ## UniFi OS's "SIEM Server" integration (Settings → System Logging / SIEM) sends
  ## CEF-formatted messages with no <PRI> header, wrapped behind an arbitrary leading
  ## envelope token (observed: "CGU ", also "pandora " in vendor docs) that isn't a
  ## stable/parseable hostname. Splitting on the literal "CEF:0|" (rather than assuming
  ## the message starts with it) discards that leading token regardless of its content.
  if contains(string!(.message), "CEF:0|") {
    .parser = "unifi-cef"
    cef_body = split(string!(.message), "CEF:0|")[1]
    ## split!(): cef_body's static type is "string or null" (array indexing can't be
    ## proven non-null at compile time), even though contains() above guarantees a
    ## match exists at runtime. The bang assertion is required here, not optional.
    fields = split!(cef_body, "|")
    if length(fields) >= 7 {
      .cef_vendor = fields[0]
      .cef_product = fields[1]
      .cef_version = fields[2]
      .cef_signature_id = fields[3]
      .cef_name = fields[4]
      .appname = fields[4]
      .program = fields[4]
      ## to_int() is genuinely fallible (non-numeric input), so ?? is correct here —
      ## unlike the plain-field-access cases below, where ?? is rejected by the compiler
      ## as "unnecessary error coalescing" since field access on a map can't fail.
      cef_severity = to_int(fields[5]) ?? 0
      .severity = if cef_severity >= 9 { "crit" } else if cef_severity >= 7 { "err" } else if cef_severity >= 4 { "warning" } else { "info" }
      extension = join(slice!(fields, 6), "|") ?? ""
      ext, err = parse_key_value(extension, key_value_delimiter: "=", field_delimiter: " ")
      if err == null {
        .src_ip = ext.src
        .dst_ip = ext.dst
        dpt = ext.dpt
        .dst_port = to_int(dpt) ?? null
        proto = ext.proto
        ## downcase!() (bang): parse_key_value's values are typed as
        ## "string, boolean, null or array", not guaranteed string, even inside this
        ## is_null-guarded branch — the compiler still requires the assertion.
        .proto = if is_null(proto) { "" } else { downcase!(proto) }
        .unifi_device_mac = ext.UNIFIdeviceMac
        .unifi_device_ip = ext.UNIFIdeviceIp
        ## UNIFIhost has been observed as a useless literal "Host" placeholder on a
        ## real device, while UNIFIdeviceName held the actual device name in the same
        ## message — prefer UNIFIdeviceName, fall back to UNIFIhost only if absent.
        device_name = if !is_null(ext.UNIFIdeviceName) { ext.UNIFIdeviceName } else { ext.UNIFIhost }
        if !is_null(device_name) {
          .host = device_name
          .hostname = device_name
        }
      }
    }
  } else if match(string!(.message), r'\[(?P<rule>[A-Z_]+-\w+-[ADR])\]') {
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
```

This is the entire replacement for the transform's `source` value — everything else in
the `[transforms.parse_unifi]` block (`type`, `inputs`) is unchanged.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `docker run --rm -v "$(pwd)/siem-ingest:/etc/vector" timberio/vector:0.49.0-alpine test /etc/vector/vector.toml /etc/vector/test/parse_unifi.tests.toml`
Expected: PASS (4 tests — `cef parsing extracts real fields`, `malformed CEF (too few
fields) falls through without setting CEF fields`, `cef falls back to UNIFIhost when
UNIFIdeviceName is absent`, `netfilter-bracket branch still works unaffected`).

- [ ] **Step 5: Run the full local Docker harness as an end-to-end sanity check**

The `vector test` cases above verify the transform in isolation. As a broader check that
nothing else in the pipeline breaks, run the existing local harness:
`cd siem-ingest/test && docker compose up -d`, then `./send-test-traffic.sh` (or send a
raw UDP CEF payload matching the test message above to port 514 directly, e.g. via `echo`
piped to `nc -u localhost 514`), and confirm via the harness's stub receiver / Loki query
that a CEF-sourced event reaches Loki with the expected fields, then `docker compose down`.
This step doesn't need to be exhaustive — its purpose is confirming the change composes
correctly with `enrich_geo` and the sinks, not re-verifying the transform logic itself
(already covered by Step 4).

- [ ] **Step 6: Commit**

```bash
git add siem-ingest/vector.toml siem-ingest/test/parse_unifi.tests.toml
git commit -m "Parse UniFi CEF SIEM Server events instead of relying on defaults"
```

---

### Task 2: siem-ingest — update README's known-gaps section

**Files:**
- Modify: `siem-ingest/README.md`

**Interfaces:**
- None (documentation only).

- [ ] **Step 1: Replace the CEF-integration known-gap bullet**

In `siem-ingest/README.md`'s "Known gaps in this pass" section, find the bullet starting
`**UniFi OS's "SIEM Server" integration**...` (it currently describes severity-default and
garbled-host/hostname/appname as unresolved, and ends with the timestamp-clamp paragraph).
Replace the entire bullet (from `- **UniFi OS's "SIEM Server" integration**` through the
end of its timestamp-clamp paragraph, `...even with the same \`TZ\` set.`) with:

```markdown
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
```

- [ ] **Step 2: Commit**

```bash
git add siem-ingest/README.md
git commit -m "Update siem-ingest README: UniFi CEF integration is now parsed"
```
