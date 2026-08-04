#!/bin/sh
# Runs update-threatlist.py once immediately, then on a loop every
# UPDATE_INTERVAL_SECONDS (default 24h). No cron daemon needed for a
# single scheduled job - a sleep loop is simpler and matches this
# project's preference for the least machinery that does the job.
set -eu

INTERVAL="${UPDATE_INTERVAL_SECONDS:-86400}"
OUTPUT="${THREATLIST_OUTPUT:-/geoip/threatlist.csv}"

while true; do
	python3 /app/update-threatlist.py --output "$OUTPUT" --verbose || echo "update-threatlist.py failed, will retry next interval" >&2
	sleep "$INTERVAL"
done
