-- Tables/columns added after the initial release. Every statement in this
-- file MUST be individually idempotent (IF NOT EXISTS / OR IGNORE) - unlike
-- schema.sql, which only ever runs once against a genuinely fresh database
-- (see store.Migrate), this file runs on EVERY startup, including against
-- an already-populated production database.

CREATE TABLE IF NOT EXISTS notification_settings (
  id           INTEGER PRIMARY KEY CHECK (id = 1),
  min_severity TEXT NOT NULL DEFAULT 'info'
);
INSERT OR IGNORE INTO notification_settings (id, min_severity) VALUES (1, 'info');

CREATE TABLE IF NOT EXISTS insights (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  created_at    TEXT NOT NULL,
  title         TEXT NOT NULL,
  detail        TEXT NOT NULL,
  severity      TEXT NOT NULL,
  category      TEXT NOT NULL,
  evidence_json TEXT NOT NULL,
  dismissed     INTEGER NOT NULL DEFAULT 0
);

-- Admin-editable overrides for siem-insights' Ollama call. Defaults: a low
-- temperature keeps this analytical/extractive task grounded rather than
-- creative; num_predict bounds response length (and so worst-case
-- generation time) independent of OLLAMA_TIMEOUT_SEC; num_ctx is set
-- explicitly because Ollama's own runtime default (often 2048-4096) can
-- otherwise silently truncate the rollup+samples data in the prompt.
CREATE TABLE IF NOT EXISTS ollama_settings (
  id            INTEGER PRIMARY KEY CHECK (id = 1),
  system_prompt TEXT NOT NULL DEFAULT '',
  temperature   REAL NOT NULL DEFAULT 0.2,
  top_p         REAL NOT NULL DEFAULT 0.9,
  num_predict   INTEGER NOT NULL DEFAULT 1024,
  num_ctx       INTEGER NOT NULL DEFAULT 8192
);
INSERT OR IGNORE INTO ollama_settings (id) VALUES (1);
