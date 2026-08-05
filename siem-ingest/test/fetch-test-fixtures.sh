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

# Generate a throwaway self-signed cert/key for the hosts_tls source
# (port 6514). This is NOT optional: docker-compose.yml mounts both files
# into the vector container, and vector.toml's [sources.hosts_tls.tls]
# block reads them at boot. Without them Docker silently creates
# *directories* at the mount paths and Vector refuses to build the whole
# topology ("Could not read certificate file ...: Is a directory"), taking
# down all three syslog ports, the Loki sink, the fast path and the
# heartbeat with it - not just port 6514. Test-only material; fixtures/ is
# gitignored, and the real deployment generates its own per docs/tls-setup.md.
mkdir -p fixtures/tls
if [ ! -s fixtures/tls/server.crt ] || [ ! -s fixtures/tls/server.key ]; then
  openssl req -x509 -newkey rsa:2048 -nodes -days 3650 \
    -keyout fixtures/tls/server.key \
    -out fixtures/tls/server.crt \
    -subj "/CN=siem-ingest.test" 2>/dev/null
  echo "Generated fixtures/tls/server.{crt,key} (self-signed, test-only)"
else
  echo "Reusing existing fixtures/tls/server.{crt,key}"
fi
