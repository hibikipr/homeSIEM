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
   deployment host. No `vector.toml` edit is needed —
   `enrichment_tables.geolite.path` already defaults to
   `/geoip/GeoLite2-City.mmdb`, which is where the compose file's
   `${MY_DOCKER_DATA_DIR}/homesiem/geoip:/geoip` mount puts it. (Only the
   local test harness overrides this, via `GEOIP_MMDB_PATH`, because
   MaxMind's test-only database has a different filename.)

   Do this **before** first start. If the file is missing, Vector does not
   report "file not found" — it silently drops the enrichment table, and the
   `enrich_geo` transform then fails to compile with `invalid enum variant
   for argument "table" ... received: "geolite"`, which takes down the whole
   pipeline, not just geo enrichment.
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
