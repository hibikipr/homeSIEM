# Forwarding a native Homebridge install's logs

Homebridge installed via the official Raspberry Pi image or the `hb-service`
installer runs as a systemd service, journald-backed, on its own Pi -
separate from the SIEM host. The journald-driver + host-rsyslog-bridge
approach already used for the SIEM host's own Docker containers
(`stacks/homelab-siem/log-forwarder/` in the infra repo) applies here too,
with two changes:

- **Filter on the systemd unit, not `CONTAINER_NAME`.** The existing bridge
  matches `$!CONTAINER_NAME != ""`, which only Docker's journald log driver
  sets. A native systemd service like Homebridge has no such field - filter
  on `$!_SYSTEMD_UNIT` instead.
- **Forward to a real LAN address, not `127.0.0.1`.** The existing bridge
  targets `127.0.0.1:601` because it runs on the same host as `siem-ingest`.
  This one runs on a different physical Pi, so `target` has to be
  `siem-ingest`'s actual LAN IP.

## Setup

1. On the Homebridge Pi, confirm the actual unit name:
   ```bash
   systemctl status homebridge
   ```
   (Standard `hb-service` installs use `homebridge.service`; child bridges
   run as separate `homebridge@<name>.service` units if you have that
   feature enabled - the filter below catches both via `startswith`.)

2. Install rsyslog and deploy [`homebridge-rsyslog.conf`](homebridge-rsyslog.conf)
   (this directory), with `SIEM_INGEST_HOST` replaced by `siem-ingest`'s LAN
   IP - the same host `514`/`601`/`6514` are already published on for every
   other syslog sender:
   ```bash
   sudo apt-get update -qq && sudo apt-get install -y rsyslog

   sudo cp homebridge-rsyslog.conf /etc/rsyslog.d/60-siem-homebridge-forward.conf
   sudo sed -i 's/SIEM_INGEST_HOST/<your-siem-ingest-lan-ip>/' /etc/rsyslog.d/60-siem-homebridge-forward.conf

   sudo systemctl enable rsyslog
   sudo systemctl restart rsyslog
   ```

3. Verify the bridge itself started cleanly:
   ```bash
   sudo journalctl -u rsyslog -n 20
   ```

4. Confirm events are actually arriving - check from the `siem-ingest` side
   (not just that rsyslog is running, which only proves the bridge process
   started, not that anything matched and got forwarded):
   - In siem-web's Search, filter for `program=homebridge`.
   - Or query Loki directly for `{job="siem", program="homebridge"}`.

   If nothing shows up after Homebridge has logged something new, check on
   the Homebridge Pi:
   - `systemctl status homebridge` actually matches what the config's
     `startswith "homebridge"` filter expects (case-sensitive, matches the
     `_SYSTEMD_UNIT` journal field written by systemd itself).
   - `sudo journalctl -u homebridge -n 5` shows recent activity - a bridge
     with nothing new to log won't produce anything to forward, independent
     of whether the bridge itself is correctly configured.
   - The Homebridge Pi can actually reach `siem-ingest` on port 601:
     `nc -zv <siem-ingest-lan-ip> 601` from the Homebridge Pi.

## New sources may need a new severity-detection branch

Same caveat as any new source (see the Unraid doc): `enrich_geo`'s severity
cascade in `vector.toml` only recognizes log formats it's already seen. If
Homebridge's own log lines don't match any existing branch, everything logged
to stderr will land as `err` regardless of its real level until a matching
branch is added - check a real captured sample once events are flowing.
