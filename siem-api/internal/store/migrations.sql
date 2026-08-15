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

-- fingerprint/occurrence_count/last_seen_at columns are added to this table
-- from Go (see store.Migrate / addColumnIfMissing in store.go), not here -
-- SQLite has no "ALTER TABLE ... ADD COLUMN IF NOT EXISTS", and a bare ADD
-- COLUMN would fail with "duplicate column name" on every startup after the
-- first, since (unlike schema.sql) this file runs on every startup.
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

-- A fingerprint (category + the set of programs in its evidence - see
-- store.ComputeFingerprint) that's been muted never gets inserted or bumped
-- by Service.GenerateNow again, regardless of how many times the underlying
-- condition recurs. Deliberately separate from `insights.dismissed`: a
-- dismissed insight that recurs is new information worth re-surfacing (see
-- Store.BumpInsight un-dismissing on recurrence); a muted one is a standing
-- "never show me this again" that recurrence must not override.
-- example_title is added from Go, not here - see mutedFingerprintColumns in
-- store.go, same reasoning as insights' fingerprint/occurrence_count/
-- last_seen_at above.
CREATE TABLE IF NOT EXISTS muted_insight_fingerprints (
  fingerprint TEXT PRIMARY KEY,
  category    TEXT NOT NULL,
  programs    TEXT NOT NULL,
  muted_at    TEXT NOT NULL
);

-- Admin-editable overrides for siem-insights' Ollama call. Defaults: a low
-- temperature keeps this analytical/extractive task grounded rather than
-- creative; num_predict bounds response length (and so worst-case
-- generation time) independent of OLLAMA_TIMEOUT_SEC; num_ctx is set
-- explicitly because Ollama's own runtime default (often 2048-4096) can
-- otherwise silently truncate the rollup+samples data in the prompt.
-- num_predict was 1024 until found in production that a genuinely eventful
-- pass (several distinct insights, each with its own evidence table) can
-- need more than that, and Ollama hitting the cap mid-array is exactly
-- what "unexpected end of JSON input" in the logs turned out to be -
-- doubled to 2048 for headroom; parseModelResponse also now salvages
-- whatever complete insights a still-truncated response did finish,
-- rather than discarding the whole pass, so this isn't the only mitigation.
CREATE TABLE IF NOT EXISTS ollama_settings (
  id            INTEGER PRIMARY KEY CHECK (id = 1),
  system_prompt TEXT NOT NULL DEFAULT '',
  temperature   REAL NOT NULL DEFAULT 0.2,
  top_p         REAL NOT NULL DEFAULT 0.9,
  num_predict   INTEGER NOT NULL DEFAULT 2048,
  num_ctx       INTEGER NOT NULL DEFAULT 8192
);
INSERT OR IGNORE INTO ollama_settings (id) VALUES (1);
