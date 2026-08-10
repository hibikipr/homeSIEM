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
