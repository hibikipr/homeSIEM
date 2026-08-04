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

`scripts/update-threatlist.py` generates this file for you, pulled from
five free, no-signup IP threat-intel feeds (picked from
[awesome-threat-intelligence](https://github.com/hslatman/awesome-threat-intelligence),
cross-checked against
[MISP's default feed manifest](https://raw.githubusercontent.com/MISP/MISP/2.4/app/files/feed-metadata/defaults.json)
— see "Why these five feeds, and not MISP itself" below).

**Nothing it produces is committed to this repo** (matches the project's
existing decision not to embed GeoIP data either) — the feeds change
multiple times a day, so a checked-in copy would be stale before it
shipped.

### Recommended: the `siem-threatlist-updater` container

Both `docker-compose.yml` here and the root example stack already run it
for you as a service — `siem-threatlist-updater`, built from
`scripts/Dockerfile` and published to
`ghcr.io/hibikipr/siem-threatlist-updater`. It fetches once at container
start, then re-fetches every `UPDATE_INTERVAL_SECONDS` (default 86400 = 24h),
writing straight into the mounted geoip volume. Nothing else to run — bring
the stack up and it stays current on its own. It deliberately isn't on the
`backend` network (that's `internal: true` in a real deployment sharing
this repo's existing infra, with no route to the actual feed URLs), so it
gets its own default network with a normal route out.

`siem-ingest`'s `depends_on` waits on this container's healthcheck (which
only passes once `threatlist.csv` has real content), not just "container
started" — so on a fresh deploy Vector genuinely won't try to boot before
the file exists. This is the one piece of the "file must exist before
first start" requirement below that the container path handles for you
automatically; GeoLite2-City.mmdb still needs the manual steps above
regardless of which threatlist.csv path you use.

### Alternative: run the script directly

If you'd rather not run it as a container — a bare-metal deployment, or
you want a one-off fetch — `python3 scripts/update-threatlist.py` works
standalone, stdlib-only, no `pip install` needed:

```bash
python3 scripts/update-threatlist.py \
  --output ${MY_DOCKER_DATA_DIR}/homesiem/geoip/threatlist.csv
```

An IP seen in more than one feed gets a combined tag
(`attacker+ipsum-aggregate`) rather than being deduped down to one source.
To add your own entries alongside the fetched feeds without editing the
script, pass `--extra your-own.csv` (same `ip,tag` shape) — they're merged
in, not overwritten on the next run. On a schedule via cron:

```cron
# /etc/cron.d/homesiem-threatlist — refresh daily at 06:00
0 6 * * * root python3 /path/to/homeSIEM/siem-ingest/scripts/update-threatlist.py --output ${MY_DOCKER_DATA_DIR}/homesiem/geoip/threatlist.csv >> /var/log/homesiem-threatlist.log 2>&1
```

The **file itself still has to exist** before first start, same failure
mode as the GeoIP database (a missing enrichment-table file takes down the
whole pipeline, not just that one enrichment lookup) — but unlike the
GeoIP database, it's fine for it to have zero data rows below the header
while you wait on the first real fetch, same as the checked-in fixture at
`test/threatlist.csv`. Run the script once before deploying `siem-ingest`
so real data is already there from the start.

### Why these five feeds, and not MISP itself

[MISP](https://github.com/MISP/MISP) is a full threat-intel *sharing
platform* (PHP + MySQL + Redis), not just a feed — running one solely to
re-expose IP lists you can already fetch as plain text is unnecessary
weight for a homelab. What MISP actually gives you for free, with no
server required, is its
[default feed manifest](https://www.misp-project.org/feeds/): a JSON list
of ~88 community OSINT feeds (URLs + formats), several of which are the
same sources used here (blocklist.de, the IPsum levels). That manifest is
a good place to look when you want to add more feeds later — the script's
`SOURCES` list is deliberately small and hand-picked, not comprehensive.

Feeds that only publish CIDR ranges rather than individual IPs (Spamhaus
DROP, SANS ISC/DShield's `block.txt`) are excluded on purpose: Vector's
file-based enrichment table does an **exact-key lookup**, not CIDR
matching, so a `/20` network block in the CSV would never match a real
`src_ip` anyway. abuse.ch's SSLBL feed is also excluded — it was
deprecated in January 2025.

If you already run your own MISP instance, its REST API
(`POST {MISP_URL}/attributes/restSearch`, `Authorization: <api-key>`
header, `{"returnFormat":"json","type":{"OR":["ip-src","ip-dst"]}}` body)
is a reasonable future extension point for this script — not implemented
here since it needs a live instance + API key most homelab setups won't
have.
