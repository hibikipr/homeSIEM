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

## Important: Mutual TLS requirement caveat

In testing against the actual Vector binary, `verify_certificate = true` on
the `hosts_tls` source may imply **mutual TLS** — not just "the sender must
trust Vector's certificate", but also "Vector requires the sender to present
a client certificate that Vector's trust store accepts". Bare `openssl
s_client` connections without a client certificate failed the TLS handshake
entirely (SSL alert number 40), suggesting the server enforces bidirectional
certificate verification. This requirement is stricter than one-way TLS and
was not fully verified in production. **Before relying on port 6514 in
production, test against a real syslog-sending client (not just `openssl
s_client`) to confirm whether mutual TLS is actually enforced and, if so,
what certificate configuration the sending host requires.**

Any host configured to forward syslog to port 6514 over TLS needs
`verify_certificate = true` (already set in `vector.toml`) satisfied on its
end — either trust this specific self-signed cert explicitly on the sending
host, or switch that host to the unencrypted TCP/601 source instead if its
syslog client can't be configured to trust a custom CA/cert.
