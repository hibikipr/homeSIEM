#!/usr/bin/env python3
"""Fetch free IP threat-intel feeds and write them out as threatlist.csv.

Output matches vector.toml's enrichment_tables.threatlist schema exactly:
a header row `ip,tag` followed by one row per single IP address (Vector's
file-based enrichment table does exact-key lookups, not CIDR matching, so
sources here are deliberately restricted to ones that publish individual
IPs rather than network blocks — see SOURCES below and docs/geoip-setup.md
for why e.g. Spamhaus DROP and DShield's block list are excluded).

Usage:
    python3 update-threatlist.py [--output PATH] [--extra PATH] [--verbose]

Intended to run on a schedule (cron/systemd timer) against the real
deployment path, e.g.:
    0 6 * * * python3 update-threatlist.py \
      --output ${MY_DOCKER_DATA_DIR}/homesiem/geoip/threatlist.csv
"""

from __future__ import annotations

import argparse
import csv
import ipaddress
import sys
import urllib.error
import urllib.request
from pathlib import Path

USER_AGENT = "homeSIEM-threatlist-updater/1.0 (+https://github.com/hibikipr/homeSIEM)"
TIMEOUT_SECONDS = 30

# Each source publishes one bare IP per line (optionally with trailing
# whitespace/extra columns we ignore), with '#'-prefixed comment/header
# lines. Picked from https://github.com/hslatman/awesome-threat-intelligence
# and cross-checked against MISP's default feed manifest
# (https://raw.githubusercontent.com/MISP/MISP/2.4/app/files/feed-metadata/defaults.json)
# for single-IP, no-auth, actively-maintained feeds only. Feeds that only
# publish CIDR ranges (Spamhaus DROP, DShield block.txt) are deliberately
# excluded — see the module docstring.
SOURCES = [
    {
        "name": "feodotracker",
        "url": "https://feodotracker.abuse.ch/downloads/ipblocklist.txt",
        "tag": "botnet-c2",
    },
    {
        "name": "blocklist.de",
        "url": "https://lists.blocklist.de/lists/all.txt",
        "tag": "attacker",
    },
    {
        "name": "cins-army",
        "url": "http://cinsscore.com/list/ci-badguys.txt",
        "tag": "cins-badguys",
    },
    {
        "name": "ipsum",
        "url": "https://raw.githubusercontent.com/stamparm/ipsum/master/levels/3.txt",
        "tag": "ipsum-aggregate",
    },
    {
        "name": "emergingthreats",
        "url": "https://rules.emergingthreats.net/blockrules/compromised-ips.txt",
        "tag": "compromised-host",
    },
]


def fetch_source(source: dict, verbose: bool) -> dict[str, set[str]]:
    """Fetch one source, return {ip: {tag}} for every valid IP line found."""
    request = urllib.request.Request(source["url"], headers={"User-Agent": USER_AGENT})
    ips: dict[str, set[str]] = {}
    try:
        with urllib.request.urlopen(request, timeout=TIMEOUT_SECONDS) as response:
            body = response.read().decode("utf-8", errors="replace")
    except (urllib.error.URLError, TimeoutError) as exc:
        print(f"warning: {source['name']}: fetch failed ({exc}), skipping", file=sys.stderr)
        return ips

    matched = 0
    for line in body.splitlines():
        line = line.strip()
        if not line or line.startswith("#") or line.startswith(";"):
            continue
        candidate = line.split()[0]
        try:
            ipaddress.ip_address(candidate)
        except ValueError:
            continue
        ips.setdefault(candidate, set()).add(source["tag"])
        matched += 1

    if verbose:
        print(f"{source['name']}: {matched} IPs from {source['url']}", file=sys.stderr)
    return ips


def load_extra(path: Path) -> dict[str, set[str]]:
    """Load a hand-maintained ip,tag CSV to merge in alongside the feeds."""
    ips: dict[str, set[str]] = {}
    with path.open(newline="") as f:
        for row in csv.DictReader(f):
            ip, tag = row.get("ip", "").strip(), row.get("tag", "").strip()
            if not ip or not tag:
                continue
            try:
                ipaddress.ip_address(ip)
            except ValueError:
                print(f"warning: --extra: skipping invalid IP {ip!r}", file=sys.stderr)
                continue
            ips.setdefault(ip, set()).add(tag)
    return ips


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument(
        "--output",
        type=Path,
        default=Path(__file__).resolve().parent.parent / "threatlist.csv",
        help="Path to write the resulting ip,tag CSV (default: siem-ingest/threatlist.csv)",
    )
    parser.add_argument(
        "--extra",
        type=Path,
        default=None,
        help="Optional hand-maintained ip,tag CSV to merge in alongside the fetched feeds",
    )
    parser.add_argument("--verbose", action="store_true", help="Log per-source IP counts to stderr")
    args = parser.parse_args()

    merged: dict[str, set[str]] = {}
    sources_ok = 0
    for source in SOURCES:
        result = fetch_source(source, args.verbose)
        if result:
            sources_ok += 1
        for ip, tags in result.items():
            merged.setdefault(ip, set()).update(tags)

    if args.extra:
        for ip, tags in load_extra(args.extra).items():
            merged.setdefault(ip, set()).update(tags)

    if sources_ok == 0 and not args.extra:
        print("error: every feed failed and no --extra file given, nothing to write", file=sys.stderr)
        return 1

    args.output.parent.mkdir(parents=True, exist_ok=True)
    with args.output.open("w", newline="") as f:
        writer = csv.writer(f)
        writer.writerow(["ip", "tag"])
        for ip in sorted(merged, key=lambda ip: (ipaddress.ip_address(ip).version, ipaddress.ip_address(ip))):
            writer.writerow([ip, "+".join(sorted(merged[ip]))])

    print(
        f"Wrote {len(merged)} IPs from {sources_ok}/{len(SOURCES)} feeds"
        f"{' + --extra' if args.extra else ''} to {args.output}"
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
