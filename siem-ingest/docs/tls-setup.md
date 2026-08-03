# TLS setup for the syslog-TLS source (port 6514)

`hosts_tls` (Vector's TCP+TLS syslog source, port 6514) needs a server
certificate and key. No existing homelab certificate is directly reusable
here — nginx-proxy-manager's Let's Encrypt certificates are issued for
HTTP-routed hostnames, not exposed for arbitrary raw TCP services like this
one.

A self-signed certificate is appropriate: this is host-to-host syslog
forwarding on the internal `backend` Docker network, not a
publicly-verified TLS endpoint.

```bash
mkdir -p ${MY_DOCKER_DATA_DIR}/homesiem/vector/tls
openssl req -x509 -newkey rsa:2048 -nodes -days 3650 \
  -keyout ${MY_DOCKER_DATA_DIR}/homesiem/vector/tls/server.key \
  -out ${MY_DOCKER_DATA_DIR}/homesiem/vector/tls/server.crt \
  -subj "/CN=siem-ingest.internal"
```

`vector.toml`'s `sources.hosts_tls.tls` block expects these at
`/etc/vector/tls/server.crt` and `/etc/vector/tls/server.key` inside the
container — mount `${MY_DOCKER_DATA_DIR}/homesiem/vector/tls` there (or
place the cert/key directly under the existing
`${MY_DOCKER_DATA_DIR}/homesiem/vector` mount, in a `tls/` subdirectory, so
one volume mount covers both `vector.toml` and the TLS material).

## `verify_certificate` means mutual TLS — resolved

An earlier pass flagged this as an open question. It is now settled, with
direct evidence against `timberio/vector:0.49.0-alpine`.

On a Vector **source**, `verify_certificate` controls *client*-certificate
verification — mutual TLS — not "verify our own certificate". With
`verify_certificate = true` and no `ca_file`, port 6514 rejects **every**
connection:

| Client | Result |
| --- | --- |
| No client certificate | TLS alert 40 (`handshake_failure`) |
| Self-signed client certificate | TLS alert 48 (`unknown_ca`) |

The alert-40 → alert-48 change is the proof: Vector did request and read the
client certificate, then rejected it because it was not issued by a CA in
Vector's trust store. With no `ca_file` set, Vector falls back to the
container's system root store, which will never contain a private homelab
certificate. The port was therefore unusable as originally shipped.

`vector.toml` now sets `verify_certificate = false`, giving ordinary one-way
TLS: the connection is encrypted, and the **sending host** verifies Vector's
certificate. That matches what this document describes above and what the
design intended. Verified end-to-end at this setting — a plain `openssl
s_client` with no client certificate completes the handshake, and the line
lands in Loki with `transport="tls/6514"` and produces a source heartbeat.

So: any host forwarding syslog to port 6514 needs to **trust this
self-signed certificate** (copy `server.crt` into its trust store, or point
its syslog client's CA setting at it). A sender that cannot be configured to
trust a custom certificate should use the unencrypted TCP/601 source instead.

### Opting into real mutual TLS (optional)

If you do want client-certificate authentication, one-way TLS is not enough.
Stand up a small private CA, issue Vector a server certificate and each
sending host a client certificate from it, then set both of these in
`vector.toml`'s `[sources.hosts_tls.tls]` block:

```toml
verify_certificate = true
ca_file = "/etc/vector/tls/ca.crt"
```

Without the `ca_file` line, `verify_certificate = true` will lock out every
sender, exactly as measured above.
