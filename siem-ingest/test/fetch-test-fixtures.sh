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
