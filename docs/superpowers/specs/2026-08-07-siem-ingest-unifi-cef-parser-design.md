# siem-ingest: UniFi CEF ("SIEM Server") parser — design

Status: approved
Scope: replaces a documented known-gap in `siem-ingest/vector.toml`'s `parse_unifi`
transform. UniFi OS's "SIEM Server" integration (Settings → System Logging / SIEM — a
distinct feature from the classic "Remote Logging" toggle this pipeline was originally
built against) sends CEF-formatted messages that this transform currently doesn't parse
at all — `enrich_geo`'s severity-default and timestamp-clamp are the only handling that
exists today, both defensive fallbacks rather than real parsing. This pass adds real CEF
parsing, fixing the root cause both fallbacks were papering over.

## Context

Confirmed empirically (real UDM device, `tcpdump`-captured packets, and cross-referenced
against public documentation this session — Ubiquiti's own SIEM integration article and
Graylog's UniFi content-pack docs, which show a real example CEF line): the wire format is
a syslog envelope with no `<PRI>` header, an arbitrary leading token (observed: `"CGU "`,
and separately `"pandora "` in Graylog's own example — not a stable/parseable hostname),
followed by `CEF:0|Ubiquiti|<product>|<version>|<signature-id>|<name>|<severity>|
<extension>`, where `<extension>` is a space-separated `key=value` list using UniFi-
specific extension keys (`UNIFIhost`, `UNIFIdeviceName`, `UNIFIdeviceMac`,
`UNIFIdeviceIp`) alongside standard CEF keys (`proto`, `src`, `spt`, `dst`, `dpt`).

Real captured example (device identity redacted, structure preserved) shows a real,
observed discrepancy worth designing around: `UNIFIhost=Host` (the literal string "Host",
not a real hostname) appeared alongside `UNIFIdeviceName=Townsville` (the actual device
name) in the same message. Graylog's own docs example shows `UNIFIhost=Office-UDM-Pro` (a
real name in that case) — so `UNIFIhost`'s reliability appears to vary; `UNIFIdeviceName`
is the field actually observed working correctly.

Two consequences of the current gap, both currently handled only by defensive fallback,
not real parsing:
- No `<PRI>` header means Vector's syslog decoder never populates `.severity` —
  `enrich_geo` defaults it to `"info"` unconditionally for every CEF event, discarding the
  real CEF severity that's present in the message.
- Nothing parses CEF's pipe-delimited structure, so `host`/`hostname`/`appname` end up
  populated with whatever Vector's lenient RFC3164-ish decoder guessed at (previously
  observed: the CEF payload's own embedded timestamp landing in the `host` field).

## Goals

- Extend the existing `[transforms.parse_unifi]` remap (not a parallel transform) with a
  content-based branch: `contains(.message, "CEF:0|")` detects a CEF-shaped message before
  falling through to the existing netfilter-bracket branch.
- Parse the 7 pipe-delimited CEF header fields, and the extension via VRL's built-in
  `parse_key_value` (space-delimited fields, `=`-delimited key/value — exactly this
  message's shape).
- Map CEF's numeric severity (0-10) into this pipeline's existing RFC 5424 text vocabulary:
  `0-3 → "info"`, `4-6 → "warning"`, `7-8 → "err"`, `9-10 → "crit"`.
- Map extracted fields onto the **same structured field names the netfilter branch
  already uses** (`.src_ip`, `.dst_ip`, `.dst_port`, `.proto`), so both UniFi message
  formats produce a uniform event shape — `fast_path`'s filter, `enrich_geo`'s GeoIP/
  threat-intel lookup, and the Search screen's filters all keep working unmodified for
  CEF-sourced events, with no changes needed outside `parse_unifi` itself.
- Set `.host`/`.hostname` from `UNIFIdeviceName`, falling back to `UNIFIhost` only if
  `UNIFIdeviceName` is absent — fixing the garbled-host bug directly, based on the real
  observed reliability difference between the two fields.
- Set `.appname`/`.program` from the CEF `Name` header field (index 4) — currently
  garbled for CEF events; this gives a real, meaningful value (e.g. `"Threat Detected and
  Blocked"`) instead.
- Capture `UNIFIdeviceMac`/`UNIFIdeviceIp` as new fields (`.unifi_device_mac`,
  `.unifi_device_ip`) for completeness, even though nothing downstream consumes them yet.
- If the CEF header has fewer than 7 pipe-delimited fields (malformed/truncated), skip the
  branch entirely — the event falls through to `enrich_geo`'s existing severity-default/
  timestamp-clamp safety net unchanged, same graceful-degradation behavior as today. Never
  drop the event.

## Explicitly out of scope this pass

- **CEF escape-sequence handling** (`\|` inside a field value per the CEF spec). A plain
  pipe-split matches every real example seen; a fully spec-compliant parser is more
  complexity than this scope needs. Documented as a known simplification.
- **Setting `.action` for CEF events.** The netfilter branch sets `.action = "drop"|
  "accept"` from its rule-name suffix, which feeds `fast_path`'s filter condition. A CEF
  event's `Name` field mentioning "Blocked" could heuristically map to the same thing, but
  that's inventing business logic without real evidence it's correct for UniFi's actual
  vocabulary. Left unset — CEF events still reach `fast_path` via the existing threat-intel
  path (which now works correctly once `.src_ip` is real), just not via the drop-rule path.
  A future pass could revisit this with more real examples of CEF `Name` values.
- Any change to `enrich_geo`'s existing severity-default/timestamp-clamp fallbacks — both
  stay as-is, as the safety net for CEF messages that fail the new branch's 7-field guard
  (or any other malformed input).

## Verification (before this is considered done)

Same discipline as every other VRL change in this pipeline: tested against the real
`timberio/vector:0.49.0-alpine` binary via the local Docker harness (`siem-ingest/test/`),
replaying the real captured CEF line's structure (leading envelope token + full CEF
header/extension, per the Context section above) — not just written and assumed correct
from reading VRL documentation. `parse_key_value`'s exact call signature in particular
needs real-binary confirmation, not just recalled from memory.

## Testing

- Local Docker harness (`siem-ingest/test/docker-compose.yml`, real Vector + real Loki):
  replay a synthetic CEF-shaped UDP payload matching the real captured structure, confirm
  the resulting Loki-stored event has real `severity`/`host`/`program`/`src_ip`/`dst_ip`/
  `dst_port`/`proto` values, not defaults or garbage.
  - One additional test case: a CEF message with fewer than 7 pipe-delimited fields,
    confirming it falls through to `enrich_geo`'s existing fallback instead of erroring or
    being dropped.
- No unit-test framework exists for VRL itself (Vector doesn't have one) — verification is
  end-to-end through the real harness, matching how every other transform in this file was
  verified.

## Known gaps after this pass

- CEF escape sequences aren't handled (see Explicitly out of scope above).
- `.action` isn't set for CEF events, so they don't participate in `fast_path`'s drop-rule
  forwarding path (only the threat-intel path).
- This pass is verified against the local Docker harness with a synthetic payload matching
  the real captured structure — not re-verified against a live UDM device's actual "Send
  Test Event" button, since a real device isn't reachable from this environment. The
  pipeline's established next real-world verification step (documented in the README's
  existing known-gaps list) still applies once deployed.
