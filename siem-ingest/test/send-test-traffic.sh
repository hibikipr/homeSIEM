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
if command -v ncat >/dev/null 2>&1; then
  NC_UDP=(ncat -u -w1)
  NC_TCP=(ncat -w1)
else
  NC_UDP=(nc -u -w1)
  NC_TCP=(nc -w1)
fi

echo "<134>$(date -u +'%b %e %H:%M:%S') UDM-Ultra kernel: [WAN_LOCAL-1000-D] IN=eth0 OUT= SRC=203.0.113.7 DST=10.0.0.1 PROTO=TCP SPT=443 DPT=22" \
  | "${NC_UDP[@]}" "$UNIFI_HOST" "$UNIFI_PORT"

echo "<134>1 $(date -u +'%Y-%m-%dT%H:%M:%SZ') test-host-1 sshd 1234 - - Failed password for root from 198.51.100.5 port 4242 ssh2" \
  | "${NC_TCP[@]}" "$UNIFI_HOST" "$HOSTS_TCP_PORT"

echo "Sent 2 synthetic syslog lines (UniFi UDP/$UNIFI_PORT, host TCP/$HOSTS_TCP_PORT)."
