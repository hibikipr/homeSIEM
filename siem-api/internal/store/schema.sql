-- siem.db - SQLite, WAL mode.
-- Small, effectively single-writer: rules, alerts, sources, identities, audit.
-- Log events themselves live in Loki, never here.

PRAGMA journal_mode = WAL;
PRAGMA busy_timeout = 5000;
PRAGMA foreign_keys = ON;

CREATE TABLE sources (
  id            INTEGER PRIMARY KEY,
  name          TEXT NOT NULL UNIQUE,       -- 'udm-ultra'
  address       TEXT NOT NULL,              -- sender IP
  transport     TEXT NOT NULL,              -- 'udp/514' | 'tcp/601' | 'tls/6514'
  parser        TEXT NOT NULL,              -- 'unifi-os' | 'rfc5424' | ...
  claimed       INTEGER NOT NULL DEFAULT 0,
  heartbeat_sec INTEGER NOT NULL DEFAULT 900,
  last_seen_at  TEXT,
  created_at    TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX idx_sources_address ON sources(address);

CREATE TABLE rules (
  id            INTEGER PRIMARY KEY,
  name          TEXT NOT NULL UNIQUE,       -- 'wan-portscan'
  shape         TEXT NOT NULL,              -- 'threshold' | 'first_seen' | 'absence'
  logql         TEXT NOT NULL,
  window_sec    INTEGER NOT NULL,
  threshold     INTEGER,
  group_by      TEXT,                       -- JSON array of field names
  severity      TEXT NOT NULL,              -- 'critical'|'warning'|'info'
  destinations  TEXT NOT NULL,              -- JSON array: ["inapp","ntfy"]
  cooldown_sec  INTEGER NOT NULL DEFAULT 3600,
  interval_sec  INTEGER NOT NULL DEFAULT 60,
  enabled       INTEGER NOT NULL DEFAULT 1,
  last_run_at   TEXT,
  created_by    INTEGER REFERENCES users(id),
  created_at    TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE alerts (
  id            INTEGER PRIMARY KEY,
  rule_id       INTEGER NOT NULL REFERENCES rules(id) ON DELETE CASCADE,
  group_key     TEXT NOT NULL,              -- dedupe key, e.g. src_ip value
  severity      TEXT NOT NULL,
  title         TEXT NOT NULL,
  body          TEXT NOT NULL,
  event_count   INTEGER NOT NULL DEFAULT 1,
  context       TEXT,                       -- JSON: ports, geoip, intel
  state         TEXT NOT NULL DEFAULT 'open', -- open|acked|muted|closed
  first_seen_at TEXT NOT NULL DEFAULT (datetime('now')),
  last_seen_at  TEXT NOT NULL DEFAULT (datetime('now')),
  acked_by      INTEGER REFERENCES users(id),
  acked_at      TEXT,
  muted_until   TEXT,                       -- set when state = 'muted'
  UNIQUE (rule_id, group_key, state)
);
CREATE INDEX idx_alerts_state ON alerts(state, last_seen_at DESC);

-- A handful of representative raw lines per alert, so the detail view does
-- not have to re-query Loki for context that may have aged out.
CREATE TABLE alert_samples (
  id        INTEGER PRIMARY KEY,
  alert_id  INTEGER NOT NULL REFERENCES alerts(id) ON DELETE CASCADE,
  ts        TEXT NOT NULL,
  line      TEXT NOT NULL
);

CREATE TABLE users (
  id          INTEGER PRIMARY KEY,
  subject     TEXT UNIQUE,                  -- OIDC 'sub'; NULL for break-glass
  email       TEXT,
  display_name TEXT,
  role        TEXT NOT NULL,                -- admin|analyst|viewer
  local_hash  TEXT,                         -- only for the break-glass admin
  last_login_at TEXT,
  created_at  TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE role_mappings (
  id          INTEGER PRIMARY KEY,
  group_claim TEXT NOT NULL UNIQUE,         -- 'admins'
  role        TEXT NOT NULL,                -- 'admin'
  priority    INTEGER NOT NULL DEFAULT 100  -- lowest wins
);

CREATE TABLE saved_searches (
  id        INTEGER PRIMARY KEY,
  name      TEXT NOT NULL,
  query     TEXT NOT NULL,
  owner_id  INTEGER REFERENCES users(id),
  shared    INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE audit (
  id        INTEGER PRIMARY KEY,
  ts        TEXT NOT NULL DEFAULT (datetime('now')),
  user_id   INTEGER REFERENCES users(id),
  action    TEXT NOT NULL,                  -- 'alert.ack','rule.update','auth.login'
  target    TEXT,
  detail    TEXT                            -- JSON
);
CREATE INDEX idx_audit_ts ON audit(ts DESC);

-- Addition beyond the reference schema: tracks values already observed per
-- first_seen rule, so FirstSeenEvaluator can diff new LogQL results against
-- what it has seen before without re-querying Loki's full history.
CREATE TABLE seen_values (
  id            INTEGER PRIMARY KEY,
  rule_id       INTEGER NOT NULL REFERENCES rules(id) ON DELETE CASCADE,
  value         TEXT NOT NULL,
  first_seen_at TEXT NOT NULL DEFAULT (datetime('now')),
  UNIQUE (rule_id, value)
);
