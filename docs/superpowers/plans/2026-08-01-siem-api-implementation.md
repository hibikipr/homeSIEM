# siem-api Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build `siem-api`, the Go service that owns rule evaluation, alert lifecycle, auth/RBAC, and the query surface for the homeSIEM console, per `docs/superpowers/specs/2026-08-01-siem-api-design.md`.

**Architecture:** A single Go binary (`cmd/siem-api`) wiring together `internal/` packages — `store` (SQLite), `auth` (internal JWT + RBAC), `loki`/`ntfy` (external clients), `rules` (scheduler + evaluators), `alerts` (lifecycle), `sse` (fan-out), `api` (HTTP handlers) — behind stdlib `net/http`.

**Tech Stack:** Go, `net/http` (stdlib pattern routing, Go 1.22+), `modernc.org/sqlite`, `golang-jwt/jwt/v5`, `golang.org/x/crypto/bcrypt`, `log/slog`.

## Global Constraints

- Module root: `siem-api/` at repo root. Module path: `github.com/hibikipr/homeSIEM/siem-api`.
- Go version: 1.22+ (required for `net/http` pattern-based routing — `mux.HandleFunc("POST /rules", ...)`).
- No router library, no ORM, no query builder. `database/sql` directly against `modernc.org/sqlite`.
- SQLite DSN: driver name `sqlite` (modernc.org/sqlite registers this). Pragmas (`journal_mode=WAL`, `busy_timeout=5000`, `foreign_keys=ON`) applied via `Exec` after `sql.Open`, not via DSN query params — portable across driver versions.
- `schema.sql` lives at `siem-api/schema.sql`, embedded via `go:embed`, copied from `design_handoff_homesiem/reference/schema.sql` plus one addition: the `seen_values` table (see Task 9).
- Every package under `internal/` communicates with its neighbors through an interface, not a concrete type, wherever the design spec calls for testability without real Loki/SQLite (evaluators, scheduler, alerts service).
- `log/slog` JSON structured logging throughout; no `fmt.Println`/`log.Println`.
- Test files sit next to the code they test (`foo.go` + `foo_test.go`), standard Go convention — no separate `tests/` tree.
- Every task's Go code must compile and its tests must pass before moving to the next task. Run `go build ./...` and `go vet ./...` at the end of every task, not just the unit tests named in that task.
- Commit after every task with a message describing what the task added — no bundling multiple tasks into one commit.

---

### Task 1: Project scaffold, schema, Dockerfile

**Files:**
- Create: `siem-api/go.mod`
- Create: `siem-api/cmd/siem-api/main.go` (stub `func main() {}`)
- Create: `siem-api/schema.sql` (copy of `design_handoff_homesiem/reference/schema.sql` plus the `seen_values` table)
- Create: `siem-api/Dockerfile`
- Create: `siem-api/.dockerignore`

**Interfaces:**
- Produces: `siem-api/schema.sql` content (byte-for-byte, consumed by Task 3's `go:embed`).

- [ ] **Step 1: Initialize the Go module**

Run:
```bash
mkdir -p siem-api/cmd/siem-api
cd siem-api && go mod init github.com/hibikipr/homeSIEM/siem-api
```

- [ ] **Step 2: Write the main stub**

`siem-api/cmd/siem-api/main.go`:
```go
package main

func main() {}
```

- [ ] **Step 3: Copy and extend schema.sql**

Copy `design_handoff_homesiem/reference/schema.sql` to `siem-api/schema.sql` verbatim, then append at the end:

```sql

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
```

- [ ] **Step 4: Write the Dockerfile**

`siem-api/Dockerfile`:
```dockerfile
FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/siem-api ./cmd/siem-api

FROM alpine:3.19
RUN apk add --no-cache ca-certificates wget
COPY --from=build /out/siem-api /usr/local/bin/siem-api
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/siem-api"]
```

`siem-api/.dockerignore`:
```
*_test.go
.git
```

- [ ] **Step 5: Verify it builds**

Run: `cd siem-api && go build ./...`
Expected: succeeds with no output (nothing to build yet beyond the stub, but the module and Dockerfile syntax are both valid — confirm the Dockerfile separately with `docker build -t siem-api-scaffold-check siem-api` if Docker is available locally; skip if not, later tasks will build the real thing).

- [ ] **Step 6: Commit**

```bash
git add siem-api/go.mod siem-api/cmd siem-api/schema.sql siem-api/Dockerfile siem-api/.dockerignore
git commit -m "Scaffold siem-api Go module, schema, and Dockerfile"
```

---

### Task 2: `internal/config` — env var loading

**Files:**
- Create: `siem-api/internal/config/config.go`
- Test: `siem-api/internal/config/config_test.go`

**Interfaces:**
- Produces: `config.Config` struct and `config.Load() (Config, error)`, consumed by `main.go` (Task 29) and by every other package's constructor that needs a setting.

```go
type Config struct {
    Addr                    string
    DatabaseURL             string
    LokiURL                 string
    LokiJobLabel            string
    NtfyURL                 string
    NtfyTopic               string
    NtfyToken               string
    OIDCIssuer              string
    OIDCClientID            string
    OIDCGroupsScope         string
    GeoIPDB                 string
    SessionSecret           []byte
    FastpathToken           string
    LocalAdminUsername      string
    LocalAdminPasswordHash  string
}
```

- [ ] **Step 1: Write the failing test**

`siem-api/internal/config/config_test.go`:
```go
package config

import (
    "encoding/base64"
    "testing"
)

func setRequiredEnv(t *testing.T) {
    t.Helper()
    secret := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
    env := map[string]string{
        "DATABASE_URL":        "sqlite:///data/siem.db",
        "LOKI_URL":            "http://loki:3100",
        "NTFY_URL":            "http://ntfy",
        "NTFY_TOPIC":          "homesiem",
        "OIDC_ISSUER":         "https://pocketid.townsville.cc",
        "OIDC_CLIENT_ID":      "homeSIEM",
        "SIEM_SESSION_SECRET": secret,
        "SIEM_FASTPATH_TOKEN": "fastpath-token",
    }
    for k, v := range env {
        t.Setenv(k, v)
    }
}

func TestLoad_DefaultsApplied(t *testing.T) {
    setRequiredEnv(t)

    cfg, err := Load()
    if err != nil {
        t.Fatalf("Load() error = %v", err)
    }
    if cfg.Addr != ":8080" {
        t.Errorf("Addr = %q, want :8080", cfg.Addr)
    }
    if cfg.LokiJobLabel != "siem" {
        t.Errorf("LokiJobLabel = %q, want siem", cfg.LokiJobLabel)
    }
    if cfg.OIDCGroupsScope != "groups" {
        t.Errorf("OIDCGroupsScope = %q, want groups", cfg.OIDCGroupsScope)
    }
    if len(cfg.SessionSecret) != 32 {
        t.Errorf("SessionSecret len = %d, want 32", len(cfg.SessionSecret))
    }
}

func TestLoad_MissingRequired(t *testing.T) {
    setRequiredEnv(t)
    t.Setenv("DATABASE_URL", "")

    if _, err := Load(); err == nil {
        t.Fatal("Load() error = nil, want error for missing DATABASE_URL")
    }
}

func TestLoad_InvalidSessionSecret(t *testing.T) {
    setRequiredEnv(t)
    t.Setenv("SIEM_SESSION_SECRET", "not-valid-base64!!")

    if _, err := Load(); err == nil {
        t.Fatal("Load() error = nil, want error for invalid base64 secret")
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd siem-api && go test ./internal/config/... -v`
Expected: FAIL — package `config` / function `Load` undefined.

- [ ] **Step 3: Write the implementation**

`siem-api/internal/config/config.go`:
```go
package config

import (
    "encoding/base64"
    "fmt"
    "os"
)

type Config struct {
    Addr                   string
    DatabaseURL            string
    LokiURL                string
    LokiJobLabel           string
    NtfyURL                string
    NtfyTopic              string
    NtfyToken              string
    OIDCIssuer             string
    OIDCClientID           string
    OIDCGroupsScope        string
    GeoIPDB                string
    SessionSecret          []byte
    FastpathToken          string
    LocalAdminUsername     string
    LocalAdminPasswordHash string
}

func Load() (Config, error) {
    cfg := Config{
        Addr:            getenv("ADDR", ":8080"),
        DatabaseURL:     os.Getenv("DATABASE_URL"),
        LokiURL:         os.Getenv("LOKI_URL"),
        LokiJobLabel:    getenv("LOKI_JOB_LABEL", "siem"),
        NtfyURL:         os.Getenv("NTFY_URL"),
        NtfyTopic:       os.Getenv("NTFY_TOPIC"),
        NtfyToken:       os.Getenv("NTFY_TOKEN"),
        OIDCIssuer:      os.Getenv("OIDC_ISSUER"),
        OIDCClientID:    os.Getenv("OIDC_CLIENT_ID"),
        OIDCGroupsScope: getenv("OIDC_GROUPS_SCOPE", "groups"),
        GeoIPDB:         os.Getenv("GEOIP_DB"),
        FastpathToken:   os.Getenv("SIEM_FASTPATH_TOKEN"),

        LocalAdminUsername:     os.Getenv("SIEM_LOCAL_ADMIN_USERNAME"),
        LocalAdminPasswordHash: os.Getenv("SIEM_LOCAL_ADMIN_PASSWORD_HASH"),
    }

    required := map[string]string{
        "DATABASE_URL":        cfg.DatabaseURL,
        "LOKI_URL":            cfg.LokiURL,
        "NTFY_URL":            cfg.NtfyURL,
        "NTFY_TOPIC":          cfg.NtfyTopic,
        "OIDC_ISSUER":         cfg.OIDCIssuer,
        "OIDC_CLIENT_ID":      cfg.OIDCClientID,
        "SIEM_FASTPATH_TOKEN": cfg.FastpathToken,
    }
    for name, val := range required {
        if val == "" {
            return Config{}, fmt.Errorf("config: %s is required", name)
        }
    }

    secretRaw := os.Getenv("SIEM_SESSION_SECRET")
    if secretRaw == "" {
        return Config{}, fmt.Errorf("config: SIEM_SESSION_SECRET is required")
    }
    secret, err := base64.StdEncoding.DecodeString(secretRaw)
    if err != nil {
        return Config{}, fmt.Errorf("config: SIEM_SESSION_SECRET is not valid base64: %w", err)
    }
    cfg.SessionSecret = secret

    return cfg, nil
}

func getenv(key, fallback string) string {
    if v := os.Getenv(key); v != "" {
        return v
    }
    return fallback
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd siem-api && go test ./internal/config/... -v`
Expected: PASS (all three tests).

- [ ] **Step 5: Commit**

```bash
git add siem-api/internal/config
git commit -m "Add siem-api config loading from environment"
```

---

### Task 3: `internal/store` — DB open, pragmas, schema migration

**Files:**
- Create: `siem-api/internal/store/store.go`
- Create: `siem-api/internal/store/schema.sql` (`go:embed` source — copy of `siem-api/schema.sql`; kept alongside the package so `go:embed` doesn't need `../../`)
- Test: `siem-api/internal/store/store_test.go`

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces: `store.Open(databaseURL string) (*sql.DB, error)`, `store.Migrate(db *sql.DB) error`, `store.New(db *sql.DB) *Store`, `type Store struct{ db *sql.DB }`. Every later store task (4-9) adds methods on `*Store` in its own file within this package.

- [ ] **Step 1: Add the sqlite driver dependency**

Run: `cd siem-api && go get modernc.org/sqlite`

- [ ] **Step 2: Copy the embed source and write the failing test**

Copy `siem-api/schema.sql` to `siem-api/internal/store/schema.sql` (identical content — the top-level copy is the human-facing reference per the design doc, this one is what `go:embed` reads; Task 9 must update both when it adds `seen_values` — it's already included in step 3 of Task 1, so this copy already has it).

`siem-api/internal/store/store_test.go`:
```go
package store

import (
    "path/filepath"
    "testing"
)

func TestOpenAndMigrate(t *testing.T) {
    dbPath := filepath.Join(t.TempDir(), "siem.db")
    db, err := Open("sqlite://" + dbPath)
    if err != nil {
        t.Fatalf("Open() error = %v", err)
    }
    defer db.Close()

    if err := Migrate(db); err != nil {
        t.Fatalf("Migrate() error = %v", err)
    }

    tables := []string{"sources", "rules", "alerts", "alert_samples", "users",
        "role_mappings", "saved_searches", "audit", "seen_values"}
    for _, table := range tables {
        var name string
        err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&name)
        if err != nil {
            t.Errorf("table %q not found after Migrate(): %v", table, err)
        }
    }
}

func TestMigrate_Idempotent(t *testing.T) {
    dbPath := filepath.Join(t.TempDir(), "siem.db")
    db, err := Open("sqlite://" + dbPath)
    if err != nil {
        t.Fatalf("Open() error = %v", err)
    }
    defer db.Close()

    if err := Migrate(db); err != nil {
        t.Fatalf("first Migrate() error = %v", err)
    }
    if err := Migrate(db); err != nil {
        t.Fatalf("second Migrate() error = %v, want nil (must be idempotent)", err)
    }
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `cd siem-api && go test ./internal/store/... -v`
Expected: FAIL — `Open`/`Migrate` undefined.

- [ ] **Step 4: Write the implementation**

`siem-api/internal/store/store.go`:
```go
package store

import (
    "database/sql"
    _ "embed"
    "fmt"
    "strings"

    _ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaSQL string

type Store struct {
    db *sql.DB
}

func New(db *sql.DB) *Store {
    return &Store{db: db}
}

// Open accepts a DATABASE_URL of the form "sqlite:///path/to/file.db"
// (query params, if any, are ignored — pragmas are applied explicitly
// below rather than via DSN, since that's portable across driver versions).
func Open(databaseURL string) (*sql.DB, error) {
    path := strings.TrimPrefix(databaseURL, "sqlite://")
    if idx := strings.IndexByte(path, '?'); idx >= 0 {
        path = path[:idx]
    }

    db, err := sql.Open("sqlite", path)
    if err != nil {
        return nil, fmt.Errorf("store: open %s: %w", path, err)
    }

    for _, pragma := range []string{
        "PRAGMA journal_mode = WAL",
        "PRAGMA busy_timeout = 5000",
        "PRAGMA foreign_keys = ON",
    } {
        if _, err := db.Exec(pragma); err != nil {
            db.Close()
            return nil, fmt.Errorf("store: %s: %w", pragma, err)
        }
    }

    // SQLite locking is unreliable across multiple connections issuing
    // concurrent writes; the schema is designed single-writer, so pin
    // the pool to one connection to avoid SQLITE_BUSY under load.
    db.SetMaxOpenConns(1)

    return db, nil
}

// Migrate applies schema.sql if the schema hasn't been created yet.
// Idempotent: safe to call on every startup.
func Migrate(db *sql.DB) error {
    var exists int
    err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='sources'`).Scan(&exists)
    if err != nil {
        return fmt.Errorf("store: check schema: %w", err)
    }
    if exists > 0 {
        return nil
    }

    if _, err := db.Exec(schemaSQL); err != nil {
        return fmt.Errorf("store: apply schema: %w", err)
    }
    return nil
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `cd siem-api && go test ./internal/store/... -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add siem-api/go.mod siem-api/go.sum siem-api/internal/store
git commit -m "Add siem-api store: DB open, pragmas, schema migration"
```

---

### Task 4: `store` — audit log

**Files:**
- Create: `siem-api/internal/store/audit.go`
- Test: `siem-api/internal/store/audit_test.go`

**Interfaces:**
- Consumes: `Store` from Task 3.
- Produces: `AuditEntry` struct, `(s *Store) ListAudit(ctx, limit int) ([]AuditEntry, error)`, and the unexported `writeAudit(tx *sql.Tx, e AuditEntry) error` helper that Tasks 6-8 use internally to write an audit row in the same transaction as the state change it records.

```go
type AuditEntry struct {
    ID     int64
    TS     time.Time
    UserID *int64
    Action string
    Target *string
    Detail string // JSON
}
```

- [ ] **Step 1: Write the failing test**

`siem-api/internal/store/audit_test.go`:
```go
package store

import (
    "context"
    "path/filepath"
    "testing"
)

func newTestStore(t *testing.T) *Store {
    t.Helper()
    dbPath := filepath.Join(t.TempDir(), "siem.db")
    db, err := Open("sqlite://" + dbPath)
    if err != nil {
        t.Fatalf("Open() error = %v", err)
    }
    t.Cleanup(func() { db.Close() })
    if err := Migrate(db); err != nil {
        t.Fatalf("Migrate() error = %v", err)
    }
    return New(db)
}

func TestWriteAuditAndList(t *testing.T) {
    s := newTestStore(t)
    ctx := context.Background()

    tx, err := s.db.BeginTx(ctx, nil)
    if err != nil {
        t.Fatalf("BeginTx() error = %v", err)
    }
    target := "rule:1"
    if err := writeAudit(tx, AuditEntry{Action: "rule.create", Target: &target, Detail: `{"name":"wan-portscan"}`}); err != nil {
        t.Fatalf("writeAudit() error = %v", err)
    }
    if err := tx.Commit(); err != nil {
        t.Fatalf("Commit() error = %v", err)
    }

    entries, err := s.ListAudit(ctx, 10)
    if err != nil {
        t.Fatalf("ListAudit() error = %v", err)
    }
    if len(entries) != 1 {
        t.Fatalf("len(entries) = %d, want 1", len(entries))
    }
    if entries[0].Action != "rule.create" {
        t.Errorf("Action = %q, want rule.create", entries[0].Action)
    }
    if entries[0].Target == nil || *entries[0].Target != "rule:1" {
        t.Errorf("Target = %v, want rule:1", entries[0].Target)
    }
}

func TestWriteAudit_RolledBackNotListed(t *testing.T) {
    s := newTestStore(t)
    ctx := context.Background()

    tx, err := s.db.BeginTx(ctx, nil)
    if err != nil {
        t.Fatalf("BeginTx() error = %v", err)
    }
    if err := writeAudit(tx, AuditEntry{Action: "rule.create", Detail: "{}"}); err != nil {
        t.Fatalf("writeAudit() error = %v", err)
    }
    if err := tx.Rollback(); err != nil {
        t.Fatalf("Rollback() error = %v", err)
    }

    entries, err := s.ListAudit(ctx, 10)
    if err != nil {
        t.Fatalf("ListAudit() error = %v", err)
    }
    if len(entries) != 0 {
        t.Fatalf("len(entries) = %d, want 0 after rollback", len(entries))
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd siem-api && go test ./internal/store/... -run Audit -v`
Expected: FAIL — `writeAudit`/`AuditEntry`/`ListAudit` undefined.

- [ ] **Step 3: Write the implementation**

`siem-api/internal/store/audit.go`:
```go
package store

import (
    "context"
    "database/sql"
    "time"
)

type AuditEntry struct {
    ID     int64
    TS     time.Time
    UserID *int64
    Action string
    Target *string
    Detail string
}

func writeAudit(tx *sql.Tx, e AuditEntry) error {
    _, err := tx.Exec(
        `INSERT INTO audit (user_id, action, target, detail) VALUES (?, ?, ?, ?)`,
        e.UserID, e.Action, e.Target, e.Detail,
    )
    return err
}

func (s *Store) ListAudit(ctx context.Context, limit int) ([]AuditEntry, error) {
    rows, err := s.db.QueryContext(ctx,
        `SELECT id, ts, user_id, action, target, detail FROM audit ORDER BY ts DESC LIMIT ?`, limit)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var entries []AuditEntry
    for rows.Next() {
        var e AuditEntry
        var ts string
        if err := rows.Scan(&e.ID, &ts, &e.UserID, &e.Action, &e.Target, &e.Detail); err != nil {
            return nil, err
        }
        e.TS, err = time.Parse("2006-01-02 15:04:05", ts)
        if err != nil {
            return nil, err
        }
        entries = append(entries, e)
    }
    return entries, rows.Err()
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd siem-api && go test ./internal/store/... -run Audit -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add siem-api/internal/store/audit.go siem-api/internal/store/audit_test.go
git commit -m "Add siem-api store: audit log"
```

---

### Task 5: `store` — sources

**Files:**
- Create: `siem-api/internal/store/sources.go`
- Test: `siem-api/internal/store/sources_test.go`

**Interfaces:**
- Consumes: `Store` from Task 3.
- Produces: `Source` struct and methods below, consumed by Task 20 (`AbsenceEvaluator`), Task 27 (`/sources` handlers), and the fastpath ingest handler (Task 23) for `TouchSourceLastSeen`.

```go
type Source struct {
    ID           int64
    Name         string
    Address      string
    Transport    string
    Parser       string
    Claimed      bool
    HeartbeatSec int
    LastSeenAt   *time.Time
    CreatedAt    time.Time
}
func (s *Store) ListSources(ctx context.Context) ([]Source, error)
func (s *Store) UpsertSource(ctx context.Context, src Source) (Source, error)
func (s *Store) TouchSourceLastSeen(ctx context.Context, name string, at time.Time) error
func (s *Store) ClaimSource(ctx context.Context, id int64) error
func (s *Store) StaleSources(ctx context.Context, now time.Time) ([]Source, error)
```

- [ ] **Step 1: Write the failing test**

`siem-api/internal/store/sources_test.go`:
```go
package store

import (
    "context"
    "testing"
    "time"
)

func TestUpsertAndListSources(t *testing.T) {
    s := newTestStore(t)
    ctx := context.Background()

    created, err := s.UpsertSource(ctx, Source{
        Name: "udm-ultra", Address: "10.0.0.1", Transport: "udp/514",
        Parser: "unifi-os", HeartbeatSec: 900,
    })
    if err != nil {
        t.Fatalf("UpsertSource() error = %v", err)
    }
    if created.ID == 0 {
        t.Error("UpsertSource() ID = 0, want nonzero")
    }

    // Upsert again with same name should update, not duplicate.
    _, err = s.UpsertSource(ctx, Source{
        Name: "udm-ultra", Address: "10.0.0.1", Transport: "udp/514",
        Parser: "unifi-os", HeartbeatSec: 600,
    })
    if err != nil {
        t.Fatalf("second UpsertSource() error = %v", err)
    }

    sources, err := s.ListSources(ctx)
    if err != nil {
        t.Fatalf("ListSources() error = %v", err)
    }
    if len(sources) != 1 {
        t.Fatalf("len(sources) = %d, want 1", len(sources))
    }
    if sources[0].HeartbeatSec != 600 {
        t.Errorf("HeartbeatSec = %d, want 600 (upsert should update)", sources[0].HeartbeatSec)
    }
}

func TestTouchSourceLastSeen(t *testing.T) {
    s := newTestStore(t)
    ctx := context.Background()

    if _, err := s.UpsertSource(ctx, Source{Name: "udm-ultra", Address: "10.0.0.1", Transport: "udp/514", Parser: "unifi-os", HeartbeatSec: 900}); err != nil {
        t.Fatalf("UpsertSource() error = %v", err)
    }

    now := time.Now().UTC().Truncate(time.Second)
    if err := s.TouchSourceLastSeen(ctx, "udm-ultra", now); err != nil {
        t.Fatalf("TouchSourceLastSeen() error = %v", err)
    }

    sources, err := s.ListSources(ctx)
    if err != nil {
        t.Fatalf("ListSources() error = %v", err)
    }
    if sources[0].LastSeenAt == nil {
        t.Fatal("LastSeenAt is nil after TouchSourceLastSeen")
    }
}

func TestClaimSource(t *testing.T) {
    s := newTestStore(t)
    ctx := context.Background()

    created, err := s.UpsertSource(ctx, Source{Name: "unclaimed-host", Address: "10.0.0.2", Transport: "tcp/601", Parser: "rfc5424", HeartbeatSec: 900})
    if err != nil {
        t.Fatalf("UpsertSource() error = %v", err)
    }
    if err := s.ClaimSource(ctx, created.ID); err != nil {
        t.Fatalf("ClaimSource() error = %v", err)
    }

    sources, err := s.ListSources(ctx)
    if err != nil {
        t.Fatalf("ListSources() error = %v", err)
    }
    if !sources[0].Claimed {
        t.Error("Claimed = false after ClaimSource()")
    }
}

func TestStaleSources(t *testing.T) {
    s := newTestStore(t)
    ctx := context.Background()

    if _, err := s.UpsertSource(ctx, Source{Name: "silent-host", Address: "10.0.0.3", Transport: "tcp/601", Parser: "rfc5424", HeartbeatSec: 60}); err != nil {
        t.Fatalf("UpsertSource() error = %v", err)
    }
    old := time.Now().UTC().Add(-2 * time.Hour)
    if err := s.TouchSourceLastSeen(ctx, "silent-host", old); err != nil {
        t.Fatalf("TouchSourceLastSeen() error = %v", err)
    }

    stale, err := s.StaleSources(ctx, time.Now().UTC())
    if err != nil {
        t.Fatalf("StaleSources() error = %v", err)
    }
    if len(stale) != 1 || stale[0].Name != "silent-host" {
        t.Fatalf("StaleSources() = %+v, want [silent-host]", stale)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd siem-api && go test ./internal/store/... -run Source -v`
Expected: FAIL — `Source`/`UpsertSource`/etc. undefined.

- [ ] **Step 3: Write the implementation**

`siem-api/internal/store/sources.go`:
```go
package store

import (
    "context"
    "database/sql"
    "time"
)

type Source struct {
    ID           int64
    Name         string
    Address      string
    Transport    string
    Parser       string
    Claimed      bool
    HeartbeatSec int
    LastSeenAt   *time.Time
    CreatedAt    time.Time
}

func (s *Store) UpsertSource(ctx context.Context, src Source) (Source, error) {
    _, err := s.db.ExecContext(ctx, `
        INSERT INTO sources (name, address, transport, parser, heartbeat_sec)
        VALUES (?, ?, ?, ?, ?)
        ON CONFLICT(name) DO UPDATE SET
            address = excluded.address,
            transport = excluded.transport,
            parser = excluded.parser,
            heartbeat_sec = excluded.heartbeat_sec
    `, src.Name, src.Address, src.Transport, src.Parser, src.HeartbeatSec)
    if err != nil {
        return Source{}, err
    }

    var out Source
    err = s.db.QueryRowContext(ctx,
        `SELECT id, name, address, transport, parser, claimed, heartbeat_sec, last_seen_at, created_at
         FROM sources WHERE name = ?`, src.Name).
        Scan(&out.ID, &out.Name, &out.Address, &out.Transport, &out.Parser,
            &out.Claimed, &out.HeartbeatSec, scanNullTime(&out.LastSeenAt), scanTime(&out.CreatedAt))
    return out, err
}

func (s *Store) ListSources(ctx context.Context) ([]Source, error) {
    rows, err := s.db.QueryContext(ctx,
        `SELECT id, name, address, transport, parser, claimed, heartbeat_sec, last_seen_at, created_at
         FROM sources ORDER BY name`)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var out []Source
    for rows.Next() {
        var src Source
        if err := rows.Scan(&src.ID, &src.Name, &src.Address, &src.Transport, &src.Parser,
            &src.Claimed, &src.HeartbeatSec, scanNullTime(&src.LastSeenAt), scanTime(&src.CreatedAt)); err != nil {
            return nil, err
        }
        out = append(out, src)
    }
    return out, rows.Err()
}

func (s *Store) TouchSourceLastSeen(ctx context.Context, name string, at time.Time) error {
    _, err := s.db.ExecContext(ctx,
        `UPDATE sources SET last_seen_at = ? WHERE name = ?`, formatTime(at), name)
    return err
}

func (s *Store) ClaimSource(ctx context.Context, id int64) error {
    _, err := s.db.ExecContext(ctx, `UPDATE sources SET claimed = 1 WHERE id = ?`, id)
    return err
}

func (s *Store) StaleSources(ctx context.Context, now time.Time) ([]Source, error) {
    rows, err := s.db.QueryContext(ctx, `
        SELECT id, name, address, transport, parser, claimed, heartbeat_sec, last_seen_at, created_at
        FROM sources
        WHERE last_seen_at IS NULL
           OR (julianday(?) - julianday(last_seen_at)) * 86400 > heartbeat_sec
    `, formatTime(now))
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var out []Source
    for rows.Next() {
        var src Source
        if err := rows.Scan(&src.ID, &src.Name, &src.Address, &src.Transport, &src.Parser,
            &src.Claimed, &src.HeartbeatSec, scanNullTime(&src.LastSeenAt), scanTime(&src.CreatedAt)); err != nil {
            return nil, err
        }
        out = append(out, src)
    }
    return out, rows.Err()
}

// -- time helpers shared across store files --

func formatTime(t time.Time) string {
    return t.UTC().Format("2006-01-02 15:04:05")
}

func scanTime(dst *time.Time) *timeScanner {
    return &timeScanner{dst: dst}
}

func scanNullTime(dst **time.Time) *nullTimeScanner {
    return &nullTimeScanner{dst: dst}
}

type timeScanner struct{ dst *time.Time }

func (ts *timeScanner) Scan(src any) error {
    s, ok := src.(string)
    if !ok {
        return sql.ErrNoRows
    }
    t, err := time.Parse("2006-01-02 15:04:05", s)
    if err != nil {
        return err
    }
    *ts.dst = t
    return nil
}

type nullTimeScanner struct{ dst **time.Time }

func (ns *nullTimeScanner) Scan(src any) error {
    if src == nil {
        *ns.dst = nil
        return nil
    }
    s, ok := src.(string)
    if !ok {
        return sql.ErrNoRows
    }
    t, err := time.Parse("2006-01-02 15:04:05", s)
    if err != nil {
        return err
    }
    *ns.dst = &t
    return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd siem-api && go test ./internal/store/... -run Source -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add siem-api/internal/store/sources.go siem-api/internal/store/sources_test.go
git commit -m "Add siem-api store: sources"
```

---

### Task 6: `store` — rules

**Files:**
- Create: `siem-api/internal/store/rules.go`
- Test: `siem-api/internal/store/rules_test.go`

**Interfaces:**
- Consumes: `Store` (Task 3), `writeAudit`/`AuditEntry` (Task 4), `formatTime`/`scanTime`/`scanNullTime` (Task 5).
- Produces: `Rule` struct and methods below. Consumed by Task 21 (`Scheduler`), Tasks 18-20 (evaluators receive a `Rule` per evaluation), Task 26 (`/rules` handlers).

```go
type Rule struct {
    ID           int64
    Name         string
    Shape        string // "threshold" | "first_seen" | "absence"
    LogQL        string
    WindowSec    int
    Threshold    *int
    GroupBy      []string
    Severity     string
    Destinations []string
    CooldownSec  int
    IntervalSec  int
    Enabled      bool
    LastRunAt    *time.Time
    CreatedBy    *int64
    CreatedAt    time.Time
}
func (s *Store) ListRules(ctx context.Context) ([]Rule, error)
func (s *Store) ListEnabledRules(ctx context.Context) ([]Rule, error)
func (s *Store) GetRule(ctx context.Context, id int64) (Rule, error)
func (s *Store) CreateRule(ctx context.Context, r Rule, actorUserID *int64) (Rule, error)
func (s *Store) UpdateRule(ctx context.Context, r Rule, actorUserID *int64) (Rule, error)
func (s *Store) DeleteRule(ctx context.Context, id int64, actorUserID *int64) error
func (s *Store) TouchRuleLastRun(ctx context.Context, id int64, at time.Time) error
```

`GroupBy` and `Destinations` are stored as JSON text in the `group_by`/`destinations` columns (schema comment: "JSON array") and marshaled/unmarshaled at the store boundary — callers never see JSON strings.

- [ ] **Step 1: Write the failing test**

`siem-api/internal/store/rules_test.go`:
```go
package store

import (
    "context"
    "testing"
)

func TestCreateAndGetRule(t *testing.T) {
    s := newTestStore(t)
    ctx := context.Background()

    threshold := 5
    created, err := s.CreateRule(ctx, Rule{
        Name: "wan-portscan", Shape: "threshold", LogQL: `{job="siem"} |= "DROP"`,
        WindowSec: 60, Threshold: &threshold, GroupBy: []string{"src_ip"},
        Severity: "critical", Destinations: []string{"inapp", "ntfy"},
        CooldownSec: 3600, IntervalSec: 60, Enabled: true,
    }, nil)
    if err != nil {
        t.Fatalf("CreateRule() error = %v", err)
    }
    if created.ID == 0 {
        t.Error("CreateRule() ID = 0, want nonzero")
    }

    got, err := s.GetRule(ctx, created.ID)
    if err != nil {
        t.Fatalf("GetRule() error = %v", err)
    }
    if got.Name != "wan-portscan" || got.Shape != "threshold" {
        t.Errorf("GetRule() = %+v", got)
    }
    if len(got.GroupBy) != 1 || got.GroupBy[0] != "src_ip" {
        t.Errorf("GroupBy = %v, want [src_ip]", got.GroupBy)
    }
    if got.Threshold == nil || *got.Threshold != 5 {
        t.Errorf("Threshold = %v, want 5", got.Threshold)
    }
}

func TestListEnabledRules(t *testing.T) {
    s := newTestStore(t)
    ctx := context.Background()

    if _, err := s.CreateRule(ctx, Rule{Name: "on", Shape: "absence", Severity: "low",
        Destinations: []string{"inapp"}, CooldownSec: 60, IntervalSec: 60, Enabled: true}, nil); err != nil {
        t.Fatalf("CreateRule() error = %v", err)
    }
    if _, err := s.CreateRule(ctx, Rule{Name: "off", Shape: "absence", Severity: "low",
        Destinations: []string{"inapp"}, CooldownSec: 60, IntervalSec: 60, Enabled: false}, nil); err != nil {
        t.Fatalf("CreateRule() error = %v", err)
    }

    enabled, err := s.ListEnabledRules(ctx)
    if err != nil {
        t.Fatalf("ListEnabledRules() error = %v", err)
    }
    if len(enabled) != 1 || enabled[0].Name != "on" {
        t.Fatalf("ListEnabledRules() = %+v, want [on]", enabled)
    }
}

func TestUpdateAndDeleteRule(t *testing.T) {
    s := newTestStore(t)
    ctx := context.Background()

    created, err := s.CreateRule(ctx, Rule{Name: "r1", Shape: "absence", Severity: "low",
        Destinations: []string{"inapp"}, CooldownSec: 60, IntervalSec: 60, Enabled: true}, nil)
    if err != nil {
        t.Fatalf("CreateRule() error = %v", err)
    }

    created.Enabled = false
    updated, err := s.UpdateRule(ctx, created, nil)
    if err != nil {
        t.Fatalf("UpdateRule() error = %v", err)
    }
    if updated.Enabled {
        t.Error("UpdateRule() Enabled = true, want false")
    }

    if err := s.DeleteRule(ctx, created.ID, nil); err != nil {
        t.Fatalf("DeleteRule() error = %v", err)
    }
    if _, err := s.GetRule(ctx, created.ID); err == nil {
        t.Fatal("GetRule() after delete: error = nil, want not found")
    }

    entries, err := s.ListAudit(ctx, 10)
    if err != nil {
        t.Fatalf("ListAudit() error = %v", err)
    }
    var actions []string
    for _, e := range entries {
        actions = append(actions, e.Action)
    }
    wantContains := []string{"rule.create", "rule.update", "rule.delete"}
    for _, want := range wantContains {
        found := false
        for _, a := range actions {
            if a == want {
                found = true
            }
        }
        if !found {
            t.Errorf("audit actions = %v, want to contain %q", actions, want)
        }
    }
}

func TestTouchRuleLastRun(t *testing.T) {
    s := newTestStore(t)
    ctx := context.Background()

    created, err := s.CreateRule(ctx, Rule{Name: "r1", Shape: "absence", Severity: "low",
        Destinations: []string{"inapp"}, CooldownSec: 60, IntervalSec: 60, Enabled: true}, nil)
    if err != nil {
        t.Fatalf("CreateRule() error = %v", err)
    }

    if err := s.TouchRuleLastRun(ctx, created.ID, time.Now()); err != nil {
        t.Fatalf("TouchRuleLastRun() error = %v", err)
    }
    got, err := s.GetRule(ctx, created.ID)
    if err != nil {
        t.Fatalf("GetRule() error = %v", err)
    }
    if got.LastRunAt == nil {
        t.Error("LastRunAt is nil after TouchRuleLastRun")
    }
}
```

`rules_test.go`'s import block needs `"time"` alongside `"context"` and `"testing"` for the `time.Now()` call above:
```go
import (
    "context"
    "testing"
    "time"
)
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd siem-api && go test ./internal/store/... -run Rule -v`
Expected: FAIL — `Rule`/`CreateRule`/etc. undefined.

- [ ] **Step 3: Write the implementation**

`siem-api/internal/store/rules.go`:
```go
package store

import (
    "context"
    "database/sql"
    "encoding/json"
    "errors"
    "strconv"
    "time"
)

type Rule struct {
    ID           int64
    Name         string
    Shape        string
    LogQL        string
    WindowSec    int
    Threshold    *int
    GroupBy      []string
    Severity     string
    Destinations []string
    CooldownSec  int
    IntervalSec  int
    Enabled      bool
    LastRunAt    *time.Time
    CreatedBy    *int64
    CreatedAt    time.Time
}

func (s *Store) CreateRule(ctx context.Context, r Rule, actorUserID *int64) (Rule, error) {
    groupBy, err := json.Marshal(r.GroupBy)
    if err != nil {
        return Rule{}, err
    }
    dests, err := json.Marshal(r.Destinations)
    if err != nil {
        return Rule{}, err
    }

    tx, err := s.db.BeginTx(ctx, nil)
    if err != nil {
        return Rule{}, err
    }
    defer tx.Rollback()

    res, err := tx.ExecContext(ctx, `
        INSERT INTO rules (name, shape, logql, window_sec, threshold, group_by, severity,
            destinations, cooldown_sec, interval_sec, enabled, created_by)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
    `, r.Name, r.Shape, r.LogQL, r.WindowSec, r.Threshold, string(groupBy), r.Severity,
        string(dests), r.CooldownSec, r.IntervalSec, r.Enabled, r.CreatedBy)
    if err != nil {
        return Rule{}, err
    }
    id, err := res.LastInsertId()
    if err != nil {
        return Rule{}, err
    }

    target := ruleTarget(id)
    detail, _ := json.Marshal(map[string]any{"name": r.Name, "shape": r.Shape})
    if err := writeAudit(tx, AuditEntry{UserID: actorUserID, Action: "rule.create", Target: &target, Detail: string(detail)}); err != nil {
        return Rule{}, err
    }
    if err := tx.Commit(); err != nil {
        return Rule{}, err
    }

    return s.GetRule(ctx, id)
}

func (s *Store) UpdateRule(ctx context.Context, r Rule, actorUserID *int64) (Rule, error) {
    groupBy, err := json.Marshal(r.GroupBy)
    if err != nil {
        return Rule{}, err
    }
    dests, err := json.Marshal(r.Destinations)
    if err != nil {
        return Rule{}, err
    }

    tx, err := s.db.BeginTx(ctx, nil)
    if err != nil {
        return Rule{}, err
    }
    defer tx.Rollback()

    _, err = tx.ExecContext(ctx, `
        UPDATE rules SET name=?, shape=?, logql=?, window_sec=?, threshold=?, group_by=?,
            severity=?, destinations=?, cooldown_sec=?, interval_sec=?, enabled=?
        WHERE id=?
    `, r.Name, r.Shape, r.LogQL, r.WindowSec, r.Threshold, string(groupBy), r.Severity,
        string(dests), r.CooldownSec, r.IntervalSec, r.Enabled, r.ID)
    if err != nil {
        return Rule{}, err
    }

    target := ruleTarget(r.ID)
    detail, _ := json.Marshal(map[string]any{"name": r.Name, "enabled": r.Enabled})
    if err := writeAudit(tx, AuditEntry{UserID: actorUserID, Action: "rule.update", Target: &target, Detail: string(detail)}); err != nil {
        return Rule{}, err
    }
    if err := tx.Commit(); err != nil {
        return Rule{}, err
    }

    return s.GetRule(ctx, r.ID)
}

func (s *Store) DeleteRule(ctx context.Context, id int64, actorUserID *int64) error {
    tx, err := s.db.BeginTx(ctx, nil)
    if err != nil {
        return err
    }
    defer tx.Rollback()

    if _, err := tx.ExecContext(ctx, `DELETE FROM rules WHERE id=?`, id); err != nil {
        return err
    }

    target := ruleTarget(id)
    if err := writeAudit(tx, AuditEntry{UserID: actorUserID, Action: "rule.delete", Target: &target, Detail: "{}"}); err != nil {
        return err
    }
    return tx.Commit()
}

func (s *Store) GetRule(ctx context.Context, id int64) (Rule, error) {
    row := s.db.QueryRowContext(ctx, ruleSelect+` WHERE id = ?`, id)
    r, err := scanRule(row)
    if errors.Is(err, sql.ErrNoRows) {
        return Rule{}, err
    }
    return r, err
}

func (s *Store) ListRules(ctx context.Context) ([]Rule, error) {
    return s.queryRules(ctx, ruleSelect+` ORDER BY name`)
}

func (s *Store) ListEnabledRules(ctx context.Context) ([]Rule, error) {
    return s.queryRules(ctx, ruleSelect+` WHERE enabled = 1 ORDER BY name`)
}

func (s *Store) TouchRuleLastRun(ctx context.Context, id int64, at time.Time) error {
    _, err := s.db.ExecContext(ctx, `UPDATE rules SET last_run_at = ? WHERE id = ?`, formatTime(at), id)
    return err
}

const ruleSelect = `SELECT id, name, shape, logql, window_sec, threshold, group_by, severity,
    destinations, cooldown_sec, interval_sec, enabled, last_run_at, created_by, created_at
    FROM rules`

type rowScanner interface {
    Scan(dest ...any) error
}

func scanRule(row rowScanner) (Rule, error) {
    var r Rule
    var groupBy, dests string
    if err := row.Scan(&r.ID, &r.Name, &r.Shape, &r.LogQL, &r.WindowSec, &r.Threshold,
        &groupBy, &r.Severity, &dests, &r.CooldownSec, &r.IntervalSec, &r.Enabled,
        scanNullTime(&r.LastRunAt), &r.CreatedBy, scanTime(&r.CreatedAt)); err != nil {
        return Rule{}, err
    }
    if err := json.Unmarshal([]byte(groupBy), &r.GroupBy); err != nil {
        return Rule{}, err
    }
    if err := json.Unmarshal([]byte(dests), &r.Destinations); err != nil {
        return Rule{}, err
    }
    return r, nil
}

func (s *Store) queryRules(ctx context.Context, query string) ([]Rule, error) {
    rows, err := s.db.QueryContext(ctx, query)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var out []Rule
    for rows.Next() {
        r, err := scanRule(rows)
        if err != nil {
            return nil, err
        }
        out = append(out, r)
    }
    return out, rows.Err()
}

func ruleTarget(id int64) string {
    return "rule:" + strconv.FormatInt(id, 10)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd siem-api && go test ./internal/store/... -run Rule -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add siem-api/internal/store/rules.go siem-api/internal/store/rules_test.go
git commit -m "Add siem-api store: rules"
```

---

### Task 7: `store` — alerts and alert samples

**Files:**
- Create: `siem-api/internal/store/alerts.go`
- Test: `siem-api/internal/store/alerts_test.go`

**Interfaces:**
- Consumes: `Store` (Task 3), `writeAudit`/`AuditEntry` (Task 4), `formatTime`/`scanTime`/`scanNullTime` (Task 5), `Rule` (Task 6, for the `rule_id` foreign key in tests).
- Produces: `Alert`/`AlertSample` structs and methods below. Consumed by Task 17 (`alerts.Service`), Task 25 (`/alerts` handlers).

```go
type Alert struct {
    ID          int64
    RuleID      int64
    GroupKey    string
    Severity    string
    Title       string
    Body        string
    EventCount  int
    Context     string // JSON
    State       string // open|acked|muted|closed
    FirstSeenAt time.Time
    LastSeenAt  time.Time
    AckedBy     *int64
    AckedAt     *time.Time
}
type AlertSample struct {
    ID      int64
    AlertID int64
    TS      time.Time
    Line    string
}
func (s *Store) FindOpenAlert(ctx context.Context, ruleID int64, groupKey string) (*Alert, error)
func (s *Store) InsertAlert(ctx context.Context, a Alert) (Alert, error)
func (s *Store) TouchAlert(ctx context.Context, id int64, at time.Time) error
func (s *Store) ReopenAlert(ctx context.Context, id int64, at time.Time) error
func (s *Store) AddAlertSample(ctx context.Context, alertID int64, ts time.Time, line string) error
func (s *Store) AckAlert(ctx context.Context, id int64, userID int64, at time.Time) error
func (s *Store) ListAlerts(ctx context.Context, state string) ([]Alert, error)
func (s *Store) GetAlert(ctx context.Context, id int64) (Alert, error)
func (s *Store) ListAlertSamples(ctx context.Context, alertID int64) ([]AlertSample, error)
```

`FindOpenAlert` returns `(nil, nil)` — not an error — when no open alert matches, since "not found" is an expected, common case for the alert-lifecycle caller (Task 17), not a failure.

`AddAlertSample` caps stored samples at 10 per alert: after inserting, it deletes any samples beyond the 10 most recent for that `alert_id`.

- [ ] **Step 1: Write the failing test**

`siem-api/internal/store/alerts_test.go`:
```go
package store

import (
    "context"
    "testing"
    "time"
)

func createTestRule(t *testing.T, s *Store) Rule {
    t.Helper()
    ctx := context.Background()
    r, err := s.CreateRule(ctx, Rule{Name: "wan-portscan", Shape: "threshold", Severity: "critical",
        Destinations: []string{"inapp"}, CooldownSec: 3600, IntervalSec: 60, Enabled: true}, nil)
    if err != nil {
        t.Fatalf("CreateRule() error = %v", err)
    }
    return r
}

func TestFindOpenAlert_NotFound(t *testing.T) {
    s := newTestStore(t)
    ctx := context.Background()
    rule := createTestRule(t, s)

    got, err := s.FindOpenAlert(ctx, rule.ID, "10.0.0.5")
    if err != nil {
        t.Fatalf("FindOpenAlert() error = %v", err)
    }
    if got != nil {
        t.Fatalf("FindOpenAlert() = %+v, want nil", got)
    }
}

func TestInsertAndFindOpenAlert(t *testing.T) {
    s := newTestStore(t)
    ctx := context.Background()
    rule := createTestRule(t, s)
    now := time.Now().UTC()

    inserted, err := s.InsertAlert(ctx, Alert{
        RuleID: rule.ID, GroupKey: "10.0.0.5", Severity: "critical",
        Title: "Port scan from 10.0.0.5", Body: "40 dropped connections in 60s",
        EventCount: 1, Context: "{}", State: "open", FirstSeenAt: now, LastSeenAt: now,
    })
    if err != nil {
        t.Fatalf("InsertAlert() error = %v", err)
    }
    if inserted.ID == 0 {
        t.Error("InsertAlert() ID = 0, want nonzero")
    }

    found, err := s.FindOpenAlert(ctx, rule.ID, "10.0.0.5")
    if err != nil {
        t.Fatalf("FindOpenAlert() error = %v", err)
    }
    if found == nil || found.ID != inserted.ID {
        t.Fatalf("FindOpenAlert() = %+v, want id %d", found, inserted.ID)
    }
}

func TestTouchAlert_IncrementsEventCount(t *testing.T) {
    s := newTestStore(t)
    ctx := context.Background()
    rule := createTestRule(t, s)
    now := time.Now().UTC()

    inserted, err := s.InsertAlert(ctx, Alert{
        RuleID: rule.ID, GroupKey: "10.0.0.5", Severity: "critical", Title: "t", Body: "b",
        EventCount: 1, Context: "{}", State: "open", FirstSeenAt: now, LastSeenAt: now,
    })
    if err != nil {
        t.Fatalf("InsertAlert() error = %v", err)
    }

    later := now.Add(5 * time.Minute)
    if err := s.TouchAlert(ctx, inserted.ID, later); err != nil {
        t.Fatalf("TouchAlert() error = %v", err)
    }

    got, err := s.GetAlert(ctx, inserted.ID)
    if err != nil {
        t.Fatalf("GetAlert() error = %v", err)
    }
    if got.EventCount != 2 {
        t.Errorf("EventCount = %d, want 2", got.EventCount)
    }
}

func TestAddAlertSample_CapsAtTen(t *testing.T) {
    s := newTestStore(t)
    ctx := context.Background()
    rule := createTestRule(t, s)
    now := time.Now().UTC()

    inserted, err := s.InsertAlert(ctx, Alert{
        RuleID: rule.ID, GroupKey: "10.0.0.5", Severity: "critical", Title: "t", Body: "b",
        EventCount: 1, Context: "{}", State: "open", FirstSeenAt: now, LastSeenAt: now,
    })
    if err != nil {
        t.Fatalf("InsertAlert() error = %v", err)
    }

    for i := 0; i < 15; i++ {
        ts := now.Add(time.Duration(i) * time.Second)
        if err := s.AddAlertSample(ctx, inserted.ID, ts, "line"); err != nil {
            t.Fatalf("AddAlertSample() error = %v", err)
        }
    }

    samples, err := s.ListAlertSamples(ctx, inserted.ID)
    if err != nil {
        t.Fatalf("ListAlertSamples() error = %v", err)
    }
    if len(samples) != 10 {
        t.Fatalf("len(samples) = %d, want 10", len(samples))
    }
}

func TestAckAlert(t *testing.T) {
    s := newTestStore(t)
    ctx := context.Background()
    rule := createTestRule(t, s)
    now := time.Now().UTC()

    inserted, err := s.InsertAlert(ctx, Alert{
        RuleID: rule.ID, GroupKey: "10.0.0.5", Severity: "critical", Title: "t", Body: "b",
        EventCount: 1, Context: "{}", State: "open", FirstSeenAt: now, LastSeenAt: now,
    })
    if err != nil {
        t.Fatalf("InsertAlert() error = %v", err)
    }

    if err := s.AckAlert(ctx, inserted.ID, 1, now); err != nil {
        t.Fatalf("AckAlert() error = %v", err)
    }

    got, err := s.GetAlert(ctx, inserted.ID)
    if err != nil {
        t.Fatalf("GetAlert() error = %v", err)
    }
    if got.State != "acked" {
        t.Errorf("State = %q, want acked", got.State)
    }
    if got.AckedBy == nil || *got.AckedBy != 1 {
        t.Errorf("AckedBy = %v, want 1", got.AckedBy)
    }

    entries, err := s.ListAudit(ctx, 10)
    if err != nil {
        t.Fatalf("ListAudit() error = %v", err)
    }
    found := false
    for _, e := range entries {
        if e.Action == "alert.ack" {
            found = true
        }
    }
    if !found {
        t.Error("no alert.ack audit entry found")
    }
}

func TestReopenAlert(t *testing.T) {
    s := newTestStore(t)
    ctx := context.Background()
    rule := createTestRule(t, s)
    now := time.Now().UTC()

    inserted, err := s.InsertAlert(ctx, Alert{
        RuleID: rule.ID, GroupKey: "10.0.0.5", Severity: "critical", Title: "t", Body: "b",
        EventCount: 1, Context: "{}", State: "closed", FirstSeenAt: now, LastSeenAt: now,
    })
    if err != nil {
        t.Fatalf("InsertAlert() error = %v", err)
    }

    later := now.Add(time.Hour)
    if err := s.ReopenAlert(ctx, inserted.ID, later); err != nil {
        t.Fatalf("ReopenAlert() error = %v", err)
    }

    got, err := s.GetAlert(ctx, inserted.ID)
    if err != nil {
        t.Fatalf("GetAlert() error = %v", err)
    }
    if got.State != "open" {
        t.Errorf("State = %q, want open", got.State)
    }
}

func TestListAlerts_FilterByState(t *testing.T) {
    s := newTestStore(t)
    ctx := context.Background()
    rule := createTestRule(t, s)
    now := time.Now().UTC()

    if _, err := s.InsertAlert(ctx, Alert{RuleID: rule.ID, GroupKey: "a", Severity: "low", Title: "t", Body: "b",
        EventCount: 1, Context: "{}", State: "open", FirstSeenAt: now, LastSeenAt: now}); err != nil {
        t.Fatalf("InsertAlert() error = %v", err)
    }
    if _, err := s.InsertAlert(ctx, Alert{RuleID: rule.ID, GroupKey: "b", Severity: "low", Title: "t", Body: "b",
        EventCount: 1, Context: "{}", State: "acked", FirstSeenAt: now, LastSeenAt: now}); err != nil {
        t.Fatalf("InsertAlert() error = %v", err)
    }

    open, err := s.ListAlerts(ctx, "open")
    if err != nil {
        t.Fatalf("ListAlerts() error = %v", err)
    }
    if len(open) != 1 || open[0].GroupKey != "a" {
        t.Fatalf("ListAlerts(open) = %+v, want [a]", open)
    }

    all, err := s.ListAlerts(ctx, "")
    if err != nil {
        t.Fatalf("ListAlerts() error = %v", err)
    }
    if len(all) != 2 {
        t.Fatalf("ListAlerts(\"\") len = %d, want 2", len(all))
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd siem-api && go test ./internal/store/... -run Alert -v`
Expected: FAIL — `Alert`/`InsertAlert`/etc. undefined.

- [ ] **Step 3: Write the implementation**

`siem-api/internal/store/alerts.go`:
```go
package store

import (
    "context"
    "database/sql"
    "errors"
    "time"
)

type Alert struct {
    ID          int64
    RuleID      int64
    GroupKey    string
    Severity    string
    Title       string
    Body        string
    EventCount  int
    Context     string
    State       string
    FirstSeenAt time.Time
    LastSeenAt  time.Time
    AckedBy     *int64
    AckedAt     *time.Time
}

type AlertSample struct {
    ID      int64
    AlertID int64
    TS      time.Time
    Line    string
}

const alertSelect = `SELECT id, rule_id, group_key, severity, title, body, event_count,
    context, state, first_seen_at, last_seen_at, acked_by, acked_at FROM alerts`

func scanAlert(row rowScanner) (Alert, error) {
    var a Alert
    if err := row.Scan(&a.ID, &a.RuleID, &a.GroupKey, &a.Severity, &a.Title, &a.Body,
        &a.EventCount, &a.Context, &a.State, scanTime(&a.FirstSeenAt), scanTime(&a.LastSeenAt),
        &a.AckedBy, scanNullTime(&a.AckedAt)); err != nil {
        return Alert{}, err
    }
    return a, nil
}

func (s *Store) FindOpenAlert(ctx context.Context, ruleID int64, groupKey string) (*Alert, error) {
    row := s.db.QueryRowContext(ctx,
        alertSelect+` WHERE rule_id = ? AND group_key = ? AND state = 'open'`, ruleID, groupKey)
    a, err := scanAlert(row)
    if errors.Is(err, sql.ErrNoRows) {
        return nil, nil
    }
    if err != nil {
        return nil, err
    }
    return &a, nil
}

func (s *Store) InsertAlert(ctx context.Context, a Alert) (Alert, error) {
    res, err := s.db.ExecContext(ctx, `
        INSERT INTO alerts (rule_id, group_key, severity, title, body, event_count, context,
            state, first_seen_at, last_seen_at)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
    `, a.RuleID, a.GroupKey, a.Severity, a.Title, a.Body, a.EventCount, a.Context, a.State,
        formatTime(a.FirstSeenAt), formatTime(a.LastSeenAt))
    if err != nil {
        return Alert{}, err
    }
    id, err := res.LastInsertId()
    if err != nil {
        return Alert{}, err
    }
    return s.GetAlert(ctx, id)
}

func (s *Store) TouchAlert(ctx context.Context, id int64, at time.Time) error {
    _, err := s.db.ExecContext(ctx,
        `UPDATE alerts SET event_count = event_count + 1, last_seen_at = ? WHERE id = ?`,
        formatTime(at), id)
    return err
}

func (s *Store) ReopenAlert(ctx context.Context, id int64, at time.Time) error {
    _, err := s.db.ExecContext(ctx,
        `UPDATE alerts SET state = 'open', last_seen_at = ? WHERE id = ?`, formatTime(at), id)
    return err
}

func (s *Store) AddAlertSample(ctx context.Context, alertID int64, ts time.Time, line string) error {
    tx, err := s.db.BeginTx(ctx, nil)
    if err != nil {
        return err
    }
    defer tx.Rollback()

    if _, err := tx.ExecContext(ctx,
        `INSERT INTO alert_samples (alert_id, ts, line) VALUES (?, ?, ?)`,
        alertID, formatTime(ts), line); err != nil {
        return err
    }

    // Keep only the 10 most recent samples for this alert.
    if _, err := tx.ExecContext(ctx, `
        DELETE FROM alert_samples WHERE alert_id = ? AND id NOT IN (
            SELECT id FROM alert_samples WHERE alert_id = ? ORDER BY ts DESC LIMIT 10
        )
    `, alertID, alertID); err != nil {
        return err
    }

    return tx.Commit()
}

func (s *Store) ListAlertSamples(ctx context.Context, alertID int64) ([]AlertSample, error) {
    rows, err := s.db.QueryContext(ctx,
        `SELECT id, alert_id, ts, line FROM alert_samples WHERE alert_id = ? ORDER BY ts DESC`, alertID)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var out []AlertSample
    for rows.Next() {
        var sample AlertSample
        if err := rows.Scan(&sample.ID, &sample.AlertID, scanTime(&sample.TS), &sample.Line); err != nil {
            return nil, err
        }
        out = append(out, sample)
    }
    return out, rows.Err()
}

func (s *Store) AckAlert(ctx context.Context, id int64, userID int64, at time.Time) error {
    tx, err := s.db.BeginTx(ctx, nil)
    if err != nil {
        return err
    }
    defer tx.Rollback()

    if _, err := tx.ExecContext(ctx,
        `UPDATE alerts SET state = 'acked', acked_by = ?, acked_at = ? WHERE id = ?`,
        userID, formatTime(at), id); err != nil {
        return err
    }

    target := "alert:" + strconvItoa(id)
    uid := userID
    if err := writeAudit(tx, AuditEntry{UserID: &uid, Action: "alert.ack", Target: &target, Detail: "{}"}); err != nil {
        return err
    }
    return tx.Commit()
}

func (s *Store) GetAlert(ctx context.Context, id int64) (Alert, error) {
    row := s.db.QueryRowContext(ctx, alertSelect+` WHERE id = ?`, id)
    return scanAlert(row)
}

func (s *Store) ListAlerts(ctx context.Context, state string) ([]Alert, error) {
    query := alertSelect
    var args []any
    if state != "" {
        query += ` WHERE state = ?`
        args = append(args, state)
    }
    query += ` ORDER BY last_seen_at DESC`

    rows, err := s.db.QueryContext(ctx, query, args...)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var out []Alert
    for rows.Next() {
        a, err := scanAlert(rows)
        if err != nil {
            return nil, err
        }
        out = append(out, a)
    }
    return out, rows.Err()
}
```

`strconvItoa` is a one-line wrapper so `alerts.go` doesn't need a second `strconv` import line duplicated from `rules.go`'s convention — add it once, in `rules.go`, right below `ruleTarget`, and reuse it here:
```go
func strconvItoa(id int64) string {
    return strconv.FormatInt(id, 10)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd siem-api && go test ./internal/store/... -run Alert -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add siem-api/internal/store/alerts.go siem-api/internal/store/alerts_test.go siem-api/internal/store/rules.go
git commit -m "Add siem-api store: alerts and alert samples"
```

---

### Task 8: `store` — users, role mappings, local admin

**Files:**
- Create: `siem-api/internal/store/users.go`
- Test: `siem-api/internal/store/users_test.go`

**Interfaces:**
- Consumes: `Store` (Task 3), `writeAudit`/`AuditEntry` (Task 4), `formatTime`/`scanTime`/`scanNullTime` (Task 5), `strconvItoa` (Task 7).
- Produces: `User`/`RoleMapping` structs and methods below. Consumed by Task 11 (`auth` RBAC middleware, via `ResolveRole`), Task 12 (`auth.LocalAuthenticator`, via `GetLocalAdminByUsername`/`TouchUserLogin`), Task 28 (`/settings/auth` handlers, via `ListRoleMappings`/`UpsertRoleMapping`).

```go
type User struct {
    ID          int64
    Subject     *string
    Email       *string
    DisplayName *string
    Role        string
    LocalHash   *string
    LastLoginAt *time.Time
    CreatedAt   time.Time
}
type RoleMapping struct {
    ID         int64
    GroupClaim string
    Role       string
    Priority   int
}
func (s *Store) UpsertUserBySubject(ctx context.Context, subject, email, displayName, role string) (User, error)
func (s *Store) TouchUserLogin(ctx context.Context, id int64, at time.Time) error
func (s *Store) EnsureLocalAdmin(ctx context.Context, username, passwordHash string) (User, error)
func (s *Store) GetLocalAdminByUsername(ctx context.Context, username string) (*User, error)
func (s *Store) ListRoleMappings(ctx context.Context) ([]RoleMapping, error)
func (s *Store) UpsertRoleMapping(ctx context.Context, m RoleMapping) (RoleMapping, error)
func (s *Store) ResolveRole(ctx context.Context, groups []string) (role string, ok bool)
```

`EnsureLocalAdmin` is idempotent: called on every startup with the configured username/hash, it inserts the break-glass row only if no local-admin row exists yet (`subject IS NULL`), so an operator's manual hash rotation in the DB is never clobbered by a restart. The local-admin `display_name` doubles as the login username, since `users` has no dedicated `username` column.

`ResolveRole` reads `role_mappings` ordered by `priority` ascending (schema comment: "lowest wins" — i.e. first match wins, matching the handoff's "first match wins" RBAC rule) and returns the role of the first row whose `group_claim` is present in `groups`; `ok=false` if nothing matches, signaling deny.

- [ ] **Step 1: Write the failing test**

`siem-api/internal/store/users_test.go`:
```go
package store

import (
    "context"
    "testing"
    "time"
)

func TestUpsertUserBySubject(t *testing.T) {
    s := newTestStore(t)
    ctx := context.Background()

    created, err := s.UpsertUserBySubject(ctx, "oidc-sub-1", "alice@townsville.cc", "Alice", "analyst")
    if err != nil {
        t.Fatalf("UpsertUserBySubject() error = %v", err)
    }
    if created.ID == 0 {
        t.Error("UpsertUserBySubject() ID = 0, want nonzero")
    }

    // Same subject again should update, not duplicate.
    updated, err := s.UpsertUserBySubject(ctx, "oidc-sub-1", "alice@townsville.cc", "Alice", "admin")
    if err != nil {
        t.Fatalf("second UpsertUserBySubject() error = %v", err)
    }
    if updated.ID != created.ID {
        t.Errorf("second UpsertUserBySubject() ID = %d, want %d (same user)", updated.ID, created.ID)
    }
    if updated.Role != "admin" {
        t.Errorf("Role = %q, want admin", updated.Role)
    }
}

func TestTouchUserLogin(t *testing.T) {
    s := newTestStore(t)
    ctx := context.Background()

    created, err := s.UpsertUserBySubject(ctx, "oidc-sub-1", "alice@townsville.cc", "Alice", "analyst")
    if err != nil {
        t.Fatalf("UpsertUserBySubject() error = %v", err)
    }

    if err := s.TouchUserLogin(ctx, created.ID, time.Now().UTC()); err != nil {
        t.Fatalf("TouchUserLogin() error = %v", err)
    }

    entries, err := s.ListAudit(ctx, 10)
    if err != nil {
        t.Fatalf("ListAudit() error = %v", err)
    }
    found := false
    for _, e := range entries {
        if e.Action == "auth.login" {
            found = true
        }
    }
    if !found {
        t.Error("no auth.login audit entry found")
    }
}

func TestEnsureLocalAdmin_IdempotentAndFindable(t *testing.T) {
    s := newTestStore(t)
    ctx := context.Background()

    first, err := s.EnsureLocalAdmin(ctx, "admin", "bcrypt-hash-1")
    if err != nil {
        t.Fatalf("EnsureLocalAdmin() error = %v", err)
    }

    // Calling again with a different hash must NOT overwrite the existing row.
    second, err := s.EnsureLocalAdmin(ctx, "admin", "bcrypt-hash-2")
    if err != nil {
        t.Fatalf("second EnsureLocalAdmin() error = %v", err)
    }
    if second.ID != first.ID {
        t.Errorf("second EnsureLocalAdmin() ID = %d, want %d", second.ID, first.ID)
    }

    found, err := s.GetLocalAdminByUsername(ctx, "admin")
    if err != nil {
        t.Fatalf("GetLocalAdminByUsername() error = %v", err)
    }
    if found == nil {
        t.Fatal("GetLocalAdminByUsername() = nil, want a user")
    }
    if found.LocalHash == nil || *found.LocalHash != "bcrypt-hash-1" {
        t.Errorf("LocalHash = %v, want bcrypt-hash-1 (unchanged by second EnsureLocalAdmin)", found.LocalHash)
    }
}

func TestGetLocalAdminByUsername_NotFound(t *testing.T) {
    s := newTestStore(t)
    ctx := context.Background()

    found, err := s.GetLocalAdminByUsername(ctx, "nobody")
    if err != nil {
        t.Fatalf("GetLocalAdminByUsername() error = %v", err)
    }
    if found != nil {
        t.Fatalf("GetLocalAdminByUsername() = %+v, want nil", found)
    }
}

func TestResolveRole_FirstMatchWinsAndDeny(t *testing.T) {
    s := newTestStore(t)
    ctx := context.Background()

    if _, err := s.UpsertRoleMapping(ctx, RoleMapping{GroupClaim: "admins", Role: "admin", Priority: 10}); err != nil {
        t.Fatalf("UpsertRoleMapping() error = %v", err)
    }
    if _, err := s.UpsertRoleMapping(ctx, RoleMapping{GroupClaim: "homelab", Role: "viewer", Priority: 100}); err != nil {
        t.Fatalf("UpsertRoleMapping() error = %v", err)
    }

    role, ok := s.ResolveRole(ctx, []string{"homelab", "admins"})
    if !ok || role != "admin" {
        t.Errorf("ResolveRole(homelab,admins) = (%q, %v), want (admin, true) — lowest priority wins", role, ok)
    }

    role, ok = s.ResolveRole(ctx, []string{"unmapped-group"})
    if ok {
        t.Errorf("ResolveRole(unmapped-group) ok = true, want false (deny)", )
    }
    _ = role
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd siem-api && go test ./internal/store/... -run "User|LocalAdmin|ResolveRole" -v`
Expected: FAIL — types/methods undefined.

- [ ] **Step 3: Write the implementation**

`siem-api/internal/store/users.go`:
```go
package store

import (
    "context"
    "database/sql"
    "errors"
    "time"
)

type User struct {
    ID          int64
    Subject     *string
    Email       *string
    DisplayName *string
    Role        string
    LocalHash   *string
    LastLoginAt *time.Time
    CreatedAt   time.Time
}

type RoleMapping struct {
    ID         int64
    GroupClaim string
    Role       string
    Priority   int
}

const userSelect = `SELECT id, subject, email, display_name, role, local_hash, last_login_at, created_at FROM users`

func scanUser(row rowScanner) (User, error) {
    var u User
    if err := row.Scan(&u.ID, &u.Subject, &u.Email, &u.DisplayName, &u.Role, &u.LocalHash,
        scanNullTime(&u.LastLoginAt), scanTime(&u.CreatedAt)); err != nil {
        return User{}, err
    }
    return u, nil
}

func (s *Store) UpsertUserBySubject(ctx context.Context, subject, email, displayName, role string) (User, error) {
    _, err := s.db.ExecContext(ctx, `
        INSERT INTO users (subject, email, display_name, role)
        VALUES (?, ?, ?, ?)
        ON CONFLICT(subject) DO UPDATE SET
            email = excluded.email,
            display_name = excluded.display_name,
            role = excluded.role
    `, subject, email, displayName, role)
    if err != nil {
        return User{}, err
    }

    row := s.db.QueryRowContext(ctx, userSelect+` WHERE subject = ?`, subject)
    return scanUser(row)
}

func (s *Store) TouchUserLogin(ctx context.Context, id int64, at time.Time) error {
    tx, err := s.db.BeginTx(ctx, nil)
    if err != nil {
        return err
    }
    defer tx.Rollback()

    if _, err := tx.ExecContext(ctx, `UPDATE users SET last_login_at = ? WHERE id = ?`, formatTime(at), id); err != nil {
        return err
    }

    uid := id
    target := "user:" + strconvItoa(id)
    if err := writeAudit(tx, AuditEntry{UserID: &uid, Action: "auth.login", Target: &target, Detail: "{}"}); err != nil {
        return err
    }
    return tx.Commit()
}

func (s *Store) EnsureLocalAdmin(ctx context.Context, username, passwordHash string) (User, error) {
    existing, err := s.GetLocalAdminByUsername(ctx, username)
    if err != nil {
        return User{}, err
    }
    if existing != nil {
        return *existing, nil
    }

    res, err := s.db.ExecContext(ctx, `
        INSERT INTO users (subject, display_name, role, local_hash) VALUES (NULL, ?, 'admin', ?)
    `, username, passwordHash)
    if err != nil {
        return User{}, err
    }
    id, err := res.LastInsertId()
    if err != nil {
        return User{}, err
    }

    row := s.db.QueryRowContext(ctx, userSelect+` WHERE id = ?`, id)
    return scanUser(row)
}

func (s *Store) GetLocalAdminByUsername(ctx context.Context, username string) (*User, error) {
    row := s.db.QueryRowContext(ctx,
        userSelect+` WHERE subject IS NULL AND display_name = ? AND local_hash IS NOT NULL`, username)
    u, err := scanUser(row)
    if errors.Is(err, sql.ErrNoRows) {
        return nil, nil
    }
    if err != nil {
        return nil, err
    }
    return &u, nil
}

func (s *Store) ListRoleMappings(ctx context.Context) ([]RoleMapping, error) {
    rows, err := s.db.QueryContext(ctx,
        `SELECT id, group_claim, role, priority FROM role_mappings ORDER BY priority ASC`)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var out []RoleMapping
    for rows.Next() {
        var m RoleMapping
        if err := rows.Scan(&m.ID, &m.GroupClaim, &m.Role, &m.Priority); err != nil {
            return nil, err
        }
        out = append(out, m)
    }
    return out, rows.Err()
}

func (s *Store) UpsertRoleMapping(ctx context.Context, m RoleMapping) (RoleMapping, error) {
    _, err := s.db.ExecContext(ctx, `
        INSERT INTO role_mappings (group_claim, role, priority) VALUES (?, ?, ?)
        ON CONFLICT(group_claim) DO UPDATE SET role = excluded.role, priority = excluded.priority
    `, m.GroupClaim, m.Role, m.Priority)
    if err != nil {
        return RoleMapping{}, err
    }

    var out RoleMapping
    err = s.db.QueryRowContext(ctx,
        `SELECT id, group_claim, role, priority FROM role_mappings WHERE group_claim = ?`, m.GroupClaim).
        Scan(&out.ID, &out.GroupClaim, &out.Role, &out.Priority)
    return out, err
}

func (s *Store) ResolveRole(ctx context.Context, groups []string) (string, bool) {
    mappings, err := s.ListRoleMappings(ctx)
    if err != nil {
        return "", false
    }

    memberOf := make(map[string]bool, len(groups))
    for _, g := range groups {
        memberOf[g] = true
    }

    for _, m := range mappings { // already ordered by priority ASC == "lowest wins"
        if memberOf[m.GroupClaim] {
            return m.Role, true
        }
    }
    return "", false
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd siem-api && go test ./internal/store/... -run "User|LocalAdmin|ResolveRole" -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add siem-api/internal/store/users.go siem-api/internal/store/users_test.go
git commit -m "Add siem-api store: users, role mappings, local admin"
```

---

### Task 9: `store` — seen values (first-seen tracking)

**Files:**
- Create: `siem-api/internal/store/seen_values.go`
- Test: `siem-api/internal/store/seen_values_test.go`

**Interfaces:**
- Consumes: `Store` (Task 3), `Rule` (Task 6, for the `rule_id` foreign key in tests), `formatTime` (Task 5).
- Produces: methods below, consumed by Task 19 (`FirstSeenEvaluator`).

```go
func (s *Store) HasSeenValue(ctx context.Context, ruleID int64, value string) (bool, error)
func (s *Store) MarkSeenValue(ctx context.Context, ruleID int64, value string, at time.Time) error
```

- [ ] **Step 1: Write the failing test**

`siem-api/internal/store/seen_values_test.go`:
```go
package store

import (
    "context"
    "testing"
    "time"
)

func TestHasSeenValue_UnseenThenSeen(t *testing.T) {
    s := newTestStore(t)
    ctx := context.Background()
    rule := createTestRule(t, s)

    seen, err := s.HasSeenValue(ctx, rule.ID, "new-domain.example")
    if err != nil {
        t.Fatalf("HasSeenValue() error = %v", err)
    }
    if seen {
        t.Error("HasSeenValue() = true, want false before MarkSeenValue")
    }

    if err := s.MarkSeenValue(ctx, rule.ID, "new-domain.example", time.Now().UTC()); err != nil {
        t.Fatalf("MarkSeenValue() error = %v", err)
    }

    seen, err = s.HasSeenValue(ctx, rule.ID, "new-domain.example")
    if err != nil {
        t.Fatalf("second HasSeenValue() error = %v", err)
    }
    if !seen {
        t.Error("HasSeenValue() = false, want true after MarkSeenValue")
    }
}

func TestMarkSeenValue_Idempotent(t *testing.T) {
    s := newTestStore(t)
    ctx := context.Background()
    rule := createTestRule(t, s)

    if err := s.MarkSeenValue(ctx, rule.ID, "v", time.Now().UTC()); err != nil {
        t.Fatalf("MarkSeenValue() error = %v", err)
    }
    if err := s.MarkSeenValue(ctx, rule.ID, "v", time.Now().UTC()); err != nil {
        t.Fatalf("second MarkSeenValue() error = %v, want nil (must be idempotent)", err)
    }
}

func TestHasSeenValue_ScopedPerRule(t *testing.T) {
    s := newTestStore(t)
    ctx := context.Background()
    rule1 := createTestRule(t, s)
    rule2, err := s.CreateRule(ctx, Rule{Name: "other-rule", Shape: "first_seen", Severity: "low",
        Destinations: []string{"inapp"}, CooldownSec: 60, IntervalSec: 60, Enabled: true}, nil)
    if err != nil {
        t.Fatalf("CreateRule() error = %v", err)
    }

    if err := s.MarkSeenValue(ctx, rule1.ID, "shared-value", time.Now().UTC()); err != nil {
        t.Fatalf("MarkSeenValue() error = %v", err)
    }

    seen, err := s.HasSeenValue(ctx, rule2.ID, "shared-value")
    if err != nil {
        t.Fatalf("HasSeenValue() error = %v", err)
    }
    if seen {
        t.Error("HasSeenValue() = true for rule2, want false — seen_values must be scoped per rule")
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd siem-api && go test ./internal/store/... -run SeenValue -v`
Expected: FAIL — `HasSeenValue`/`MarkSeenValue` undefined.

- [ ] **Step 3: Write the implementation**

`siem-api/internal/store/seen_values.go`:
```go
package store

import (
    "context"
    "database/sql"
    "errors"
    "time"
)

func (s *Store) HasSeenValue(ctx context.Context, ruleID int64, value string) (bool, error) {
    var id int64
    err := s.db.QueryRowContext(ctx,
        `SELECT id FROM seen_values WHERE rule_id = ? AND value = ?`, ruleID, value).Scan(&id)
    if errors.Is(err, sql.ErrNoRows) {
        return false, nil
    }
    if err != nil {
        return false, err
    }
    return true, nil
}

func (s *Store) MarkSeenValue(ctx context.Context, ruleID int64, value string, at time.Time) error {
    _, err := s.db.ExecContext(ctx, `
        INSERT INTO seen_values (rule_id, value, first_seen_at) VALUES (?, ?, ?)
        ON CONFLICT(rule_id, value) DO NOTHING
    `, ruleID, value, formatTime(at))
    return err
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd siem-api && go test ./internal/store/... -run SeenValue -v`
Expected: PASS.

- [ ] **Step 5: Run the full store package test suite**

Run: `cd siem-api && go test ./internal/store/... -v`
Expected: PASS — every test from Tasks 3-9 passes together (no cross-test interference; `newTestStore(t)` gives each test its own temp-dir DB).

- [ ] **Step 6: Commit**

```bash
git add siem-api/internal/store/seen_values.go siem-api/internal/store/seen_values_test.go
git commit -m "Add siem-api store: seen values for first-seen rule tracking"
```

---

### Task 10: `internal/auth` — internal JWT verification

**Files:**
- Create: `siem-api/internal/auth/token.go`
- Test: `siem-api/internal/auth/token_test.go`

**Interfaces:**
- Consumes: nothing from earlier tasks (takes a raw `[]byte` secret — `config.Config.SessionSecret` from Task 2 at wiring time in Task 29).
- Produces: `Claims` struct and `TokenVerifier`, consumed by Task 11 (`auth.Middleware`).

```go
type Claims struct {
    UserID      int64
    Subject     string
    Email       string
    DisplayName string
    Groups      []string
}
type TokenVerifier struct{ secret []byte }
func NewTokenVerifier(secret []byte) *TokenVerifier
func (v *TokenVerifier) Verify(tokenString string) (Claims, error)
```

This verifies the internal session token `siem-web`'s BFF mints after OIDC login (or after a
successful `/auth/local` check) — `siem-api` never talks OIDC/JWKS itself (see the design
spec's Auth & RBAC section). The token is a standard HS256 JWT signed with the shared
`SIEM_SESSION_SECRET`; `siem-api` only verifies the signature and expiry and reads its claims.

`UserID` is resolved once, at login time, by whichever endpoint the BFF calls to establish
the session (`POST /auth/session` for OIDC, `POST /auth/local` for break-glass — both added
in Task 12) and embedded in the token by the BFF at mint time. This means `auth.Middleware`
(Task 11) never needs a DB round-trip to map `Subject` → local user ID on every request — it
trusts the `user_id` claim the same way it trusts `sub`, since both come from the same
signed token. `Groups`, by contrast, is deliberately re-resolved against `role_mappings` on
every request (not cached in the token) — see Task 11 — so a role-mapping edit takes effect
for active sessions immediately, without forcing re-login.

- [ ] **Step 1: Add the JWT dependency**

Run: `cd siem-api && go get github.com/golang-jwt/jwt/v5`

- [ ] **Step 2: Write the failing test**

`siem-api/internal/auth/token_test.go`:
```go
package auth

import (
    "testing"
    "time"

    "github.com/golang-jwt/jwt/v5"
)

func mintTestToken(t *testing.T, secret []byte, claims jwtClaims) string {
    t.Helper()
    token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
    signed, err := token.SignedString(secret)
    if err != nil {
        t.Fatalf("SignedString() error = %v", err)
    }
    return signed
}

func TestVerify_ValidToken(t *testing.T) {
    secret := []byte("0123456789abcdef0123456789abcdef")
    v := NewTokenVerifier(secret)

    claims := jwtClaims{
        RegisteredClaims: jwt.RegisteredClaims{
            Subject:   "oidc-sub-1",
            ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
        },
        UserID:      42,
        Email:       "alice@townsville.cc",
        DisplayName: "Alice",
        Groups:      []string{"siem-analysts"},
    }
    signed := mintTestToken(t, secret, claims)

    got, err := v.Verify(signed)
    if err != nil {
        t.Fatalf("Verify() error = %v", err)
    }
    if got.Subject != "oidc-sub-1" || got.Email != "alice@townsville.cc" {
        t.Errorf("Verify() = %+v", got)
    }
    if got.UserID != 42 {
        t.Errorf("UserID = %d, want 42", got.UserID)
    }
    if len(got.Groups) != 1 || got.Groups[0] != "siem-analysts" {
        t.Errorf("Groups = %v, want [siem-analysts]", got.Groups)
    }
}

func TestVerify_WrongSecret(t *testing.T) {
    signingSecret := []byte("0123456789abcdef0123456789abcdef")
    verifySecret := []byte("ffffffffffffffffffffffffffffffff")

    claims := jwtClaims{RegisteredClaims: jwt.RegisteredClaims{
        Subject: "oidc-sub-1", ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
    }}
    signed := mintTestToken(t, signingSecret, claims)

    v := NewTokenVerifier(verifySecret)
    if _, err := v.Verify(signed); err == nil {
        t.Fatal("Verify() error = nil, want error for wrong secret")
    }
}

func TestVerify_Expired(t *testing.T) {
    secret := []byte("0123456789abcdef0123456789abcdef")
    claims := jwtClaims{RegisteredClaims: jwt.RegisteredClaims{
        Subject: "oidc-sub-1", ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Hour)),
    }}
    signed := mintTestToken(t, secret, claims)

    v := NewTokenVerifier(secret)
    if _, err := v.Verify(signed); err == nil {
        t.Fatal("Verify() error = nil, want error for expired token")
    }
}

func TestVerify_Malformed(t *testing.T) {
    v := NewTokenVerifier([]byte("0123456789abcdef0123456789abcdef"))
    if _, err := v.Verify("not-a-jwt"); err == nil {
        t.Fatal("Verify() error = nil, want error for malformed token")
    }
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `cd siem-api && go test ./internal/auth/... -v`
Expected: FAIL — `NewTokenVerifier`/`jwtClaims`/etc. undefined.

- [ ] **Step 4: Write the implementation**

`siem-api/internal/auth/token.go`:
```go
package auth

import (
    "fmt"

    "github.com/golang-jwt/jwt/v5"
)

type Claims struct {
    UserID      int64
    Subject     string
    Email       string
    DisplayName string
    Groups      []string
}

// jwtClaims is the wire format minted by siem-web's BFF; TokenVerifier only
// ever decodes it, never encodes it (minting lives in the BFF, out of scope
// for siem-api).
type jwtClaims struct {
    jwt.RegisteredClaims
    UserID      int64    `json:"user_id"`
    Email       string   `json:"email"`
    DisplayName string   `json:"display_name"`
    Groups      []string `json:"groups"`
}

type TokenVerifier struct {
    secret []byte
}

func NewTokenVerifier(secret []byte) *TokenVerifier {
    return &TokenVerifier{secret: secret}
}

func (v *TokenVerifier) Verify(tokenString string) (Claims, error) {
    var claims jwtClaims
    token, err := jwt.ParseWithClaims(tokenString, &claims, func(t *jwt.Token) (any, error) {
        if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
            return nil, fmt.Errorf("auth: unexpected signing method %v", t.Header["alg"])
        }
        return v.secret, nil
    })
    if err != nil {
        return Claims{}, fmt.Errorf("auth: verify token: %w", err)
    }
    if !token.Valid {
        return Claims{}, fmt.Errorf("auth: token invalid")
    }

    return Claims{
        UserID:      claims.UserID,
        Subject:     claims.Subject,
        Email:       claims.Email,
        DisplayName: claims.DisplayName,
        Groups:      claims.Groups,
    }, nil
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `cd siem-api && go test ./internal/auth/... -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add siem-api/go.mod siem-api/go.sum siem-api/internal/auth/token.go siem-api/internal/auth/token_test.go
git commit -m "Add siem-api auth: internal JWT verification"
```

---

### Task 11: `internal/auth` — RBAC middleware

**Files:**
- Create: `siem-api/internal/auth/middleware.go`
- Test: `siem-api/internal/auth/middleware_test.go`

**Interfaces:**
- Consumes: `TokenVerifier`/`Claims` (Task 10), `store.Store.ResolveRole` (Task 8, via the `RoleResolver` interface below — `*store.Store` satisfies it, but the interface keeps this package testable without a real DB).
- Produces: `Middleware`, `RequireRole`, `UserFromContext`, consumed by Task 22 (`api` middleware chain) and every protected handler task (23-28).

```go
type RoleResolver interface {
    ResolveRole(ctx context.Context, groups []string) (role string, ok bool)
}
func Middleware(verifier *TokenVerifier, resolver RoleResolver) func(http.Handler) http.Handler
func RequireRole(minRole string, next http.Handler) http.Handler
func UserFromContext(ctx context.Context) (userID int64, role string, ok bool)
```

`Middleware` reads `Authorization: Bearer <token>`, verifies it, resolves role from the
token's `Groups` via `resolver.ResolveRole` (fresh every request — see Task 10's note on
why this isn't cached in the token), and attaches `(userID, role)` to the request context.
Missing/invalid token → `401`. Valid token but no matching role mapping → `403` (deny,
per the handoff's "first match wins; users in no listed group are denied").

`RequireRole` wraps a handler and enforces a minimum role via a fixed hierarchy
`viewer < analyst < admin`; it reads the role `Middleware` already attached to context, so
it must sit inside `Middleware` in the chain, never outside it.

- [ ] **Step 1: Write the failing test**

`siem-api/internal/auth/middleware_test.go`:
```go
package auth

import (
    "context"
    "net/http"
    "net/http/httptest"
    "testing"
    "time"

    "github.com/golang-jwt/jwt/v5"
)

type fakeResolver struct {
    roles map[string]string // group -> role
}

func (f *fakeResolver) ResolveRole(ctx context.Context, groups []string) (string, bool) {
    for _, g := range groups {
        if role, ok := f.roles[g]; ok {
            return role, true
        }
    }
    return "", false
}

func echoHandler() http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        userID, role, ok := UserFromContext(r.Context())
        if !ok {
            http.Error(w, "no user in context", http.StatusInternalServerError)
            return
        }
        w.Header().Set("X-User-ID", itoa(userID))
        w.Header().Set("X-Role", role)
        w.WriteHeader(http.StatusOK)
    })
}

func itoa(n int64) string {
    if n == 0 {
        return "0"
    }
    neg := n < 0
    if neg {
        n = -n
    }
    var buf [20]byte
    i := len(buf)
    for n > 0 {
        i--
        buf[i] = byte('0' + n%10)
        n /= 10
    }
    if neg {
        i--
        buf[i] = '-'
    }
    return string(buf[i:])
}

func signedTestToken(t *testing.T, secret []byte, userID int64, groups []string) string {
    t.Helper()
    claims := jwtClaims{
        RegisteredClaims: jwt.RegisteredClaims{
            Subject:   "sub-1",
            ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
        },
        UserID: userID,
        Groups: groups,
    }
    token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(secret)
    if err != nil {
        t.Fatalf("SignedString() error = %v", err)
    }
    return token
}

func TestMiddleware_ValidTokenAttachesUserAndRole(t *testing.T) {
    secret := []byte("0123456789abcdef0123456789abcdef")
    resolver := &fakeResolver{roles: map[string]string{"siem-analysts": "analyst"}}
    mw := Middleware(NewTokenVerifier(secret), resolver)
    handler := mw(echoHandler())

    req := httptest.NewRequest(http.MethodGet, "/alerts", nil)
    req.Header.Set("Authorization", "Bearer "+signedTestToken(t, secret, 7, []string{"siem-analysts"}))
    rec := httptest.NewRecorder()
    handler.ServeHTTP(rec, req)

    if rec.Code != http.StatusOK {
        t.Fatalf("status = %d, want 200", rec.Code)
    }
    if rec.Header().Get("X-User-ID") != "7" {
        t.Errorf("X-User-ID = %q, want 7", rec.Header().Get("X-User-ID"))
    }
    if rec.Header().Get("X-Role") != "analyst" {
        t.Errorf("X-Role = %q, want analyst", rec.Header().Get("X-Role"))
    }
}

func TestMiddleware_MissingToken(t *testing.T) {
    secret := []byte("0123456789abcdef0123456789abcdef")
    mw := Middleware(NewTokenVerifier(secret), &fakeResolver{})
    handler := mw(echoHandler())

    req := httptest.NewRequest(http.MethodGet, "/alerts", nil)
    rec := httptest.NewRecorder()
    handler.ServeHTTP(rec, req)

    if rec.Code != http.StatusUnauthorized {
        t.Fatalf("status = %d, want 401", rec.Code)
    }
}

func TestMiddleware_UnmappedGroupDenied(t *testing.T) {
    secret := []byte("0123456789abcdef0123456789abcdef")
    resolver := &fakeResolver{roles: map[string]string{"admins": "admin"}}
    mw := Middleware(NewTokenVerifier(secret), resolver)
    handler := mw(echoHandler())

    req := httptest.NewRequest(http.MethodGet, "/alerts", nil)
    req.Header.Set("Authorization", "Bearer "+signedTestToken(t, secret, 7, []string{"some-other-group"}))
    rec := httptest.NewRecorder()
    handler.ServeHTTP(rec, req)

    if rec.Code != http.StatusForbidden {
        t.Fatalf("status = %d, want 403", rec.Code)
    }
}

func TestRequireRole_InsufficientDenied(t *testing.T) {
    secret := []byte("0123456789abcdef0123456789abcdef")
    resolver := &fakeResolver{roles: map[string]string{"homelab": "viewer"}}
    mw := Middleware(NewTokenVerifier(secret), resolver)
    handler := mw(RequireRole("admin", echoHandler()))

    req := httptest.NewRequest(http.MethodPost, "/rules", nil)
    req.Header.Set("Authorization", "Bearer "+signedTestToken(t, secret, 7, []string{"homelab"}))
    rec := httptest.NewRecorder()
    handler.ServeHTTP(rec, req)

    if rec.Code != http.StatusForbidden {
        t.Fatalf("status = %d, want 403", rec.Code)
    }
}

func TestRequireRole_SufficientAllowed(t *testing.T) {
    secret := []byte("0123456789abcdef0123456789abcdef")
    resolver := &fakeResolver{roles: map[string]string{"admins": "admin"}}
    mw := Middleware(NewTokenVerifier(secret), resolver)
    handler := mw(RequireRole("analyst", echoHandler()))

    req := httptest.NewRequest(http.MethodPost, "/rules", nil)
    req.Header.Set("Authorization", "Bearer "+signedTestToken(t, secret, 7, []string{"admins"}))
    rec := httptest.NewRecorder()
    handler.ServeHTTP(rec, req)

    if rec.Code != http.StatusOK {
        t.Fatalf("status = %d, want 200", rec.Code)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd siem-api && go test ./internal/auth/... -run "Middleware|RequireRole" -v`
Expected: FAIL — `Middleware`/`RequireRole`/`UserFromContext` undefined.

- [ ] **Step 3: Write the implementation**

`siem-api/internal/auth/middleware.go`:
```go
package auth

import (
    "context"
    "net/http"
    "strings"
)

type RoleResolver interface {
    ResolveRole(ctx context.Context, groups []string) (role string, ok bool)
}

type ctxKey int

const (
    ctxUserIDKey ctxKey = iota
    ctxRoleKey
)

var roleRank = map[string]int{"viewer": 1, "analyst": 2, "admin": 3}

func Middleware(verifier *TokenVerifier, resolver RoleResolver) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            header := r.Header.Get("Authorization")
            const prefix = "Bearer "
            if !strings.HasPrefix(header, prefix) {
                http.Error(w, "missing bearer token", http.StatusUnauthorized)
                return
            }

            claims, err := verifier.Verify(strings.TrimPrefix(header, prefix))
            if err != nil {
                http.Error(w, "invalid token", http.StatusUnauthorized)
                return
            }

            role, ok := resolver.ResolveRole(r.Context(), claims.Groups)
            if !ok {
                http.Error(w, "no role mapping for this identity", http.StatusForbidden)
                return
            }

            ctx := context.WithValue(r.Context(), ctxUserIDKey, claims.UserID)
            ctx = context.WithValue(ctx, ctxRoleKey, role)
            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}

func RequireRole(minRole string, next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        _, role, ok := UserFromContext(r.Context())
        if !ok || roleRank[role] < roleRank[minRole] {
            http.Error(w, "insufficient role", http.StatusForbidden)
            return
        }
        next.ServeHTTP(w, r)
    })
}

func UserFromContext(ctx context.Context) (userID int64, role string, ok bool) {
    uid, uidOK := ctx.Value(ctxUserIDKey).(int64)
    r, roleOK := ctx.Value(ctxRoleKey).(string)
    if !uidOK || !roleOK {
        return 0, "", false
    }
    return uid, r, true
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd siem-api && go test ./internal/auth/... -v`
Expected: PASS (all tests from Tasks 10 and 11).

- [ ] **Step 5: Commit**

```bash
git add siem-api/internal/auth/middleware.go siem-api/internal/auth/middleware_test.go
git commit -m "Add siem-api auth: RBAC middleware"
```

---

### Task 12: `internal/auth` — session establishment (OIDC handoff + local login)

**Files:**
- Create: `siem-api/internal/auth/session.go`
- Test: `siem-api/internal/auth/session_test.go`

**Interfaces:**
- Consumes: `RoleResolver` (Task 11), `store.User` (Task 8).
- Produces: `SessionEstablisher` and `LocalAuthenticator`, consumed by Task 28 (`POST /auth/session` and `POST /auth/local` handlers).

```go
type SessionStore interface {
    UpsertUserBySubject(ctx context.Context, subject, email, displayName, role string) (store.User, error)
    TouchUserLogin(ctx context.Context, id int64, at time.Time) error
}
type SessionEstablisher struct{ store SessionStore; resolver RoleResolver }
func NewSessionEstablisher(s SessionStore, r RoleResolver) *SessionEstablisher
func (e *SessionEstablisher) Establish(ctx context.Context, subject, email, displayName string, groups []string) (store.User, error)

type LocalUserStore interface {
    GetLocalAdminByUsername(ctx context.Context, username string) (*store.User, error)
    TouchUserLogin(ctx context.Context, id int64, at time.Time) error
}
type LocalAuthenticator struct{ store LocalUserStore }
func NewLocalAuthenticator(s LocalUserStore) *LocalAuthenticator
func (a *LocalAuthenticator) Login(ctx context.Context, username, password string) (store.User, error)
```

Neither `siem-api` endpoint that will use these (Task 28) does its own OIDC/JWKS work —
`SessionEstablisher.Establish` is called by the BFF *after* it has already verified the ID
token against Pocket ID's JWKS; the `subject`/`email`/`displayName`/`groups` arguments here
are that token's already-verified claims, forwarded over the trusted `backend` network.
`Establish` denies (returns an error) if `resolver.ResolveRole` finds no mapping — the same
"first match wins, deny if unmapped" rule `auth.Middleware` (Task 11) applies per-request
also applies at login, so an unmapped identity is rejected before a session ever exists,
not just on its first subsequent API call.

- [ ] **Step 1: Add the bcrypt dependency**

Run: `cd siem-api && go get golang.org/x/crypto/bcrypt`

- [ ] **Step 2: Write the failing test**

`siem-api/internal/auth/session_test.go`:
```go
package auth

import (
    "context"
    "testing"
    "time"

    "github.com/hibikipr/homeSIEM/siem-api/internal/store"
    "golang.org/x/crypto/bcrypt"
)

type fakeSessionStore struct {
    users       map[string]store.User // keyed by subject
    nextID      int64
    loginTouches []int64
}

func (f *fakeSessionStore) UpsertUserBySubject(ctx context.Context, subject, email, displayName, role string) (store.User, error) {
    if u, ok := f.users[subject]; ok {
        u.Email, u.DisplayName, u.Role = &email, &displayName, role
        f.users[subject] = u
        return u, nil
    }
    f.nextID++
    u := store.User{ID: f.nextID, Subject: &subject, Email: &email, DisplayName: &displayName, Role: role}
    f.users[subject] = u
    return u, nil
}

func (f *fakeSessionStore) TouchUserLogin(ctx context.Context, id int64, at time.Time) error {
    f.loginTouches = append(f.loginTouches, id)
    return nil
}

func TestSessionEstablisher_Success(t *testing.T) {
    store := &fakeSessionStore{users: map[string]store.User{}}
    resolver := &fakeResolver{roles: map[string]string{"siem-analysts": "analyst"}}
    e := NewSessionEstablisher(store, resolver)

    u, err := e.Establish(context.Background(), "sub-1", "alice@townsville.cc", "Alice", []string{"siem-analysts"})
    if err != nil {
        t.Fatalf("Establish() error = %v", err)
    }
    if u.Role != "analyst" {
        t.Errorf("Role = %q, want analyst", u.Role)
    }
    if len(store.loginTouches) != 1 || store.loginTouches[0] != u.ID {
        t.Errorf("loginTouches = %v, want [%d]", store.loginTouches, u.ID)
    }
}

func TestSessionEstablisher_UnmappedGroupDenied(t *testing.T) {
    store := &fakeSessionStore{users: map[string]store.User{}}
    resolver := &fakeResolver{roles: map[string]string{}}
    e := NewSessionEstablisher(store, resolver)

    if _, err := e.Establish(context.Background(), "sub-1", "a@b.c", "A", []string{"no-mapping"}); err == nil {
        t.Fatal("Establish() error = nil, want error for unmapped groups")
    }
    if len(store.loginTouches) != 0 {
        t.Error("TouchUserLogin called for a denied session, want not called")
    }
}

type fakeLocalStore struct {
    user         *store.User
    loginTouches []int64
}

func (f *fakeLocalStore) GetLocalAdminByUsername(ctx context.Context, username string) (*store.User, error) {
    if f.user == nil || f.user.DisplayName == nil || *f.user.DisplayName != username {
        return nil, nil
    }
    return f.user, nil
}

func (f *fakeLocalStore) TouchUserLogin(ctx context.Context, id int64, at time.Time) error {
    f.loginTouches = append(f.loginTouches, id)
    return nil
}

func TestLocalAuthenticator_Success(t *testing.T) {
    hash, err := bcrypt.GenerateFromPassword([]byte("correct-horse"), bcrypt.DefaultCost)
    if err != nil {
        t.Fatalf("GenerateFromPassword() error = %v", err)
    }
    hashStr := string(hash)
    username := "admin"
    fs := &fakeLocalStore{user: &store.User{ID: 1, DisplayName: &username, LocalHash: &hashStr, Role: "admin"}}
    a := NewLocalAuthenticator(fs)

    u, err := a.Login(context.Background(), "admin", "correct-horse")
    if err != nil {
        t.Fatalf("Login() error = %v", err)
    }
    if u.ID != 1 {
        t.Errorf("ID = %d, want 1", u.ID)
    }
    if len(fs.loginTouches) != 1 {
        t.Errorf("loginTouches = %v, want one entry", fs.loginTouches)
    }
}

func TestLocalAuthenticator_WrongPassword(t *testing.T) {
    hash, err := bcrypt.GenerateFromPassword([]byte("correct-horse"), bcrypt.DefaultCost)
    if err != nil {
        t.Fatalf("GenerateFromPassword() error = %v", err)
    }
    hashStr := string(hash)
    username := "admin"
    fs := &fakeLocalStore{user: &store.User{ID: 1, DisplayName: &username, LocalHash: &hashStr, Role: "admin"}}
    a := NewLocalAuthenticator(fs)

    if _, err := a.Login(context.Background(), "admin", "wrong-password"); err == nil {
        t.Fatal("Login() error = nil, want error for wrong password")
    }
    if len(fs.loginTouches) != 0 {
        t.Error("TouchUserLogin called on failed login, want not called")
    }
}

func TestLocalAuthenticator_UnknownUsername(t *testing.T) {
    fs := &fakeLocalStore{user: nil}
    a := NewLocalAuthenticator(fs)

    if _, err := a.Login(context.Background(), "ghost", "anything"); err == nil {
        t.Fatal("Login() error = nil, want error for unknown username")
    }
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `cd siem-api && go test ./internal/auth/... -run "SessionEstablisher|LocalAuthenticator" -v`
Expected: FAIL — types undefined.

- [ ] **Step 4: Write the implementation**

`siem-api/internal/auth/session.go`:
```go
package auth

import (
    "context"
    "fmt"
    "time"

    "github.com/hibikipr/homeSIEM/siem-api/internal/store"
    "golang.org/x/crypto/bcrypt"
)

type SessionStore interface {
    UpsertUserBySubject(ctx context.Context, subject, email, displayName, role string) (store.User, error)
    TouchUserLogin(ctx context.Context, id int64, at time.Time) error
}

type SessionEstablisher struct {
    store    SessionStore
    resolver RoleResolver
}

func NewSessionEstablisher(s SessionStore, r RoleResolver) *SessionEstablisher {
    return &SessionEstablisher{store: s, resolver: r}
}

func (e *SessionEstablisher) Establish(ctx context.Context, subject, email, displayName string, groups []string) (store.User, error) {
    role, ok := e.resolver.ResolveRole(ctx, groups)
    if !ok {
        return store.User{}, fmt.Errorf("auth: no role mapping for groups %v", groups)
    }

    u, err := e.store.UpsertUserBySubject(ctx, subject, email, displayName, role)
    if err != nil {
        return store.User{}, err
    }

    if err := e.store.TouchUserLogin(ctx, u.ID, time.Now()); err != nil {
        return store.User{}, err
    }
    return u, nil
}

type LocalUserStore interface {
    GetLocalAdminByUsername(ctx context.Context, username string) (*store.User, error)
    TouchUserLogin(ctx context.Context, id int64, at time.Time) error
}

type LocalAuthenticator struct {
    store LocalUserStore
}

func NewLocalAuthenticator(s LocalUserStore) *LocalAuthenticator {
    return &LocalAuthenticator{store: s}
}

func (a *LocalAuthenticator) Login(ctx context.Context, username, password string) (store.User, error) {
    u, err := a.store.GetLocalAdminByUsername(ctx, username)
    if err != nil {
        return store.User{}, err
    }
    if u == nil || u.LocalHash == nil {
        return store.User{}, fmt.Errorf("auth: invalid credentials")
    }

    if err := bcrypt.CompareHashAndPassword([]byte(*u.LocalHash), []byte(password)); err != nil {
        return store.User{}, fmt.Errorf("auth: invalid credentials")
    }

    if err := a.store.TouchUserLogin(ctx, u.ID, time.Now()); err != nil {
        return store.User{}, err
    }
    return *u, nil
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `cd siem-api && go test ./internal/auth/... -v`
Expected: PASS (all `internal/auth` tests from Tasks 10-12).

- [ ] **Step 6: Commit**

```bash
git add siem-api/go.mod siem-api/go.sum siem-api/internal/auth/session.go siem-api/internal/auth/session_test.go
git commit -m "Add siem-api auth: session establishment for OIDC handoff and local login"
```

---

### Task 13: `internal/loki` — QueryRange client

**Files:**
- Create: `siem-api/internal/loki/client.go`
- Test: `siem-api/internal/loki/client_test.go`

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces: `Client`, `LogEntry`, `QueryResult`, `Client.QueryRange`, consumed by Task 18 (`ThresholdEvaluator`)/Task 19 (`FirstSeenEvaluator`) via the `rules.LokiQuerier` interface, and Task 24 (`/events/search`, `/events/tail`).

```go
type LogEntry struct {
    Timestamp time.Time
    Labels    map[string]string
    Line      string
}
type QueryResult struct {
    Entries []LogEntry
}
type Client struct { baseURL string; httpClient *http.Client }
func New(baseURL string, httpClient *http.Client) *Client
func (c *Client) QueryRange(ctx context.Context, logql string, start, end time.Time, limit int) (QueryResult, error)
```

Calls Loki's `GET /loki/api/v1/query_range`, entries sorted oldest-first regardless of the
order Loki returns streams in (Loki groups by stream, not globally by time — callers of
`QueryRange`, especially the tail poller in Task 24, need a single chronological sequence).

- [ ] **Step 1: Write the failing test**

`siem-api/internal/loki/client_test.go`:
```go
package loki

import (
    "net/http"
    "net/http/httptest"
    "net/url"
    "testing"
    "time"
)

func TestQueryRange_ParsesAndSortsEntries(t *testing.T) {
    var gotQuery url.Values
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        gotQuery = r.URL.Query()
        w.Header().Set("Content-Type", "application/json")
        w.Write([]byte(`{
            "status": "success",
            "data": {
                "resultType": "streams",
                "result": [
                    {
                        "stream": {"job": "siem", "source": "udm-ultra"},
                        "values": [["1700000002000000000", "second line"], ["1700000000000000000", "first line"]]
                    },
                    {
                        "stream": {"job": "siem", "source": "host-1"},
                        "values": [["1700000001000000000", "middle line"]]
                    }
                ]
            }
        }`))
    }))
    defer srv.Close()

    c := New(srv.URL, srv.Client())
    start := time.Unix(1700000000, 0)
    end := time.Unix(1700000010, 0)
    result, err := c.QueryRange(context.Background(), `{job="siem"}`, start, end, 100)
    if err != nil {
        t.Fatalf("QueryRange() error = %v", err)
    }

    if gotQuery.Get("query") != `{job="siem"}` {
        t.Errorf("query param = %q", gotQuery.Get("query"))
    }
    if gotQuery.Get("limit") != "100" {
        t.Errorf("limit param = %q, want 100", gotQuery.Get("limit"))
    }

    if len(result.Entries) != 3 {
        t.Fatalf("len(Entries) = %d, want 3", len(result.Entries))
    }
    if result.Entries[0].Line != "first line" {
        t.Errorf("Entries[0].Line = %q, want first line (entries must be time-sorted ascending)", result.Entries[0].Line)
    }
    if result.Entries[1].Line != "middle line" {
        t.Errorf("Entries[1].Line = %q, want middle line", result.Entries[1].Line)
    }
    if result.Entries[2].Line != "second line" {
        t.Errorf("Entries[2].Line = %q, want second line", result.Entries[2].Line)
    }
    if result.Entries[0].Labels["source"] != "udm-ultra" {
        t.Errorf("Labels[source] = %q, want udm-ultra", result.Entries[0].Labels["source"])
    }
}

func TestQueryRange_NonSuccessStatus(t *testing.T) {
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusInternalServerError)
        w.Write([]byte(`{"status":"error","error":"boom"}`))
    }))
    defer srv.Close()

    c := New(srv.URL, srv.Client())
    if _, err := c.QueryRange(context.Background(), `{job="siem"}`, time.Now(), time.Now(), 10); err == nil {
        t.Fatal("QueryRange() error = nil, want error for 500 response")
    }
}
```

Add `"context"` to the import block at the top of `client_test.go`:
```go
import (
    "context"
    "net/http"
    "net/http/httptest"
    "net/url"
    "testing"
    "time"
)
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd siem-api && go test ./internal/loki/... -v`
Expected: FAIL — `New`/`QueryRange` undefined.

- [ ] **Step 3: Write the implementation**

`siem-api/internal/loki/client.go`:
```go
package loki

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "net/url"
    "sort"
    "strconv"
    "time"
)

type LogEntry struct {
    Timestamp time.Time
    Labels    map[string]string
    Line      string
}

type QueryResult struct {
    Entries []LogEntry
}

type Client struct {
    baseURL    string
    httpClient *http.Client
}

func New(baseURL string, httpClient *http.Client) *Client {
    if httpClient == nil {
        httpClient = http.DefaultClient
    }
    return &Client{baseURL: baseURL, httpClient: httpClient}
}

type queryRangeResponse struct {
    Status string `json:"status"`
    Data   struct {
        Result []struct {
            Stream map[string]string `json:"stream"`
            Values [][2]string        `json:"values"`
        } `json:"result"`
    } `json:"data"`
    Error string `json:"error"`
}

func (c *Client) QueryRange(ctx context.Context, logql string, start, end time.Time, limit int) (QueryResult, error) {
    q := url.Values{}
    q.Set("query", logql)
    q.Set("start", strconv.FormatInt(start.UnixNano(), 10))
    q.Set("end", strconv.FormatInt(end.UnixNano(), 10))
    q.Set("limit", strconv.Itoa(limit))

    req, err := http.NewRequestWithContext(ctx, http.MethodGet,
        c.baseURL+"/loki/api/v1/query_range?"+q.Encode(), nil)
    if err != nil {
        return QueryResult{}, err
    }

    resp, err := c.httpClient.Do(req)
    if err != nil {
        return QueryResult{}, fmt.Errorf("loki: query_range request: %w", err)
    }
    defer resp.Body.Close()

    var parsed queryRangeResponse
    if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
        return QueryResult{}, fmt.Errorf("loki: decode response: %w", err)
    }
    if resp.StatusCode != http.StatusOK || parsed.Status != "success" {
        return QueryResult{}, fmt.Errorf("loki: query_range failed: status=%d error=%q", resp.StatusCode, parsed.Error)
    }

    var entries []LogEntry
    for _, stream := range parsed.Data.Result {
        for _, v := range stream.Values {
            nanos, err := strconv.ParseInt(v[0], 10, 64)
            if err != nil {
                return QueryResult{}, fmt.Errorf("loki: parse timestamp %q: %w", v[0], err)
            }
            entries = append(entries, LogEntry{
                Timestamp: time.Unix(0, nanos).UTC(),
                Labels:    stream.Stream,
                Line:      v[1],
            })
        }
    }

    sort.Slice(entries, func(i, j int) bool {
        return entries[i].Timestamp.Before(entries[j].Timestamp)
    })

    return QueryResult{Entries: entries}, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd siem-api && go test ./internal/loki/... -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add siem-api/internal/loki/client.go siem-api/internal/loki/client_test.go
git commit -m "Add siem-api loki: QueryRange client"
```

---

### Task 14: `internal/loki` — BuildQuery (label discipline enforcement)

**Files:**
- Create: `siem-api/internal/loki/query.go`
- Test: `siem-api/internal/loki/query_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `Filters` struct and `BuildQuery(jobLabel string, f Filters) string`, consumed by Task 24 (`/events/search`, `/events/tail`) and by rule authors indirectly (rules store their own `logql` directly in Task 6's schema, so `BuildQuery` is specifically the search/tail query path, not the rule-evaluation path).

```go
type Filters struct {
    Source   string
    Host     string
    Program  string
    Severity string
    Facility string
    Extra    map[string]string // non-label fields, e.g. src_ip — always emitted as filters, never as labels
    FreeText string
}
func BuildQuery(jobLabel string, f Filters) string
```

This is the design spec's "one enforcement point" for label discipline: `Source`/`Host`/
`Program`/`Severity`/`Facility` are the only fields that can ever land inside `{...}` label
matchers — everything else (`Extra`, keyed by arbitrary field names like `src_ip`, `rule`,
`geoip.cc`) is always compiled to `| json | field="value"` filters after the label
selector, never as a label, no matter what key names a caller passes in.

- [ ] **Step 1: Write the failing test**

`siem-api/internal/loki/query_test.go`:
```go
package loki

import (
    "strings"
    "testing"
)

func TestBuildQuery_JobLabelOnly(t *testing.T) {
    got := BuildQuery("siem", Filters{})
    want := `{job="siem"}`
    if got != want {
        t.Errorf("BuildQuery() = %q, want %q", got, want)
    }
}

func TestBuildQuery_MandatedLabels(t *testing.T) {
    got := BuildQuery("siem", Filters{Source: "udm-ultra", Severity: "critical"})
    want := `{job="siem",source="udm-ultra",severity="critical"}`
    if got != want {
        t.Errorf("BuildQuery() = %q, want %q", got, want)
    }
}

func TestBuildQuery_ExtraFieldsNeverBecomeLabels(t *testing.T) {
    got := BuildQuery("siem", Filters{Source: "udm-ultra", Extra: map[string]string{"src_ip": "10.0.0.5"}})

    braceEnd := strings.Index(got, "}")
    if braceEnd < 0 {
        t.Fatalf("BuildQuery() = %q, no closing brace found", got)
    }
    labelPart := got[:braceEnd]
    if strings.Contains(labelPart, "src_ip") {
        t.Errorf("BuildQuery() label selector %q contains src_ip — Extra fields must never become labels", labelPart)
    }

    rest := got[braceEnd:]
    if !strings.Contains(rest, `| json`) || !strings.Contains(rest, `src_ip="10.0.0.5"`) {
        t.Errorf("BuildQuery() = %q, want a json filter for src_ip after the label selector", got)
    }
}

func TestBuildQuery_ExtraFieldsSortedDeterministically(t *testing.T) {
    got := BuildQuery("siem", Filters{Extra: map[string]string{"rule": "wan-portscan", "dst_port": "22"}})
    dstIdx := strings.Index(got, "dst_port")
    ruleIdx := strings.Index(got, `rule=`)
    if dstIdx < 0 || ruleIdx < 0 || dstIdx > ruleIdx {
        t.Errorf("BuildQuery() = %q, want dst_port before rule (alphabetical)", got)
    }
}

func TestBuildQuery_FreeText(t *testing.T) {
    got := BuildQuery("siem", Filters{FreeText: "timeout"})
    want := `{job="siem"} |= "timeout"`
    if got != want {
        t.Errorf("BuildQuery() = %q, want %q", got, want)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd siem-api && go test ./internal/loki/... -run BuildQuery -v`
Expected: FAIL — `BuildQuery`/`Filters` undefined.

- [ ] **Step 3: Write the implementation**

`siem-api/internal/loki/query.go`:
```go
package loki

import (
    "fmt"
    "sort"
    "strings"
)

type Filters struct {
    Source   string
    Host     string
    Program  string
    Severity string
    Facility string
    Extra    map[string]string
    FreeText string
}

func BuildQuery(jobLabel string, f Filters) string {
    labels := []string{fmt.Sprintf("job=%q", jobLabel)}
    for _, pair := range []struct{ name, value string }{
        {"source", f.Source},
        {"host", f.Host},
        {"program", f.Program},
        {"severity", f.Severity},
        {"facility", f.Facility},
    } {
        if pair.value != "" {
            labels = append(labels, fmt.Sprintf("%s=%q", pair.name, pair.value))
        }
    }
    query := "{" + strings.Join(labels, ",") + "}"

    if len(f.Extra) > 0 {
        keys := make([]string, 0, len(f.Extra))
        for k := range f.Extra {
            keys = append(keys, k)
        }
        sort.Strings(keys)

        query += " | json"
        for _, k := range keys {
            query += fmt.Sprintf(` | %s=%q`, k, f.Extra[k])
        }
    }

    if f.FreeText != "" {
        query += fmt.Sprintf(` |= %q`, f.FreeText)
    }

    return query
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd siem-api && go test ./internal/loki/... -v`
Expected: PASS (all `internal/loki` tests from Tasks 13-14).

- [ ] **Step 5: Commit**

```bash
git add siem-api/internal/loki/query.go siem-api/internal/loki/query_test.go
git commit -m "Add siem-api loki: BuildQuery with label discipline enforcement"
```

---

### Task 15: `internal/ntfy` — publish client

**Files:**
- Create: `siem-api/internal/ntfy/client.go`
- Test: `siem-api/internal/ntfy/client_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `Client`, `Client.Publish`, consumed by Task 17 (`alerts.Service.Raise`).

```go
type Client struct { baseURL, topic, token string; httpClient *http.Client }
func New(baseURL, topic, token string, httpClient *http.Client) *Client
func (c *Client) Publish(ctx context.Context, title, message, priority string) error
```

- [ ] **Step 1: Write the failing test**

`siem-api/internal/ntfy/client_test.go`:
```go
package ntfy

import (
    "context"
    "io"
    "net/http"
    "net/http/httptest"
    "testing"
)

func TestPublish_SendsExpectedRequest(t *testing.T) {
    var gotPath, gotTitle, gotPriority, gotAuth, gotBody string
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        gotPath = r.URL.Path
        gotTitle = r.Header.Get("Title")
        gotPriority = r.Header.Get("Priority")
        gotAuth = r.Header.Get("Authorization")
        body, _ := io.ReadAll(r.Body)
        gotBody = string(body)
        w.WriteHeader(http.StatusOK)
    }))
    defer srv.Close()

    c := New(srv.URL, "homesiem", "ntfy-token", srv.Client())
    err := c.Publish(context.Background(), "Port scan detected", "40 dropped connections from 10.0.0.5", "high")
    if err != nil {
        t.Fatalf("Publish() error = %v", err)
    }

    if gotPath != "/homesiem" {
        t.Errorf("path = %q, want /homesiem", gotPath)
    }
    if gotTitle != "Port scan detected" {
        t.Errorf("Title header = %q", gotTitle)
    }
    if gotPriority != "high" {
        t.Errorf("Priority header = %q, want high", gotPriority)
    }
    if gotAuth != "Bearer ntfy-token" {
        t.Errorf("Authorization header = %q, want Bearer ntfy-token", gotAuth)
    }
    if gotBody != "40 dropped connections from 10.0.0.5" {
        t.Errorf("body = %q", gotBody)
    }
}

func TestPublish_NoTokenOmitsAuthHeader(t *testing.T) {
    var gotAuth string
    hadAuth := false
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        gotAuth, hadAuth = r.Header.Get("Authorization"), r.Header.Get("Authorization") != ""
        w.WriteHeader(http.StatusOK)
    }))
    defer srv.Close()

    c := New(srv.URL, "homesiem", "", srv.Client())
    if err := c.Publish(context.Background(), "t", "m", "default"); err != nil {
        t.Fatalf("Publish() error = %v", err)
    }
    if hadAuth {
        t.Errorf("Authorization header = %q, want absent when no token configured", gotAuth)
    }
}

func TestPublish_ServerError(t *testing.T) {
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusInternalServerError)
    }))
    defer srv.Close()

    c := New(srv.URL, "homesiem", "", srv.Client())
    if err := c.Publish(context.Background(), "t", "m", "default"); err == nil {
        t.Fatal("Publish() error = nil, want error for 500 response")
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd siem-api && go test ./internal/ntfy/... -v`
Expected: FAIL — `New`/`Publish` undefined.

- [ ] **Step 3: Write the implementation**

`siem-api/internal/ntfy/client.go`:
```go
package ntfy

import (
    "context"
    "fmt"
    "net/http"
    "strings"
)

type Client struct {
    baseURL    string
    topic      string
    token      string
    httpClient *http.Client
}

func New(baseURL, topic, token string, httpClient *http.Client) *Client {
    if httpClient == nil {
        httpClient = http.DefaultClient
    }
    return &Client{baseURL: baseURL, topic: topic, token: token, httpClient: httpClient}
}

func (c *Client) Publish(ctx context.Context, title, message, priority string) error {
    req, err := http.NewRequestWithContext(ctx, http.MethodPost,
        strings.TrimRight(c.baseURL, "/")+"/"+c.topic, strings.NewReader(message))
    if err != nil {
        return err
    }
    req.Header.Set("Title", title)
    req.Header.Set("Priority", priority)
    if c.token != "" {
        req.Header.Set("Authorization", "Bearer "+c.token)
    }

    resp, err := c.httpClient.Do(req)
    if err != nil {
        return fmt.Errorf("ntfy: publish request: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode >= 300 {
        return fmt.Errorf("ntfy: publish failed: status=%d", resp.StatusCode)
    }
    return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd siem-api && go test ./internal/ntfy/... -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add siem-api/internal/ntfy/client.go siem-api/internal/ntfy/client_test.go
git commit -m "Add siem-api ntfy: publish client"
```

---

### Task 16: `internal/sse` — broadcast hub

**Files:**
- Create: `siem-api/internal/sse/hub.go`
- Test: `siem-api/internal/sse/hub_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `Hub`, consumed by Task 17 (`alerts.Service`, publishing to the `"alerts"` topic), Task 24 (`/events/tail`, publishing to a `"tail"` topic and serving both via `Hub.ServeHTTP`), Task 25 (`/alerts` SSE stream).

```go
type Hub struct{ /* unexported */ }
func NewHub() *Hub
func (h *Hub) Subscribe(topic string) (ch chan []byte, cancel func())
func (h *Hub) Publish(topic string, data []byte)
func (h *Hub) ServeHTTP(topic string, w http.ResponseWriter, r *http.Request)
```

A plain fan-out: `Publish` never blocks — a subscriber whose buffered channel (16 messages)
is full has its message dropped rather than stalling every other publisher, since a slow
browser tab must never back up alert delivery for everyone else. `cancel()` removes the
subscriber; `ServeHTTP` calls it via `defer` when the client disconnects (request context
cancellation), so no subscriber ever leaks.

- [ ] **Step 1: Write the failing test**

`siem-api/internal/sse/hub_test.go`:
```go
package sse

import (
    "context"
    "net/http"
    "net/http/httptest"
    "strings"
    "testing"
    "time"
)

func TestSubscribeAndPublish(t *testing.T) {
    h := NewHub()
    ch, cancel := h.Subscribe("alerts")
    defer cancel()

    h.Publish("alerts", []byte(`{"id":1}`))

    select {
    case msg := <-ch:
        if string(msg) != `{"id":1}` {
            t.Errorf("msg = %q", msg)
        }
    case <-time.After(time.Second):
        t.Fatal("timed out waiting for published message")
    }
}

func TestPublish_DifferentTopicNotReceived(t *testing.T) {
    h := NewHub()
    ch, cancel := h.Subscribe("alerts")
    defer cancel()

    h.Publish("tail", []byte("irrelevant"))

    select {
    case msg := <-ch:
        t.Fatalf("received message on wrong topic: %q", msg)
    case <-time.After(50 * time.Millisecond):
        // expected: nothing arrives
    }
}

func TestCancel_RemovesSubscriber(t *testing.T) {
    h := NewHub()
    _, cancel := h.Subscribe("alerts")
    if h.subscriberCount("alerts") != 1 {
        t.Fatalf("subscriberCount = %d, want 1", h.subscriberCount("alerts"))
    }
    cancel()
    if h.subscriberCount("alerts") != 0 {
        t.Fatalf("subscriberCount = %d, want 0 after cancel", h.subscriberCount("alerts"))
    }
}

func TestPublish_SlowSubscriberDoesNotBlock(t *testing.T) {
    h := NewHub()
    _, cancel := h.Subscribe("alerts") // never drained
    defer cancel()

    done := make(chan struct{})
    go func() {
        for i := 0; i < 100; i++ {
            h.Publish("alerts", []byte("x"))
        }
        close(done)
    }()

    select {
    case <-done:
    case <-time.After(time.Second):
        t.Fatal("Publish() blocked on a full subscriber channel")
    }
}

func TestServeHTTP_StreamsPublishedMessages(t *testing.T) {
    h := NewHub()
    ctx, cancel := context.WithCancel(context.Background())
    req := httptest.NewRequest(http.MethodGet, "/events/tail", nil).WithContext(ctx)
    rec := httptest.NewRecorder()

    handlerDone := make(chan struct{})
    go func() {
        h.ServeHTTP("tail", rec, req)
        close(handlerDone)
    }()

    // Give ServeHTTP a moment to subscribe before publishing.
    for h.subscriberCount("tail") == 0 {
        time.Sleep(time.Millisecond)
    }
    h.Publish("tail", []byte("hello"))
    time.Sleep(20 * time.Millisecond) // let the write land before we cancel

    cancel()
    select {
    case <-handlerDone:
    case <-time.After(time.Second):
        t.Fatal("ServeHTTP did not return after context cancellation")
    }

    if !strings.Contains(rec.Body.String(), "data: hello") {
        t.Errorf("body = %q, want it to contain %q", rec.Body.String(), "data: hello")
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd siem-api && go test ./internal/sse/... -v`
Expected: FAIL — `NewHub`/`Subscribe`/etc. undefined.

- [ ] **Step 3: Write the implementation**

`siem-api/internal/sse/hub.go`:
```go
package sse

import (
    "fmt"
    "net/http"
    "sync"
)

type Hub struct {
    mu   sync.Mutex
    subs map[string]map[chan []byte]struct{}
}

func NewHub() *Hub {
    return &Hub{subs: make(map[string]map[chan []byte]struct{})}
}

func (h *Hub) Subscribe(topic string) (chan []byte, func()) {
    ch := make(chan []byte, 16)

    h.mu.Lock()
    if h.subs[topic] == nil {
        h.subs[topic] = make(map[chan []byte]struct{})
    }
    h.subs[topic][ch] = struct{}{}
    h.mu.Unlock()

    cancel := func() {
        h.mu.Lock()
        delete(h.subs[topic], ch)
        h.mu.Unlock()
    }
    return ch, cancel
}

func (h *Hub) Publish(topic string, data []byte) {
    h.mu.Lock()
    defer h.mu.Unlock()

    for ch := range h.subs[topic] {
        select {
        case ch <- data:
        default:
            // Slow consumer: drop rather than block every other publish.
        }
    }
}

func (h *Hub) subscriberCount(topic string) int {
    h.mu.Lock()
    defer h.mu.Unlock()
    return len(h.subs[topic])
}

func (h *Hub) ServeHTTP(topic string, w http.ResponseWriter, r *http.Request) {
    flusher, ok := w.(http.Flusher)
    if !ok {
        http.Error(w, "streaming unsupported", http.StatusInternalServerError)
        return
    }

    w.Header().Set("Content-Type", "text/event-stream")
    w.Header().Set("Cache-Control", "no-cache")
    w.Header().Set("Connection", "keep-alive")
    w.WriteHeader(http.StatusOK)
    flusher.Flush()

    ch, cancel := h.Subscribe(topic)
    defer cancel()

    for {
        select {
        case <-r.Context().Done():
            return
        case data := <-ch:
            fmt.Fprintf(w, "data: %s\n\n", data)
            flusher.Flush()
        }
    }
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd siem-api && go test ./internal/sse/... -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add siem-api/internal/sse/hub.go siem-api/internal/sse/hub_test.go
git commit -m "Add siem-api sse: broadcast hub for alerts and tail streams"
```

---

### Task 17: `internal/alerts` — alert lifecycle (Raise)

**Files:**
- Create: `siem-api/internal/alerts/service.go`
- Test: `siem-api/internal/alerts/service_test.go`

**Interfaces:**
- Consumes: `store.Alert`/`store.Rule` (Tasks 6-7, via the `AlertStore` interface below), `sse.Hub` (Task 16), `ntfy.Client` (Task 15, via the `Notifier` interface below).
- Produces: `Candidate`, `Sample`, `Service`, `Service.Raise`. `Candidate` is consumed by every task that produces candidates for an alert — Task 18-20 (evaluators) and Task 23 (fastpath handler) — and by Task 21 (`rules.Raiser`, which references this package's `Candidate` type in its interface signature). This is the one place `rules` imports `alerts` (not the reverse — `alerts` never imports `rules`, keeping the dependency graph acyclic).

```go
type Sample struct {
    TS   time.Time
    Line string
}
type Candidate struct {
    RuleID   int64
    GroupKey string
    Severity string
    Title    string
    Body     string
    Samples  []Sample
    Context  map[string]any
}

type AlertStore interface {
    GetRule(ctx context.Context, id int64) (store.Rule, error)
    FindOpenAlert(ctx context.Context, ruleID int64, groupKey string) (*store.Alert, error)
    InsertAlert(ctx context.Context, a store.Alert) (store.Alert, error)
    TouchAlert(ctx context.Context, id int64, at time.Time) error
    ReopenAlert(ctx context.Context, id int64, at time.Time) error
    AddAlertSample(ctx context.Context, alertID int64, ts time.Time, line string) error
}
type Notifier interface {
    Publish(ctx context.Context, title, message, priority string) error
}
type Service struct{ /* unexported */ }
func NewService(store AlertStore, hub *sse.Hub, notifier Notifier, logger *slog.Logger) *Service
func (s *Service) Raise(ctx context.Context, c Candidate) error
```

Implements the design spec's four-step lifecycle exactly: within-cooldown → `TouchAlert`
only (no notify); past-cooldown or no existing alert → insert/reopen + notify (SSE publish
+ best-effort ntfy, logged not returned on ntfy failure); samples always appended,
regardless of the notify branch.

- [ ] **Step 1: Write the failing test**

`siem-api/internal/alerts/service_test.go`:
```go
package alerts

import (
    "context"
    "fmt"
    "io"
    "log/slog"
    "testing"
    "time"

    "github.com/hibikipr/homeSIEM/siem-api/internal/sse"
    "github.com/hibikipr/homeSIEM/siem-api/internal/store"
)

type fakeAlertStore struct {
    rules        map[int64]store.Rule
    openAlerts   map[string]*store.Alert // key: ruleID:groupKey
    nextID       int64
    inserted     []store.Alert
    touched      []int64
    reopened     []int64
    samplesAdded int
}

func newFakeAlertStore() *fakeAlertStore {
    return &fakeAlertStore{rules: map[int64]store.Rule{}, openAlerts: map[string]*store.Alert{}}
}

func key(ruleID int64, groupKey string) string {
    return fmt.Sprintf("%d:%s", ruleID, groupKey)
}

func (f *fakeAlertStore) GetRule(ctx context.Context, id int64) (store.Rule, error) {
    return f.rules[id], nil
}

func (f *fakeAlertStore) FindOpenAlert(ctx context.Context, ruleID int64, groupKey string) (*store.Alert, error) {
    return f.openAlerts[key(ruleID, groupKey)], nil
}

func (f *fakeAlertStore) InsertAlert(ctx context.Context, a store.Alert) (store.Alert, error) {
    f.nextID++
    a.ID = f.nextID
    f.inserted = append(f.inserted, a)
    f.openAlerts[key(a.RuleID, a.GroupKey)] = &a
    return a, nil
}

func (f *fakeAlertStore) TouchAlert(ctx context.Context, id int64, at time.Time) error {
    f.touched = append(f.touched, id)
    for _, a := range f.openAlerts {
        if a.ID == id {
            a.EventCount++
            a.LastSeenAt = at
        }
    }
    return nil
}

func (f *fakeAlertStore) ReopenAlert(ctx context.Context, id int64, at time.Time) error {
    f.reopened = append(f.reopened, id)
    for _, a := range f.openAlerts {
        if a.ID == id {
            a.LastSeenAt = at
        }
    }
    return nil
}

func (f *fakeAlertStore) AddAlertSample(ctx context.Context, alertID int64, ts time.Time, line string) error {
    f.samplesAdded++
    return nil
}

type fakeNotifier struct {
    calls int
}

func (f *fakeNotifier) Publish(ctx context.Context, title, message, priority string) error {
    f.calls++
    return nil
}

func testLogger() *slog.Logger {
    return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestRaise_NewAlert_InsertsAndNotifies(t *testing.T) {
    fs := newFakeAlertStore()
    fs.rules[1] = store.Rule{ID: 1, CooldownSec: 3600}
    hub := sse.NewHub()
    ch, cancel := hub.Subscribe("alerts")
    defer cancel()
    notifier := &fakeNotifier{}

    svc := NewService(fs, hub, notifier, testLogger())
    err := svc.Raise(context.Background(), Candidate{
        RuleID: 1, GroupKey: "10.0.0.5", Severity: "critical",
        Title: "Port scan", Body: "40 drops", Samples: []Sample{{TS: time.Now(), Line: "line1"}},
    })
    if err != nil {
        t.Fatalf("Raise() error = %v", err)
    }

    if len(fs.inserted) != 1 {
        t.Fatalf("inserted = %d, want 1", len(fs.inserted))
    }
    if fs.samplesAdded != 1 {
        t.Errorf("samplesAdded = %d, want 1", fs.samplesAdded)
    }
    if notifier.calls != 1 {
        t.Errorf("notifier.calls = %d, want 1", notifier.calls)
    }

    select {
    case <-ch:
    case <-time.After(time.Second):
        t.Fatal("expected an SSE publish for a new alert")
    }
}

func TestRaise_WithinCooldown_TouchesOnlyNoNotify(t *testing.T) {
    fs := newFakeAlertStore()
    fs.rules[1] = store.Rule{ID: 1, CooldownSec: 3600}
    now := time.Now().UTC()
    fs.openAlerts[key(1, "10.0.0.5")] = &store.Alert{ID: 99, RuleID: 1, GroupKey: "10.0.0.5", LastSeenAt: now}
    hub := sse.NewHub()
    ch, cancel := hub.Subscribe("alerts")
    defer cancel()
    notifier := &fakeNotifier{}

    svc := NewService(fs, hub, notifier, testLogger())
    err := svc.Raise(context.Background(), Candidate{RuleID: 1, GroupKey: "10.0.0.5", Severity: "critical", Title: "t", Body: "b"})
    if err != nil {
        t.Fatalf("Raise() error = %v", err)
    }

    if len(fs.touched) != 1 || fs.touched[0] != 99 {
        t.Errorf("touched = %v, want [99]", fs.touched)
    }
    if len(fs.inserted) != 0 || len(fs.reopened) != 0 {
        t.Error("expected no insert/reopen within cooldown")
    }
    if notifier.calls != 0 {
        t.Errorf("notifier.calls = %d, want 0 within cooldown", notifier.calls)
    }

    select {
    case msg := <-ch:
        t.Fatalf("expected no SSE publish within cooldown, got %q", msg)
    case <-time.After(50 * time.Millisecond):
    }
}

func TestRaise_PastCooldown_ReopensAndNotifies(t *testing.T) {
    fs := newFakeAlertStore()
    fs.rules[1] = store.Rule{ID: 1, CooldownSec: 60}
    old := time.Now().UTC().Add(-time.Hour)
    fs.openAlerts[key(1, "10.0.0.5")] = &store.Alert{ID: 99, RuleID: 1, GroupKey: "10.0.0.5", LastSeenAt: old}
    hub := sse.NewHub()
    notifier := &fakeNotifier{}

    svc := NewService(fs, hub, notifier, testLogger())
    err := svc.Raise(context.Background(), Candidate{RuleID: 1, GroupKey: "10.0.0.5", Severity: "critical", Title: "t", Body: "b"})
    if err != nil {
        t.Fatalf("Raise() error = %v", err)
    }

    if len(fs.reopened) != 1 || fs.reopened[0] != 99 {
        t.Errorf("reopened = %v, want [99]", fs.reopened)
    }
    if notifier.calls != 1 {
        t.Errorf("notifier.calls = %d, want 1 past cooldown", notifier.calls)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd siem-api && go test ./internal/alerts/... -v`
Expected: FAIL — package undefined.

- [ ] **Step 3: Write the implementation**

`siem-api/internal/alerts/service.go`:
```go
package alerts

import (
    "context"
    "encoding/json"
    "log/slog"
    "time"

    "github.com/hibikipr/homeSIEM/siem-api/internal/sse"
    "github.com/hibikipr/homeSIEM/siem-api/internal/store"
)

type Sample struct {
    TS   time.Time
    Line string
}

type Candidate struct {
    RuleID   int64
    GroupKey string
    Severity string
    Title    string
    Body     string
    Samples  []Sample
    Context  map[string]any
}

type AlertStore interface {
    GetRule(ctx context.Context, id int64) (store.Rule, error)
    FindOpenAlert(ctx context.Context, ruleID int64, groupKey string) (*store.Alert, error)
    InsertAlert(ctx context.Context, a store.Alert) (store.Alert, error)
    TouchAlert(ctx context.Context, id int64, at time.Time) error
    ReopenAlert(ctx context.Context, id int64, at time.Time) error
    AddAlertSample(ctx context.Context, alertID int64, ts time.Time, line string) error
}

type Notifier interface {
    Publish(ctx context.Context, title, message, priority string) error
}

type Service struct {
    store    AlertStore
    hub      *sse.Hub
    notifier Notifier
    logger   *slog.Logger
}

func NewService(s AlertStore, hub *sse.Hub, notifier Notifier, logger *slog.Logger) *Service {
    return &Service{store: s, hub: hub, notifier: notifier, logger: logger}
}

func (s *Service) Raise(ctx context.Context, c Candidate) error {
    rule, err := s.store.GetRule(ctx, c.RuleID)
    if err != nil {
        return err
    }

    contextJSON, err := json.Marshal(c.Context)
    if err != nil {
        return err
    }

    existing, err := s.store.FindOpenAlert(ctx, c.RuleID, c.GroupKey)
    if err != nil {
        return err
    }

    now := time.Now().UTC()
    var alertID int64
    notify := false

    switch {
    case existing != nil && now.Sub(existing.LastSeenAt) < time.Duration(rule.CooldownSec)*time.Second:
        if err := s.store.TouchAlert(ctx, existing.ID, now); err != nil {
            return err
        }
        alertID = existing.ID

    case existing != nil:
        if err := s.store.ReopenAlert(ctx, existing.ID, now); err != nil {
            return err
        }
        alertID = existing.ID
        notify = true

    default:
        inserted, err := s.store.InsertAlert(ctx, store.Alert{
            RuleID: c.RuleID, GroupKey: c.GroupKey, Severity: c.Severity,
            Title: c.Title, Body: c.Body, EventCount: 1, Context: string(contextJSON),
            State: "open", FirstSeenAt: now, LastSeenAt: now,
        })
        if err != nil {
            return err
        }
        alertID = inserted.ID
        notify = true
    }

    for _, sample := range c.Samples {
        if err := s.store.AddAlertSample(ctx, alertID, sample.TS, sample.Line); err != nil {
            return err
        }
    }

    if !notify {
        return nil
    }

    payload, _ := json.Marshal(struct {
        ID       int64  `json:"id"`
        RuleID   int64  `json:"rule_id"`
        Severity string `json:"severity"`
        Title    string `json:"title"`
        Body     string `json:"body"`
    }{alertID, c.RuleID, c.Severity, c.Title, c.Body})
    s.hub.Publish("alerts", payload)

    if s.notifier != nil {
        if err := s.notifier.Publish(ctx, c.Title, c.Body, severityToPriority(c.Severity)); err != nil {
            s.logger.Error("ntfy publish failed", "error", err, "alert_id", alertID)
        }
    }
    return nil
}

func severityToPriority(severity string) string {
    switch severity {
    case "critical", "error":
        return "urgent"
    case "warning":
        return "high"
    default:
        return "default"
    }
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd siem-api && go test ./internal/alerts/... -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add siem-api/internal/alerts/service.go siem-api/internal/alerts/service_test.go
git commit -m "Add siem-api alerts: lifecycle (Raise) with dedupe, cooldown, notify"
```

---

### Task 18: `internal/rules` — ThresholdEvaluator

**Files:**
- Create: `siem-api/internal/rules/threshold.go`
- Test: `siem-api/internal/rules/threshold_test.go`

**Interfaces:**
- Consumes: `alerts.Candidate`/`alerts.Sample` (Task 17), `loki.LogEntry`/`loki.QueryResult` (Task 13), `store.Rule` (Task 6).
- Produces: `LokiQuerier` interface, `Evaluator` interface, `ThresholdEvaluator`, consumed by Task 21 (`Scheduler`, which dispatches to whichever `Evaluator` matches a rule's `Shape`).

```go
type LokiQuerier interface {
    QueryRange(ctx context.Context, logql string, start, end time.Time, limit int) (loki.QueryResult, error)
}
type Evaluator interface {
    Evaluate(ctx context.Context, rule store.Rule) ([]alerts.Candidate, error)
}
type ThresholdEvaluator struct{ Querier LokiQuerier }
func (e *ThresholdEvaluator) Evaluate(ctx context.Context, rule store.Rule) ([]alerts.Candidate, error)
```

Queries `rule.LogQL` over `[now-WindowSec, now]`, groups matched entries by parsing each
entry's `Line` as JSON and reading the `rule.GroupBy` field names out of it (log lines are
JSON per the label-discipline design in Task 14 — grouping fields like `src_ip` are never
Loki labels, so they only exist in the parsed line body), and emits one `Candidate` per
group whose count reaches `rule.Threshold`. A rule with no `GroupBy` groups everything into
a single bucket (key `"_all"`). Up to 5 sample lines per candidate.

- [ ] **Step 1: Write the failing test**

`siem-api/internal/rules/threshold_test.go`:
```go
package rules

import (
    "context"
    "testing"
    "time"

    "github.com/hibikipr/homeSIEM/siem-api/internal/loki"
    "github.com/hibikipr/homeSIEM/siem-api/internal/store"
)

type fakeLokiQuerier struct {
    result loki.QueryResult
    err    error
    gotLogQL string
}

func (f *fakeLokiQuerier) QueryRange(ctx context.Context, logql string, start, end time.Time, limit int) (loki.QueryResult, error) {
    f.gotLogQL = logql
    return f.result, f.err
}

func TestThresholdEvaluator_FiresWhenThresholdReached(t *testing.T) {
    now := time.Now().UTC()
    querier := &fakeLokiQuerier{result: loki.QueryResult{Entries: []loki.LogEntry{
        {Timestamp: now, Line: `{"src_ip":"10.0.0.5","dst_port":22}`},
        {Timestamp: now, Line: `{"src_ip":"10.0.0.5","dst_port":23}`},
        {Timestamp: now, Line: `{"src_ip":"10.0.0.5","dst_port":25}`},
        {Timestamp: now, Line: `{"src_ip":"10.0.0.9","dst_port":80}`},
    }}}
    e := &ThresholdEvaluator{Querier: querier}

    threshold := 3
    rule := store.Rule{ID: 1, Name: "wan-portscan", LogQL: `{job="siem"}`, WindowSec: 60,
        Threshold: &threshold, GroupBy: []string{"src_ip"}, Severity: "critical"}

    candidates, err := e.Evaluate(context.Background(), rule)
    if err != nil {
        t.Fatalf("Evaluate() error = %v", err)
    }
    if len(candidates) != 1 {
        t.Fatalf("len(candidates) = %d, want 1", len(candidates))
    }
    if candidates[0].GroupKey != "10.0.0.5" {
        t.Errorf("GroupKey = %q, want 10.0.0.5", candidates[0].GroupKey)
    }
    if candidates[0].RuleID != 1 {
        t.Errorf("RuleID = %d, want 1", candidates[0].RuleID)
    }
    if len(candidates[0].Samples) != 3 {
        t.Errorf("len(Samples) = %d, want 3", len(candidates[0].Samples))
    }

    if querier.gotLogQL != `{job="siem"}` {
        t.Errorf("QueryRange logql = %q, want rule.LogQL", querier.gotLogQL)
    }
}

func TestThresholdEvaluator_BelowThresholdNoCandidate(t *testing.T) {
    now := time.Now().UTC()
    querier := &fakeLokiQuerier{result: loki.QueryResult{Entries: []loki.LogEntry{
        {Timestamp: now, Line: `{"src_ip":"10.0.0.5"}`},
    }}}
    e := &ThresholdEvaluator{Querier: querier}

    threshold := 3
    rule := store.Rule{ID: 1, LogQL: `{job="siem"}`, WindowSec: 60, Threshold: &threshold, GroupBy: []string{"src_ip"}}

    candidates, err := e.Evaluate(context.Background(), rule)
    if err != nil {
        t.Fatalf("Evaluate() error = %v", err)
    }
    if len(candidates) != 0 {
        t.Fatalf("len(candidates) = %d, want 0", len(candidates))
    }
}

func TestThresholdEvaluator_NoGroupByGroupsAll(t *testing.T) {
    now := time.Now().UTC()
    querier := &fakeLokiQuerier{result: loki.QueryResult{Entries: []loki.LogEntry{
        {Timestamp: now, Line: `{"anything":"a"}`},
        {Timestamp: now, Line: `{"anything":"b"}`},
    }}}
    e := &ThresholdEvaluator{Querier: querier}

    threshold := 2
    rule := store.Rule{ID: 1, LogQL: `{job="siem"}`, WindowSec: 60, Threshold: &threshold}

    candidates, err := e.Evaluate(context.Background(), rule)
    if err != nil {
        t.Fatalf("Evaluate() error = %v", err)
    }
    if len(candidates) != 1 || candidates[0].GroupKey != "_all" {
        t.Fatalf("candidates = %+v, want one candidate with GroupKey _all", candidates)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd siem-api && go test ./internal/rules/... -v`
Expected: FAIL — package undefined.

- [ ] **Step 3: Write the implementation**

`siem-api/internal/rules/threshold.go`:
```go
package rules

import (
    "context"
    "encoding/json"
    "fmt"
    "strings"
    "time"

    "github.com/hibikipr/homeSIEM/siem-api/internal/alerts"
    "github.com/hibikipr/homeSIEM/siem-api/internal/loki"
    "github.com/hibikipr/homeSIEM/siem-api/internal/store"
)

type LokiQuerier interface {
    QueryRange(ctx context.Context, logql string, start, end time.Time, limit int) (loki.QueryResult, error)
}

type Evaluator interface {
    Evaluate(ctx context.Context, rule store.Rule) ([]alerts.Candidate, error)
}

type ThresholdEvaluator struct {
    Querier LokiQuerier
}

func (e *ThresholdEvaluator) Evaluate(ctx context.Context, rule store.Rule) ([]alerts.Candidate, error) {
    end := time.Now().UTC()
    start := end.Add(-time.Duration(rule.WindowSec) * time.Second)

    result, err := e.Querier.QueryRange(ctx, rule.LogQL, start, end, 5000)
    if err != nil {
        return nil, err
    }

    threshold := 1
    if rule.Threshold != nil {
        threshold = *rule.Threshold
    }

    groups := map[string][]loki.LogEntry{}
    for _, entry := range result.Entries {
        k := groupKeyFor(entry, rule.GroupBy)
        groups[k] = append(groups[k], entry)
    }

    var candidates []alerts.Candidate
    for k, entries := range groups {
        if len(entries) < threshold {
            continue
        }
        candidates = append(candidates, alerts.Candidate{
            RuleID:   rule.ID,
            GroupKey: k,
            Severity: rule.Severity,
            Title:    fmt.Sprintf("%s: %d events for %s", rule.Name, len(entries), k),
            Body:     fmt.Sprintf("%d matching events in the last %ds", len(entries), rule.WindowSec),
            Samples:  samplesFrom(entries),
        })
    }
    return candidates, nil
}

func groupKeyFor(entry loki.LogEntry, groupBy []string) string {
    if len(groupBy) == 0 {
        return "_all"
    }
    var fields map[string]any
    _ = json.Unmarshal([]byte(entry.Line), &fields) // best-effort; ungrouped fields become ""

    parts := make([]string, len(groupBy))
    for i, field := range groupBy {
        if v, ok := fields[field]; ok {
            parts[i] = fmt.Sprintf("%v", v)
        }
    }
    return strings.Join(parts, "|")
}

func samplesFrom(entries []loki.LogEntry) []alerts.Sample {
    n := len(entries)
    if n > 5 {
        n = 5
    }
    out := make([]alerts.Sample, n)
    for i := 0; i < n; i++ {
        out[i] = alerts.Sample{TS: entries[i].Timestamp, Line: entries[i].Line}
    }
    return out
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd siem-api && go test ./internal/rules/... -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add siem-api/internal/rules/threshold.go siem-api/internal/rules/threshold_test.go
git commit -m "Add siem-api rules: ThresholdEvaluator"
```

---

### Task 19: `internal/rules` — FirstSeenEvaluator

**Files:**
- Create: `siem-api/internal/rules/first_seen.go`
- Test: `siem-api/internal/rules/first_seen_test.go`

**Interfaces:**
- Consumes: `LokiQuerier`, `groupKeyFor`, `samplesFrom` (Task 18, same package), `alerts.Candidate` (Task 17).
- Produces: `SeenStore` interface, `FirstSeenEvaluator`, consumed by Task 21 (`Scheduler`) and Task 29 (`main.go` wires `*store.Store` into it — `*store.Store` satisfies `SeenStore` via its Task 9 methods).

```go
type SeenStore interface {
    HasSeenValue(ctx context.Context, ruleID int64, value string) (bool, error)
    MarkSeenValue(ctx context.Context, ruleID int64, value string, at time.Time) error
}
type FirstSeenEvaluator struct{ Querier LokiQuerier; Seen SeenStore }
func (e *FirstSeenEvaluator) Evaluate(ctx context.Context, rule store.Rule) ([]alerts.Candidate, error)
```

Queries the same way as `ThresholdEvaluator`, groups entries by `rule.GroupBy` using the
same `groupKeyFor` helper (a `first_seen` rule's `GroupBy` names the single field being
watched, e.g. `["domain"]`), then for each distinct value seen in this window: skips it if
`SeenStore.HasSeenValue` says it's been seen before, otherwise marks it seen and emits a
candidate. Values with an empty group key (couldn't be parsed from the line) are skipped —
there's nothing meaningful to diff against "seen before".

- [ ] **Step 1: Write the failing test**

`siem-api/internal/rules/first_seen_test.go`:
```go
package rules

import (
    "context"
    "testing"
    "time"

    "github.com/hibikipr/homeSIEM/siem-api/internal/loki"
    "github.com/hibikipr/homeSIEM/siem-api/internal/store"
)

type fakeSeenStore struct {
    seen  map[string]bool
    marks []string
}

func newFakeSeenStore(alreadySeen ...string) *fakeSeenStore {
    s := &fakeSeenStore{seen: map[string]bool{}}
    for _, v := range alreadySeen {
        s.seen[v] = true
    }
    return s
}

func (f *fakeSeenStore) HasSeenValue(ctx context.Context, ruleID int64, value string) (bool, error) {
    return f.seen[value], nil
}

func (f *fakeSeenStore) MarkSeenValue(ctx context.Context, ruleID int64, value string, at time.Time) error {
    f.marks = append(f.marks, value)
    f.seen[value] = true
    return nil
}

func TestFirstSeenEvaluator_NewValueFires(t *testing.T) {
    now := time.Now().UTC()
    querier := &fakeLokiQuerier{result: loki.QueryResult{Entries: []loki.LogEntry{
        {Timestamp: now, Line: `{"domain":"new-domain.example"}`},
    }}}
    seen := newFakeSeenStore() // nothing seen yet
    e := &FirstSeenEvaluator{Querier: querier, Seen: seen}

    rule := store.Rule{ID: 1, Name: "new-domain-burst", LogQL: `{job="siem"}`, WindowSec: 3600,
        GroupBy: []string{"domain"}, Severity: "warning"}

    candidates, err := e.Evaluate(context.Background(), rule)
    if err != nil {
        t.Fatalf("Evaluate() error = %v", err)
    }
    if len(candidates) != 1 || candidates[0].GroupKey != "new-domain.example" {
        t.Fatalf("candidates = %+v, want one for new-domain.example", candidates)
    }
    if len(seen.marks) != 1 || seen.marks[0] != "new-domain.example" {
        t.Errorf("marks = %v, want [new-domain.example]", seen.marks)
    }
}

func TestFirstSeenEvaluator_AlreadySeenNoFire(t *testing.T) {
    now := time.Now().UTC()
    querier := &fakeLokiQuerier{result: loki.QueryResult{Entries: []loki.LogEntry{
        {Timestamp: now, Line: `{"domain":"known-domain.example"}`},
    }}}
    seen := newFakeSeenStore("known-domain.example")
    e := &FirstSeenEvaluator{Querier: querier, Seen: seen}

    rule := store.Rule{ID: 1, LogQL: `{job="siem"}`, WindowSec: 3600, GroupBy: []string{"domain"}}

    candidates, err := e.Evaluate(context.Background(), rule)
    if err != nil {
        t.Fatalf("Evaluate() error = %v", err)
    }
    if len(candidates) != 0 {
        t.Fatalf("candidates = %+v, want none for an already-seen value", candidates)
    }
    if len(seen.marks) != 0 {
        t.Errorf("marks = %v, want none — MarkSeenValue must not be called for already-seen values", seen.marks)
    }
}

func TestFirstSeenEvaluator_DedupesWithinBatch(t *testing.T) {
    now := time.Now().UTC()
    querier := &fakeLokiQuerier{result: loki.QueryResult{Entries: []loki.LogEntry{
        {Timestamp: now, Line: `{"domain":"dup.example"}`},
        {Timestamp: now, Line: `{"domain":"dup.example"}`},
    }}}
    seen := newFakeSeenStore()
    e := &FirstSeenEvaluator{Querier: querier, Seen: seen}

    rule := store.Rule{ID: 1, LogQL: `{job="siem"}`, WindowSec: 3600, GroupBy: []string{"domain"}}

    candidates, err := e.Evaluate(context.Background(), rule)
    if err != nil {
        t.Fatalf("Evaluate() error = %v", err)
    }
    if len(candidates) != 1 {
        t.Fatalf("len(candidates) = %d, want 1 (two entries, same new value)", len(candidates))
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd siem-api && go test ./internal/rules/... -run FirstSeen -v`
Expected: FAIL — `SeenStore`/`FirstSeenEvaluator` undefined.

- [ ] **Step 3: Write the implementation**

`siem-api/internal/rules/first_seen.go`:
```go
package rules

import (
    "context"
    "fmt"
    "time"

    "github.com/hibikipr/homeSIEM/siem-api/internal/alerts"
    "github.com/hibikipr/homeSIEM/siem-api/internal/loki"
    "github.com/hibikipr/homeSIEM/siem-api/internal/store"
)

type SeenStore interface {
    HasSeenValue(ctx context.Context, ruleID int64, value string) (bool, error)
    MarkSeenValue(ctx context.Context, ruleID int64, value string, at time.Time) error
}

type FirstSeenEvaluator struct {
    Querier LokiQuerier
    Seen    SeenStore
}

func (e *FirstSeenEvaluator) Evaluate(ctx context.Context, rule store.Rule) ([]alerts.Candidate, error) {
    end := time.Now().UTC()
    start := end.Add(-time.Duration(rule.WindowSec) * time.Second)

    result, err := e.Querier.QueryRange(ctx, rule.LogQL, start, end, 5000)
    if err != nil {
        return nil, err
    }

    byValue := map[string][]loki.LogEntry{}
    for _, entry := range result.Entries {
        v := groupKeyFor(entry, rule.GroupBy)
        if v == "" {
            continue
        }
        byValue[v] = append(byValue[v], entry)
    }

    var candidates []alerts.Candidate
    for value, entries := range byValue {
        seen, err := e.Seen.HasSeenValue(ctx, rule.ID, value)
        if err != nil {
            return nil, err
        }
        if seen {
            continue
        }
        if err := e.Seen.MarkSeenValue(ctx, rule.ID, value, time.Now().UTC()); err != nil {
            return nil, err
        }

        candidates = append(candidates, alerts.Candidate{
            RuleID:   rule.ID,
            GroupKey: value,
            Severity: rule.Severity,
            Title:    fmt.Sprintf("%s: new value %q", rule.Name, value),
            Body:     fmt.Sprintf("%q was not observed before this window", value),
            Samples:  samplesFrom(entries),
        })
    }
    return candidates, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd siem-api && go test ./internal/rules/... -run FirstSeen -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add siem-api/internal/rules/first_seen.go siem-api/internal/rules/first_seen_test.go
git commit -m "Add siem-api rules: FirstSeenEvaluator"
```

---

### Task 20: `internal/rules` — AbsenceEvaluator

**Files:**
- Create: `siem-api/internal/rules/absence.go`
- Test: `siem-api/internal/rules/absence_test.go`

**Interfaces:**
- Consumes: `store.Source` (Task 5), `alerts.Candidate` (Task 17).
- Produces: `SourcesStore` interface, `AbsenceEvaluator`, consumed by Task 21 (`Scheduler`) and Task 29 (`main.go` — `*store.Store` satisfies `SourcesStore` via its Task 5 `StaleSources` method).

```go
type SourcesStore interface {
    StaleSources(ctx context.Context, now time.Time) ([]store.Source, error)
}
type AbsenceEvaluator struct{ Sources SourcesStore }
func (e *AbsenceEvaluator) Evaluate(ctx context.Context, rule store.Rule) ([]alerts.Candidate, error)
```

Ignores `rule.LogQL`/`WindowSec`/`GroupBy` entirely — this is the one evaluator shape that
never queries Loki (per the design spec). It emits one candidate per stale source on every
tick a source stays silent; `alerts.Service.Raise`'s cooldown check (Task 17) is what
prevents that from spamming notifications — repeated candidates for the same source just
touch the existing open alert until the cooldown lapses.

- [ ] **Step 1: Write the failing test**

`siem-api/internal/rules/absence_test.go`:
```go
package rules

import (
    "context"
    "testing"
    "time"

    "github.com/hibikipr/homeSIEM/siem-api/internal/store"
)

type fakeSourcesStore struct {
    stale []store.Source
    err   error
}

func (f *fakeSourcesStore) StaleSources(ctx context.Context, now time.Time) ([]store.Source, error) {
    return f.stale, f.err
}

func TestAbsenceEvaluator_OneCandidatePerStaleSource(t *testing.T) {
    lastSeen := time.Now().UTC().Add(-2 * time.Hour)
    sources := &fakeSourcesStore{stale: []store.Source{
        {Name: "silent-host", HeartbeatSec: 900, LastSeenAt: &lastSeen},
        {Name: "another-host", HeartbeatSec: 60, LastSeenAt: nil},
    }}
    e := &AbsenceEvaluator{Sources: sources}

    rule := store.Rule{ID: 1, Name: "source-heartbeat", Severity: "warning"}
    candidates, err := e.Evaluate(context.Background(), rule)
    if err != nil {
        t.Fatalf("Evaluate() error = %v", err)
    }
    if len(candidates) != 2 {
        t.Fatalf("len(candidates) = %d, want 2", len(candidates))
    }

    var groupKeys []string
    for _, c := range candidates {
        groupKeys = append(groupKeys, c.GroupKey)
        if c.RuleID != 1 || c.Severity != "warning" {
            t.Errorf("candidate = %+v, want RuleID=1 Severity=warning", c)
        }
    }
    if groupKeys[0] != "silent-host" && groupKeys[1] != "silent-host" {
        t.Errorf("groupKeys = %v, want to contain silent-host", groupKeys)
    }
}

func TestAbsenceEvaluator_NoneStaleNoCandidates(t *testing.T) {
    sources := &fakeSourcesStore{stale: nil}
    e := &AbsenceEvaluator{Sources: sources}

    candidates, err := e.Evaluate(context.Background(), store.Rule{ID: 1})
    if err != nil {
        t.Fatalf("Evaluate() error = %v", err)
    }
    if len(candidates) != 0 {
        t.Fatalf("len(candidates) = %d, want 0", len(candidates))
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd siem-api && go test ./internal/rules/... -run Absence -v`
Expected: FAIL — `SourcesStore`/`AbsenceEvaluator` undefined.

- [ ] **Step 3: Write the implementation**

`siem-api/internal/rules/absence.go`:
```go
package rules

import (
    "context"
    "fmt"
    "time"

    "github.com/hibikipr/homeSIEM/siem-api/internal/alerts"
    "github.com/hibikipr/homeSIEM/siem-api/internal/store"
)

type SourcesStore interface {
    StaleSources(ctx context.Context, now time.Time) ([]store.Source, error)
}

type AbsenceEvaluator struct {
    Sources SourcesStore
}

func (e *AbsenceEvaluator) Evaluate(ctx context.Context, rule store.Rule) ([]alerts.Candidate, error) {
    stale, err := e.Sources.StaleSources(ctx, time.Now().UTC())
    if err != nil {
        return nil, err
    }

    var candidates []alerts.Candidate
    for _, src := range stale {
        lastSeen := "never"
        if src.LastSeenAt != nil {
            lastSeen = src.LastSeenAt.Format(time.RFC3339)
        }
        candidates = append(candidates, alerts.Candidate{
            RuleID:   rule.ID,
            GroupKey: src.Name,
            Severity: rule.Severity,
            Title:    fmt.Sprintf("%s: source %q has gone silent", rule.Name, src.Name),
            Body:     fmt.Sprintf("no events from %q since %s (heartbeat %ds)", src.Name, lastSeen, src.HeartbeatSec),
        })
    }
    return candidates, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd siem-api && go test ./internal/rules/... -run Absence -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add siem-api/internal/rules/absence.go siem-api/internal/rules/absence_test.go
git commit -m "Add siem-api rules: AbsenceEvaluator"
```

---

### Task 21: `internal/rules` — Scheduler

**Files:**
- Create: `siem-api/internal/rules/scheduler.go`
- Test: `siem-api/internal/rules/scheduler_test.go`

**Interfaces:**
- Consumes: `Evaluator` (Task 18), `alerts.Candidate` (Task 17), `store.Rule` (Task 6).
- Produces: `RulesStore`, `Raiser` interfaces and `Scheduler`, consumed by Task 26 (`/rules` handlers call `StartRule`/`StopRule` on create/update/delete) and Task 29 (`main.go` builds and `Start`s it).

```go
type RulesStore interface {
    ListEnabledRules(ctx context.Context) ([]store.Rule, error)
    TouchRuleLastRun(ctx context.Context, id int64, at time.Time) error
}
type Raiser interface {
    Raise(ctx context.Context, c alerts.Candidate) error
}
type Scheduler struct{ /* unexported */ }
func NewScheduler(rulesStore RulesStore, evaluators map[string]Evaluator, raiser Raiser, logger *slog.Logger) *Scheduler
func (s *Scheduler) Start(ctx context.Context) error
func (s *Scheduler) StartRule(ctx context.Context, rule store.Rule)
func (s *Scheduler) StopRule(ruleID int64)
func (s *Scheduler) Stop()
```

`evaluators` is keyed by `Rule.Shape` (`"threshold"`, `"first_seen"`, `"absence"`) — `main.go`
(Task 29) wires `map[string]Evaluator{"threshold": &ThresholdEvaluator{...}, "first_seen":
&FirstSeenEvaluator{...}, "absence": &AbsenceEvaluator{...}}`.

`StartRule` spawns one goroutine per rule: a random jitter (0..`IntervalSec`) before the
first evaluation so a dozen rules don't all query Loki simultaneously, then a `time.Ticker`
at `IntervalSec` thereafter. `StopRule` cancels that rule's goroutine via a per-rule
`context.CancelFunc` tracked in a map — calling `StartRule` again for the same rule ID
(a rule update) cancels any existing goroutine for it first, so there's never more than
one running per rule. `Stop` cancels everything and waits for all goroutines to exit —
`*api.Server` (Task 29) calls it during graceful shutdown.

- [ ] **Step 1: Write the failing test**

`siem-api/internal/rules/scheduler_test.go`:
```go
package rules

import (
    "context"
    "io"
    "log/slog"
    "sync"
    "testing"
    "time"

    "github.com/hibikipr/homeSIEM/siem-api/internal/alerts"
    "github.com/hibikipr/homeSIEM/siem-api/internal/store"
)

func schedulerTestLogger() *slog.Logger {
    return slog.New(slog.NewTextHandler(io.Discard, nil))
}

type fakeEvaluator struct {
    mu    sync.Mutex
    calls int
    out   []alerts.Candidate
}

func (f *fakeEvaluator) Evaluate(ctx context.Context, rule store.Rule) ([]alerts.Candidate, error) {
    f.mu.Lock()
    f.calls++
    f.mu.Unlock()
    return f.out, nil
}

func (f *fakeEvaluator) callCount() int {
    f.mu.Lock()
    defer f.mu.Unlock()
    return f.calls
}

type fakeRaiser struct {
    ch chan alerts.Candidate
}

func newFakeRaiser() *fakeRaiser {
    return &fakeRaiser{ch: make(chan alerts.Candidate, 10)}
}

func (f *fakeRaiser) Raise(ctx context.Context, c alerts.Candidate) error {
    f.ch <- c
    return nil
}

type fakeRulesStore struct {
    enabled []store.Rule
    touchCh chan int64
}

func newFakeRulesStore(rules ...store.Rule) *fakeRulesStore {
    return &fakeRulesStore{enabled: rules, touchCh: make(chan int64, 10)}
}

func (f *fakeRulesStore) ListEnabledRules(ctx context.Context) ([]store.Rule, error) {
    return f.enabled, nil
}

func (f *fakeRulesStore) TouchRuleLastRun(ctx context.Context, id int64, at time.Time) error {
    f.touchCh <- id
    return nil
}

func TestScheduler_StartRule_EvaluatesAndRaises(t *testing.T) {
    evaluator := &fakeEvaluator{out: []alerts.Candidate{{RuleID: 1, GroupKey: "g"}}}
    raiser := newFakeRaiser()
    rulesStore := newFakeRulesStore()
    s := NewScheduler(rulesStore, map[string]Evaluator{"threshold": evaluator}, raiser, schedulerTestLogger())

    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()
    s.StartRule(ctx, store.Rule{ID: 1, Shape: "threshold", IntervalSec: 1})
    defer s.Stop()

    select {
    case c := <-raiser.ch:
        if c.RuleID != 1 {
            t.Errorf("Raise() candidate RuleID = %d, want 1", c.RuleID)
        }
    case <-time.After(3 * time.Second):
        t.Fatal("timed out waiting for scheduler to evaluate and raise")
    }
}

func TestScheduler_TouchesRuleLastRun(t *testing.T) {
    evaluator := &fakeEvaluator{}
    raiser := newFakeRaiser()
    rulesStore := newFakeRulesStore()
    s := NewScheduler(rulesStore, map[string]Evaluator{"absence": evaluator}, raiser, schedulerTestLogger())

    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()
    s.StartRule(ctx, store.Rule{ID: 5, Shape: "absence", IntervalSec: 1})
    defer s.Stop()

    select {
    case id := <-rulesStore.touchCh:
        if id != 5 {
            t.Errorf("TouchRuleLastRun id = %d, want 5", id)
        }
    case <-time.After(3 * time.Second):
        t.Fatal("timed out waiting for TouchRuleLastRun")
    }
}

func TestScheduler_StopRule_StopsFurtherEvaluation(t *testing.T) {
    evaluator := &fakeEvaluator{}
    raiser := newFakeRaiser()
    rulesStore := newFakeRulesStore()
    s := NewScheduler(rulesStore, map[string]Evaluator{"absence": evaluator}, raiser, schedulerTestLogger())

    s.StartRule(context.Background(), store.Rule{ID: 9, Shape: "absence", IntervalSec: 1})

    select {
    case <-rulesStore.touchCh:
    case <-time.After(3 * time.Second):
        t.Fatal("timed out waiting for first evaluation")
    }

    s.StopRule(9)
    countAfterStop := evaluator.callCount()

    time.Sleep(1500 * time.Millisecond)
    if evaluator.callCount() > countAfterStop {
        t.Errorf("evaluator called again after StopRule: count went from %d to %d", countAfterStop, evaluator.callCount())
    }
}

func TestScheduler_Start_LoadsEnabledRulesFromStore(t *testing.T) {
    evaluator := &fakeEvaluator{}
    raiser := newFakeRaiser()
    rulesStore := newFakeRulesStore(
        store.Rule{ID: 1, Shape: "absence", IntervalSec: 1},
        store.Rule{ID: 2, Shape: "absence", IntervalSec: 1},
    )
    s := NewScheduler(rulesStore, map[string]Evaluator{"absence": evaluator}, raiser, schedulerTestLogger())

    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()
    if err := s.Start(ctx); err != nil {
        t.Fatalf("Start() error = %v", err)
    }
    defer s.Stop()

    seen := map[int64]bool{}
    deadline := time.After(3 * time.Second)
    for len(seen) < 2 {
        select {
        case id := <-rulesStore.touchCh:
            seen[id] = true
        case <-deadline:
            t.Fatalf("timed out waiting for both rules to evaluate, saw %v", seen)
        }
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd siem-api && go test ./internal/rules/... -run Scheduler -v`
Expected: FAIL — `Scheduler`/`NewScheduler`/etc. undefined.

- [ ] **Step 3: Write the implementation**

`siem-api/internal/rules/scheduler.go`:
```go
package rules

import (
    "context"
    "log/slog"
    "math/rand"
    "sync"
    "time"

    "github.com/hibikipr/homeSIEM/siem-api/internal/alerts"
    "github.com/hibikipr/homeSIEM/siem-api/internal/store"
)

type RulesStore interface {
    ListEnabledRules(ctx context.Context) ([]store.Rule, error)
    TouchRuleLastRun(ctx context.Context, id int64, at time.Time) error
}

type Raiser interface {
    Raise(ctx context.Context, c alerts.Candidate) error
}

type Scheduler struct {
    rulesStore RulesStore
    evaluators map[string]Evaluator
    raiser     Raiser
    logger     *slog.Logger

    mu      sync.Mutex
    cancels map[int64]context.CancelFunc
    wg      sync.WaitGroup
}

func NewScheduler(rulesStore RulesStore, evaluators map[string]Evaluator, raiser Raiser, logger *slog.Logger) *Scheduler {
    return &Scheduler{
        rulesStore: rulesStore,
        evaluators: evaluators,
        raiser:     raiser,
        logger:     logger,
        cancels:    make(map[int64]context.CancelFunc),
    }
}

func (s *Scheduler) Start(ctx context.Context) error {
    rules, err := s.rulesStore.ListEnabledRules(ctx)
    if err != nil {
        return err
    }
    for _, rule := range rules {
        s.StartRule(ctx, rule)
    }
    return nil
}

func (s *Scheduler) StartRule(ctx context.Context, rule store.Rule) {
    s.StopRule(rule.ID) // replace any existing goroutine for this rule

    ruleCtx, cancel := context.WithCancel(ctx)

    s.mu.Lock()
    s.cancels[rule.ID] = cancel
    s.mu.Unlock()

    s.wg.Add(1)
    go func() {
        defer s.wg.Done()
        s.runRuleLoop(ruleCtx, rule)
    }()
}

func (s *Scheduler) StopRule(ruleID int64) {
    s.mu.Lock()
    cancel, ok := s.cancels[ruleID]
    delete(s.cancels, ruleID)
    s.mu.Unlock()

    if ok {
        cancel()
    }
}

func (s *Scheduler) Stop() {
    s.mu.Lock()
    for id, cancel := range s.cancels {
        cancel()
        delete(s.cancels, id)
    }
    s.mu.Unlock()

    s.wg.Wait()
}

func (s *Scheduler) runRuleLoop(ctx context.Context, rule store.Rule) {
    intervalSec := rule.IntervalSec
    if intervalSec <= 0 {
        intervalSec = 60
    }
    interval := time.Duration(intervalSec) * time.Second

    jitter := time.Duration(rand.Int63n(int64(interval) + 1))
    select {
    case <-ctx.Done():
        return
    case <-time.After(jitter):
    }

    ticker := time.NewTicker(interval)
    defer ticker.Stop()

    s.evaluateOnce(ctx, rule)
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            s.evaluateOnce(ctx, rule)
        }
    }
}

func (s *Scheduler) evaluateOnce(ctx context.Context, rule store.Rule) {
    evaluator, ok := s.evaluators[rule.Shape]
    if !ok {
        s.logger.Error("no evaluator registered for rule shape", "rule", rule.Name, "shape", rule.Shape)
        return
    }

    candidates, err := evaluator.Evaluate(ctx, rule)
    if err != nil {
        s.logger.Error("rule evaluation failed", "rule", rule.Name, "error", err)
        return
    }

    for _, c := range candidates {
        if err := s.raiser.Raise(ctx, c); err != nil {
            s.logger.Error("raise failed", "rule", rule.Name, "error", err)
        }
    }

    if err := s.rulesStore.TouchRuleLastRun(ctx, rule.ID, time.Now().UTC()); err != nil {
        s.logger.Error("touch rule last run failed", "rule", rule.Name, "error", err)
    }
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd siem-api && go test ./internal/rules/... -v`
Expected: PASS (all `internal/rules` tests from Tasks 18-21).

- [ ] **Step 5: Commit**

```bash
git add siem-api/internal/rules/scheduler.go siem-api/internal/rules/scheduler_test.go
git commit -m "Add siem-api rules: staggered per-rule scheduler"
```

---

### Task 22: `internal/api` — server scaffold, middleware chain, `/healthz`

**Files:**
- Create: `siem-api/internal/api/server.go`
- Create: `siem-api/internal/api/middleware.go`
- Create: `siem-api/internal/api/healthz.go`
- Test: `siem-api/internal/api/server_test.go`
- Test: `siem-api/internal/api/middleware_test.go`

**Interfaces:**
- Consumes: `auth.TokenVerifier`/`auth.Middleware`/`auth.RequireRole` (Tasks 10-11), `auth.SessionEstablisher`/`auth.LocalAuthenticator` (Task 12), `loki.Client` (Task 13), `sse.Hub` (Task 16), `alerts.Service` (Task 17), `rules.Scheduler` (Task 21), `store.Store` (Tasks 3-9).
- Produces: `Deps`, `Server`, `NewServer`, `(*Server).Handler`, `recoverMiddleware`, `logMiddleware`, `protect` — the scaffold every handler task (23-28) registers routes into via `s.routes()` and wraps with via `protect`.

```go
type Deps struct {
    Store         *store.Store
    Loki          *loki.Client
    JobLabel      string
    Hub           *sse.Hub
    Alerts        *alerts.Service
    Scheduler     *rules.Scheduler
    Verifier      *auth.TokenVerifier
    SessionEst    *auth.SessionEstablisher
    LocalAuth     *auth.LocalAuthenticator
    FastpathToken string
    Logger        *slog.Logger
}
type Server struct{ /* unexported */ }
func NewServer(deps Deps) *Server
func (s *Server) Handler() http.Handler
```

`Handler()` wraps the mux in `recoverMiddleware` then `logMiddleware` (outermost to
innermost: recover, then log, then routing) — panics are caught before anything else, and
every request gets logged including ones that panicked. Per-route auth is applied at
registration time via a `protect(minRole, handlerFunc)` helper (added in this task, used by
every protected handler task) that composes `auth.Middleware(deps.Verifier, deps.Store)`
with `auth.RequireRole(minRole, ...)` — `*store.Store` satisfies `auth.RoleResolver` via its
Task 8 `ResolveRole` method. `/healthz` and `/ingest/fastpath` (Task 23) are the only
unprotected routes; `/ingest/fastpath` uses its own shared-token check instead.

- [ ] **Step 1: Write the failing middleware test**

`siem-api/internal/api/middleware_test.go`:
```go
package api

import (
    "io"
    "log/slog"
    "net/http"
    "net/http/httptest"
    "testing"
)

func apiTestLogger() *slog.Logger {
    return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestRecoverMiddleware_CatchesPanic(t *testing.T) {
    panicky := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        panic("boom")
    })
    handler := recoverMiddleware(apiTestLogger())(panicky)

    req := httptest.NewRequest(http.MethodGet, "/anything", nil)
    rec := httptest.NewRecorder()
    handler.ServeHTTP(rec, req)

    if rec.Code != http.StatusInternalServerError {
        t.Fatalf("status = %d, want 500", rec.Code)
    }
}

func TestLogMiddleware_PassesThroughResponse(t *testing.T) {
    ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusTeapot)
    })
    handler := logMiddleware(apiTestLogger())(ok)

    req := httptest.NewRequest(http.MethodGet, "/anything", nil)
    rec := httptest.NewRecorder()
    handler.ServeHTTP(rec, req)

    if rec.Code != http.StatusTeapot {
        t.Fatalf("status = %d, want 418", rec.Code)
    }
}
```

- [ ] **Step 2: Write the failing server test**

`siem-api/internal/api/server_test.go`:
```go
package api

import (
    "net/http"
    "net/http/httptest"
    "testing"
)

func TestHealthz_ReturnsOK(t *testing.T) {
    s := NewServer(Deps{Logger: apiTestLogger()})

    req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
    rec := httptest.NewRecorder()
    s.Handler().ServeHTTP(rec, req)

    if rec.Code != http.StatusOK {
        t.Fatalf("status = %d, want 200", rec.Code)
    }
    if rec.Body.String() != "ok" {
        t.Errorf("body = %q, want ok", rec.Body.String())
    }
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `cd siem-api && go test ./internal/api/... -v`
Expected: FAIL — package undefined.

- [ ] **Step 4: Write the implementation**

`siem-api/internal/api/middleware.go`:
```go
package api

import (
    "log/slog"
    "net/http"
    "time"

    "github.com/hibikipr/homeSIEM/siem-api/internal/auth"
)

func recoverMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            defer func() {
                if rec := recover(); rec != nil {
                    logger.Error("panic recovered", "error", rec, "path", r.URL.Path)
                    http.Error(w, "internal error", http.StatusInternalServerError)
                }
            }()
            next.ServeHTTP(w, r)
        })
    }
}

type statusRecorder struct {
    http.ResponseWriter
    status int
}

func (r *statusRecorder) WriteHeader(code int) {
    r.status = code
    r.ResponseWriter.WriteHeader(code)
}

func logMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            start := time.Now()
            rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
            next.ServeHTTP(rec, r)
            logger.Info("request", "method", r.Method, "path", r.URL.Path,
                "status", rec.status, "duration_ms", time.Since(start).Milliseconds())
        })
    }
}

// protect composes token verification, per-request role resolution, and a
// minimum-role gate — the standard wrapper every protected route (Tasks 23-28,
// except /healthz and /ingest/fastpath) registers through.
func protect(verifier *auth.TokenVerifier, resolver auth.RoleResolver, minRole string, next http.Handler) http.Handler {
    return auth.Middleware(verifier, resolver)(auth.RequireRole(minRole, next))
}
```

`siem-api/internal/api/server.go`:
```go
package api

import (
    "log/slog"
    "net/http"

    "github.com/hibikipr/homeSIEM/siem-api/internal/alerts"
    "github.com/hibikipr/homeSIEM/siem-api/internal/auth"
    "github.com/hibikipr/homeSIEM/siem-api/internal/loki"
    "github.com/hibikipr/homeSIEM/siem-api/internal/rules"
    "github.com/hibikipr/homeSIEM/siem-api/internal/sse"
    "github.com/hibikipr/homeSIEM/siem-api/internal/store"
)

type Deps struct {
    Store         *store.Store
    Loki          *loki.Client
    JobLabel      string
    Hub           *sse.Hub
    Alerts        *alerts.Service
    Scheduler     *rules.Scheduler
    Verifier      *auth.TokenVerifier
    SessionEst    *auth.SessionEstablisher
    LocalAuth     *auth.LocalAuthenticator
    FastpathToken string
    Logger        *slog.Logger
}

type Server struct {
    mux  *http.ServeMux
    deps Deps
}

func NewServer(deps Deps) *Server {
    s := &Server{mux: http.NewServeMux(), deps: deps}
    s.routes()
    return s
}

func (s *Server) Handler() http.Handler {
    return recoverMiddleware(s.deps.Logger)(logMiddleware(s.deps.Logger)(s.mux))
}

func (s *Server) routes() {
    s.mux.HandleFunc("GET /healthz", s.handleHealthz)
}
```

`siem-api/internal/api/healthz.go`:
```go
package api

import "net/http"

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
    w.WriteHeader(http.StatusOK)
    w.Write([]byte("ok"))
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd siem-api && go test ./internal/api/... -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add siem-api/internal/api/server.go siem-api/internal/api/middleware.go siem-api/internal/api/healthz.go siem-api/internal/api/server_test.go siem-api/internal/api/middleware_test.go
git commit -m "Add siem-api api: server scaffold, middleware chain, healthz"
```

---

### Task 23: `internal/api` — `POST /ingest/fastpath`

**Files:**
- Modify: `siem-api/internal/store/rules.go` (add `GetRuleByName`)
- Modify: `siem-api/internal/api/server.go` (register the route)
- Create: `siem-api/internal/api/fastpath.go`
- Create: `siem-api/internal/api/testhelpers_test.go` (shared by every `internal/api` test file from here through Task 28)
- Test: `siem-api/internal/api/fastpath_test.go`

**Interfaces:**
- Consumes: `alerts.Candidate`/`alerts.Sample`/`alerts.Service.Raise` (Task 17), `store.Rule` (Task 6).
- Produces: `store.GetRuleByName` (a small addition beyond Task 6, needed only here), the `/ingest/fastpath` handler, and the `newTestServer`/`authToken` test helpers Tasks 24-28 build on.

```go
func (s *Store) GetRuleByName(ctx context.Context, name string) (*Rule, error) // nil, nil if not found
```

Vector's `fast_path` sink (per `vector.toml` in the handoff bundle) posts the full enriched
event as JSON to this endpoint whenever `threat_intel` is set or it's a dropped connection
with a port. The handler does **in-process field checks only** (per the design spec — no
LogQL re-evaluation on this path) against two well-known, operator-configured rule names:
`threat-intel-hit` and `wan-drop`. If a rule with that name doesn't exist yet or is
disabled, the check is skipped silently — an operator who hasn't created the rule simply
doesn't get fastpath alerts for it yet, which is not an error condition. Auth is a static
shared token (`X-Fastpath-Token` header, compared to `deps.FastpathToken`), not the OIDC/JWT
path, since Vector isn't an OIDC client.

- [ ] **Step 1: Write the shared test helper and the failing test**

`siem-api/internal/api/testhelpers_test.go` — shared across every `internal/api` test file from
this task onward, including a token minter for the handler tasks that follow (24-28), which
all need to call routes behind `protect`:
```go
package api

import (
    "path/filepath"
    "testing"
    "time"

    "github.com/golang-jwt/jwt/v5"
    "github.com/hibikipr/homeSIEM/siem-api/internal/alerts"
    "github.com/hibikipr/homeSIEM/siem-api/internal/auth"
    "github.com/hibikipr/homeSIEM/siem-api/internal/sse"
    "github.com/hibikipr/homeSIEM/siem-api/internal/store"
)

const testSessionSecret = "0123456789abcdef0123456789abcdef"

// newTestServer builds a Server backed by a real temp-dir SQLite store (fast
// enough that faking it isn't worth it — see the design spec's testing
// section) and a real sse.Hub, with a nil ntfy notifier and nil Loki client;
// tasks that need Loki (24) or the scheduler (26) set those Deps fields
// themselves after calling this.
func newTestServer(t *testing.T) (*Server, *store.Store) {
    t.Helper()
    dbPath := filepath.Join(t.TempDir(), "siem.db")
    db, err := store.Open("sqlite://" + dbPath)
    if err != nil {
        t.Fatalf("Open() error = %v", err)
    }
    t.Cleanup(func() { db.Close() })
    if err := store.Migrate(db); err != nil {
        t.Fatalf("Migrate() error = %v", err)
    }
    st := store.New(db)
    hub := sse.NewHub()
    alertsSvc := alerts.NewService(st, hub, nil, apiTestLogger())

    deps := Deps{
        Store:         st,
        Hub:           hub,
        Alerts:        alertsSvc,
        Verifier:      auth.NewTokenVerifier([]byte(testSessionSecret)),
        FastpathToken: "test-fastpath-token",
        JobLabel:      "siem",
        Logger:        apiTestLogger(),
    }
    return NewServer(deps), st
}

// authToken mints a token newTestServer's Verifier will accept, for a caller
// whose groups map to role via a role_mappings row this helper creates.
func authToken(t *testing.T, st *store.Store, role string, priority int) string {
    t.Helper()
    group := "test-group-" + role
    if _, err := st.UpsertRoleMapping(context.Background(), store.RoleMapping{
        GroupClaim: group, Role: role, Priority: priority,
    }); err != nil {
        t.Fatalf("UpsertRoleMapping() error = %v", err)
    }

    claims := struct {
        jwt.RegisteredClaims
        UserID int64    `json:"user_id"`
        Groups []string `json:"groups"`
    }{
        RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))},
        UserID:            1,
        Groups:            []string{group},
    }
    token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(testSessionSecret))
    if err != nil {
        t.Fatalf("SignedString() error = %v", err)
    }
    return token
}
```

Add `"context"` to that import block (used by `authToken`):
```go
import (
    "context"
    "path/filepath"
    "testing"
    "time"

    "github.com/golang-jwt/jwt/v5"
    "github.com/hibikipr/homeSIEM/siem-api/internal/alerts"
    "github.com/hibikipr/homeSIEM/siem-api/internal/auth"
    "github.com/hibikipr/homeSIEM/siem-api/internal/sse"
    "github.com/hibikipr/homeSIEM/siem-api/internal/store"
)
```

`siem-api/internal/api/fastpath_test.go`:
```go
package api

import (
    "bytes"
    "context"
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/hibikipr/homeSIEM/siem-api/internal/store"
)

func TestFastpath_MissingToken(t *testing.T) {
    s, _ := newTestServer(t)

    req := httptest.NewRequest(http.MethodPost, "/ingest/fastpath", bytes.NewReader([]byte(`{}`)))
    rec := httptest.NewRecorder()
    s.Handler().ServeHTTP(rec, req)

    if rec.Code != http.StatusUnauthorized {
        t.Fatalf("status = %d, want 401", rec.Code)
    }
}

func TestFastpath_WanDrop_RaisesAlertForExistingRule(t *testing.T) {
    s, st := newTestServer(t)
    ctx := context.Background()

    if _, err := st.CreateRule(ctx, store.Rule{
        Name: "wan-drop", Shape: "threshold", Severity: "warning",
        Destinations: []string{"inapp"}, CooldownSec: 3600, IntervalSec: 60, Enabled: true,
    }, nil); err != nil {
        t.Fatalf("CreateRule() error = %v", err)
    }

    body := `{"src_ip":"10.0.0.5","dst_ip":"1.2.3.4","dst_port":22,"action":"drop","message":"drop line"}`
    req := httptest.NewRequest(http.MethodPost, "/ingest/fastpath", bytes.NewReader([]byte(body)))
    req.Header.Set("X-Fastpath-Token", "test-fastpath-token")
    rec := httptest.NewRecorder()
    s.Handler().ServeHTTP(rec, req)

    if rec.Code != http.StatusAccepted {
        t.Fatalf("status = %d, want 202, body=%s", rec.Code, rec.Body.String())
    }

    alertsList, err := st.ListAlerts(ctx, "open")
    if err != nil {
        t.Fatalf("ListAlerts() error = %v", err)
    }
    if len(alertsList) != 1 || alertsList[0].GroupKey != "10.0.0.5" {
        t.Fatalf("alerts = %+v, want one open alert for 10.0.0.5", alertsList)
    }
}

func TestFastpath_UnconfiguredRuleSkippedSilently(t *testing.T) {
    s, st := newTestServer(t)
    ctx := context.Background()

    // No "wan-drop" rule created.
    body := `{"src_ip":"10.0.0.5","dst_ip":"1.2.3.4","dst_port":22,"action":"drop","message":"drop line"}`
    req := httptest.NewRequest(http.MethodPost, "/ingest/fastpath", bytes.NewReader([]byte(body)))
    req.Header.Set("X-Fastpath-Token", "test-fastpath-token")
    rec := httptest.NewRecorder()
    s.Handler().ServeHTTP(rec, req)

    if rec.Code != http.StatusAccepted {
        t.Fatalf("status = %d, want 202 even when no matching rule exists", rec.Code)
    }
    alertsList, err := st.ListAlerts(ctx, "")
    if err != nil {
        t.Fatalf("ListAlerts() error = %v", err)
    }
    if len(alertsList) != 0 {
        t.Fatalf("alerts = %+v, want none", alertsList)
    }
}

func TestFastpath_ThreatIntelHit_RaisesAlert(t *testing.T) {
    s, st := newTestServer(t)
    ctx := context.Background()

    if _, err := st.CreateRule(ctx, store.Rule{
        Name: "threat-intel-hit", Shape: "threshold", Severity: "critical",
        Destinations: []string{"inapp"}, CooldownSec: 3600, IntervalSec: 60, Enabled: true,
    }, nil); err != nil {
        t.Fatalf("CreateRule() error = %v", err)
    }

    body := `{"src_ip":"203.0.113.9","threat_intel":"known-scanner","message":"threat line"}`
    req := httptest.NewRequest(http.MethodPost, "/ingest/fastpath", bytes.NewReader([]byte(body)))
    req.Header.Set("X-Fastpath-Token", "test-fastpath-token")
    rec := httptest.NewRecorder()
    s.Handler().ServeHTTP(rec, req)

    if rec.Code != http.StatusAccepted {
        t.Fatalf("status = %d, want 202", rec.Code)
    }
    alertsList, err := st.ListAlerts(ctx, "open")
    if err != nil {
        t.Fatalf("ListAlerts() error = %v", err)
    }
    if len(alertsList) != 1 || alertsList[0].GroupKey != "203.0.113.9" {
        t.Fatalf("alerts = %+v, want one open alert for 203.0.113.9", alertsList)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd siem-api && go test ./internal/api/... -run Fastpath -v`
Expected: FAIL — route not registered / handler undefined.

- [ ] **Step 3: Add `GetRuleByName` to `store`**

Append to `siem-api/internal/store/rules.go`:
```go
func (s *Store) GetRuleByName(ctx context.Context, name string) (*Rule, error) {
    row := s.db.QueryRowContext(ctx, ruleSelect+` WHERE name = ?`, name)
    r, err := scanRule(row)
    if errors.Is(err, sql.ErrNoRows) {
        return nil, nil
    }
    if err != nil {
        return nil, err
    }
    return &r, nil
}
```

- [ ] **Step 4: Register the route**

In `siem-api/internal/api/server.go`, add to `routes()`:
```go
func (s *Server) routes() {
    s.mux.HandleFunc("GET /healthz", s.handleHealthz)
    s.mux.HandleFunc("POST /ingest/fastpath", s.handleFastpath)
}
```

- [ ] **Step 5: Write the handler**

`siem-api/internal/api/fastpath.go`:
```go
package api

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "time"

    "github.com/hibikipr/homeSIEM/siem-api/internal/alerts"
)

type fastpathEvent struct {
    SrcIP       string  `json:"src_ip"`
    DstIP       string  `json:"dst_ip"`
    DstPort     *int    `json:"dst_port"`
    Action      string  `json:"action"`
    Message     string  `json:"message"`
    ThreatIntel *string `json:"threat_intel"`
}

func (s *Server) handleFastpath(w http.ResponseWriter, r *http.Request) {
    if r.Header.Get("X-Fastpath-Token") != s.deps.FastpathToken || s.deps.FastpathToken == "" {
        http.Error(w, "invalid fastpath token", http.StatusUnauthorized)
        return
    }

    var ev fastpathEvent
    if err := json.NewDecoder(r.Body).Decode(&ev); err != nil {
        http.Error(w, "invalid json body", http.StatusBadRequest)
        return
    }

    ctx := r.Context()
    now := time.Now().UTC()

    if ev.ThreatIntel != nil && *ev.ThreatIntel != "" {
        s.raiseFastpathCandidate(ctx, "threat-intel-hit", ev.SrcIP,
            fmt.Sprintf("Threat intel hit: %s", ev.SrcIP),
            fmt.Sprintf("%s tagged %q by threat intel feed", ev.SrcIP, *ev.ThreatIntel),
            ev.Message, now)
    }

    if ev.Action == "drop" && ev.DstPort != nil {
        s.raiseFastpathCandidate(ctx, "wan-drop", ev.SrcIP,
            fmt.Sprintf("Dropped connection from %s", ev.SrcIP),
            fmt.Sprintf("%s -> %s:%d dropped at the gateway", ev.SrcIP, ev.DstIP, *ev.DstPort),
            ev.Message, now)
    }

    w.WriteHeader(http.StatusAccepted)
}

func (s *Server) raiseFastpathCandidate(ctx context.Context, ruleName, groupKey, title, body, line string, now time.Time) {
    rule, err := s.deps.Store.GetRuleByName(ctx, ruleName)
    if err != nil {
        s.deps.Logger.Error("fastpath: lookup rule failed", "rule", ruleName, "error", err)
        return
    }
    if rule == nil || !rule.Enabled {
        return
    }

    if err := s.deps.Alerts.Raise(ctx, alerts.Candidate{
        RuleID: rule.ID, GroupKey: groupKey, Severity: rule.Severity, Title: title, Body: body,
        Samples: []alerts.Sample{{TS: now, Line: line}},
    }); err != nil {
        s.deps.Logger.Error("fastpath: raise failed", "rule", ruleName, "error", err)
    }
}
```

- [ ] **Step 6: Run test to verify it passes**

Run: `cd siem-api && go test ./internal/api/... -v`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add siem-api/internal/store/rules.go siem-api/internal/api/server.go siem-api/internal/api/fastpath.go siem-api/internal/api/fastpath_test.go siem-api/internal/api/testhelpers_test.go
git commit -m "Add siem-api api: POST /ingest/fastpath"
```

---

### Task 24: `internal/api` — `GET /events/search`, `GET /events/tail`, tail poller

**Files:**
- Modify: `siem-api/internal/api/server.go` (register both routes)
- Create: `siem-api/internal/api/events.go`
- Create: `siem-api/internal/api/tail_poller.go`
- Test: `siem-api/internal/api/events_test.go`
- Test: `siem-api/internal/api/tail_poller_test.go`

**Interfaces:**
- Consumes: `loki.Client.QueryRange`/`loki.BuildQuery`/`loki.Filters`/`loki.LogEntry` (Tasks 13-14), `sse.Hub` (Task 16), `auth.Middleware`/`RequireRole` via `protect` (Task 22).
- Produces: `handleEventsSearch`, `handleEventsTail`, `RunTailPoller`, consumed by Task 29 (`main.go` starts `RunTailPoller` as a background goroutine alongside the HTTP server).

```go
func RunTailPoller(ctx context.Context, querier TailQuerier, jobLabel string, hub *sse.Hub, interval time.Duration, logger *slog.Logger)
```

`/events/search` compiles a `loki.Filters` from query params (`source`, `host`, `program`,
`severity`, `facility`, `q` for free text), defaults the range to the last 24h if
`start`/`end` aren't given (RFC3339), and returns `{logql, count, entries}` — the compiled
query is always in the response, per the design spec's "always show the compiled query"
requirement. `/events/tail` delegates straight to `Hub.ServeHTTP("tail", ...)` (Task 16) —
per-connection severity filtering is a client-side concern for the console, not built into
this endpoint; the server streams everything published to the `"tail"` topic. `RunTailPoller`
is what publishes to that topic: it polls `QueryRange` on `interval`, advancing a watermark
timestamp so each entry is published exactly once.

- [ ] **Step 1: Write the failing tests**

`siem-api/internal/api/events_test.go`:
```go
package api

import (
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/hibikipr/homeSIEM/siem-api/internal/loki"
)

func TestEventsSearch_ReturnsCompiledQueryAndEntries(t *testing.T) {
    fakeLoki := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        w.Write([]byte(`{"status":"success","data":{"result":[
            {"stream":{"job":"siem","source":"udm-ultra"},"values":[["1700000000000000000","hello"]]}
        ]}}`))
    }))
    defer fakeLoki.Close()

    s, st := newTestServer(t)
    s.deps.Loki = loki.New(fakeLoki.URL, fakeLoki.Client())
    token := authToken(t, st, "viewer", 100)

    req := httptest.NewRequest(http.MethodGet, "/events/search?source=udm-ultra", nil)
    req.Header.Set("Authorization", "Bearer "+token)
    rec := httptest.NewRecorder()
    s.Handler().ServeHTTP(rec, req)

    if rec.Code != http.StatusOK {
        t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
    }

    var resp searchResponse
    if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
        t.Fatalf("json.Unmarshal() error = %v", err)
    }
    if resp.LogQL != `{job="siem",source="udm-ultra"}` {
        t.Errorf("LogQL = %q", resp.LogQL)
    }
    if resp.Count != 1 || len(resp.Entries) != 1 || resp.Entries[0].Line != "hello" {
        t.Errorf("resp = %+v", resp)
    }
}

func TestEventsSearch_RequiresAuth(t *testing.T) {
    s, _ := newTestServer(t)

    req := httptest.NewRequest(http.MethodGet, "/events/search", nil)
    rec := httptest.NewRecorder()
    s.Handler().ServeHTTP(rec, req)

    if rec.Code != http.StatusUnauthorized {
        t.Fatalf("status = %d, want 401", rec.Code)
    }
}

func TestEventsTail_StreamsHubMessages(t *testing.T) {
    s, st := newTestServer(t)
    token := authToken(t, st, "viewer", 100)

    req := httptest.NewRequest(http.MethodGet, "/events/tail", nil)
    req.Header.Set("Authorization", "Bearer "+token)
    rec := httptest.NewRecorder()

    // handleEventsTail blocks until the request context is cancelled, same
    // as Hub.ServeHTTP in Task 16 — reuse that task's pattern.
    ctx, cancel := context.WithCancel(context.Background())
    req = req.WithContext(ctx)

    done := make(chan struct{})
    go func() {
        s.Handler().ServeHTTP(rec, req)
        close(done)
    }()

    for s.deps.Hub.subscriberCount("tail") == 0 {
        time.Sleep(time.Millisecond)
    }
    s.deps.Hub.Publish("tail", []byte(`{"line":"hi"}`))
    time.Sleep(20 * time.Millisecond)
    cancel()

    select {
    case <-done:
    case <-time.After(time.Second):
        t.Fatal("handler did not return after context cancellation")
    }

    if !strings.Contains(rec.Body.String(), `"line":"hi"`) {
        t.Errorf("body = %q, want it to contain the published message", rec.Body.String())
    }
}
```

Add these imports to `events_test.go`'s import block for `TestEventsTail_StreamsHubMessages`:
```go
import (
    "context"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "strings"
    "testing"
    "time"

    "github.com/hibikipr/homeSIEM/siem-api/internal/loki"
)
```

`siem-api/internal/api/tail_poller_test.go`:
```go
package api

import (
    "context"
    "encoding/json"
    "testing"
    "time"

    "github.com/hibikipr/homeSIEM/siem-api/internal/loki"
    "github.com/hibikipr/homeSIEM/siem-api/internal/sse"
)

type fakeTailQuerier struct {
    entries []loki.LogEntry
}

func (f *fakeTailQuerier) QueryRange(ctx context.Context, logql string, start, end time.Time, limit int) (loki.QueryResult, error) {
    var out []loki.LogEntry
    for _, e := range f.entries {
        if e.Timestamp.After(start) && !e.Timestamp.After(end) {
            out = append(out, e)
        }
    }
    return loki.QueryResult{Entries: out}, nil
}

func TestRunTailPoller_PublishesNewEntriesOnce(t *testing.T) {
    now := time.Now().UTC()
    querier := &fakeTailQuerier{entries: []loki.LogEntry{
        {Timestamp: now.Add(10 * time.Millisecond), Line: "first"},
    }}
    hub := sse.NewHub()
    ch, cancel := hub.Subscribe("tail")
    defer cancel()

    ctx, stop := context.WithCancel(context.Background())
    defer stop()
    go RunTailPoller(ctx, querier, "siem", hub, 20*time.Millisecond, apiTestLogger())

    var got struct{ Line string }
    select {
    case msg := <-ch:
        if err := json.Unmarshal(msg, &got); err != nil {
            t.Fatalf("json.Unmarshal() error = %v", err)
        }
    case <-time.After(2 * time.Second):
        t.Fatal("timed out waiting for the tail poller to publish")
    }
    if got.Line != "first" {
        t.Errorf("Line = %q, want first", got.Line)
    }

    // No further entries added — should not receive a duplicate publish.
    select {
    case msg := <-ch:
        t.Fatalf("received an unexpected second publish: %s", msg)
    case <-time.After(100 * time.Millisecond):
    }
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd siem-api && go test ./internal/api/... -run "Events|TailPoller" -v`
Expected: FAIL — handlers/`RunTailPoller` undefined.

- [ ] **Step 3: Register the routes**

In `siem-api/internal/api/server.go`, extend `routes()`:
```go
func (s *Server) routes() {
    s.mux.HandleFunc("GET /healthz", s.handleHealthz)
    s.mux.HandleFunc("POST /ingest/fastpath", s.handleFastpath)
    s.mux.Handle("GET /events/search", protect(s.deps.Verifier, s.deps.Store, "viewer", http.HandlerFunc(s.handleEventsSearch)))
    s.mux.Handle("GET /events/tail", protect(s.deps.Verifier, s.deps.Store, "viewer", http.HandlerFunc(s.handleEventsTail)))
}
```

Add `"net/http"` to `server.go`'s import block if not already present (it already is, from Task 22).

- [ ] **Step 4: Write the handlers**

`siem-api/internal/api/events.go`:
```go
package api

import (
    "encoding/json"
    "net/http"
    "strconv"
    "time"

    "github.com/hibikipr/homeSIEM/siem-api/internal/loki"
)

type searchResponse struct {
    LogQL   string          `json:"logql"`
    Count   int             `json:"count"`
    Entries []loki.LogEntry `json:"entries"`
}

func (s *Server) handleEventsSearch(w http.ResponseWriter, r *http.Request) {
    q := r.URL.Query()
    filters := loki.Filters{
        Source:   q.Get("source"),
        Host:     q.Get("host"),
        Program:  q.Get("program"),
        Severity: q.Get("severity"),
        Facility: q.Get("facility"),
        FreeText: q.Get("q"),
    }
    logql := loki.BuildQuery(s.deps.JobLabel, filters)

    end := time.Now().UTC()
    start := end.Add(-24 * time.Hour)
    if v := q.Get("start"); v != "" {
        if t, err := time.Parse(time.RFC3339, v); err == nil {
            start = t
        }
    }
    if v := q.Get("end"); v != "" {
        if t, err := time.Parse(time.RFC3339, v); err == nil {
            end = t
        }
    }
    limit := 1000
    if v := q.Get("limit"); v != "" {
        if n, err := strconv.Atoi(v); err == nil && n > 0 {
            limit = n
        }
    }

    result, err := s.deps.Loki.QueryRange(r.Context(), logql, start, end, limit)
    if err != nil {
        s.deps.Logger.Error("events search: query failed", "error", err)
        http.Error(w, "query failed", http.StatusBadGateway)
        return
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(searchResponse{LogQL: logql, Count: len(result.Entries), Entries: result.Entries})
}

func (s *Server) handleEventsTail(w http.ResponseWriter, r *http.Request) {
    s.deps.Hub.ServeHTTP("tail", w, r)
}
```

`siem-api/internal/api/tail_poller.go`:
```go
package api

import (
    "context"
    "encoding/json"
    "log/slog"
    "time"

    "github.com/hibikipr/homeSIEM/siem-api/internal/loki"
    "github.com/hibikipr/homeSIEM/siem-api/internal/sse"
)

type TailQuerier interface {
    QueryRange(ctx context.Context, logql string, start, end time.Time, limit int) (loki.QueryResult, error)
}

func RunTailPoller(ctx context.Context, querier TailQuerier, jobLabel string, hub *sse.Hub, interval time.Duration, logger *slog.Logger) {
    watermark := time.Now().UTC()
    logql := loki.BuildQuery(jobLabel, loki.Filters{})

    ticker := time.NewTicker(interval)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            end := time.Now().UTC()
            result, err := querier.QueryRange(ctx, logql, watermark, end, 1000)
            if err != nil {
                logger.Error("tail poller: query failed", "error", err)
                continue
            }

            for _, entry := range result.Entries {
                if !entry.Timestamp.After(watermark) {
                    continue
                }
                payload, err := json.Marshal(entry)
                if err != nil {
                    continue
                }
                hub.Publish("tail", payload)
            }

            if end.After(watermark) {
                watermark = end
            }
        }
    }
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd siem-api && go test ./internal/api/... -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add siem-api/internal/api/server.go siem-api/internal/api/events.go siem-api/internal/api/tail_poller.go siem-api/internal/api/events_test.go siem-api/internal/api/tail_poller_test.go
git commit -m "Add siem-api api: GET /events/search, GET /events/tail, tail poller"
```

---

### Task 25: `internal/api` — `GET /alerts`, `POST /alerts/{id}/ack`, alert stream

**Files:**
- Modify: `siem-api/internal/api/server.go` (register the routes)
- Create: `siem-api/internal/api/alerts.go`
- Test: `siem-api/internal/api/alerts_test.go`

**Interfaces:**
- Consumes: `store.Alert`/`store.Store.ListAlerts`/`AckAlert` (Task 7), `auth.UserFromContext` (Task 11), `sse.Hub` (Task 16).
- Produces: `handleListAlerts`, `handleAckAlert`, `handleAlertsStream`.

**`GET /alerts/stream` is an addition beyond the handoff's listed API surface** — same
category as `/auth/local` and `/auth/session` (Tasks 12/28). The handoff's siem-api
responsibilities list says alert lifecycle events get "push[ed] to the console over SSE,"
but only `/events/tail` is explicitly marked `(SSE)` in the endpoint list; `/alerts/stream`
is the endpoint that push actually needs, delegating to `Hub.ServeHTTP("alerts", ...)` —
the same topic `alerts.Service.Raise` (Task 17) already publishes to.

Requires `analyst`+ to ack (an alert acknowledgement is a state-changing action); `viewer`+
to list or stream.

- [ ] **Step 1: Write the failing test**

`siem-api/internal/api/alerts_test.go`:
```go
package api

import (
    "context"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "strings"
    "testing"
    "time"

    "github.com/hibikipr/homeSIEM/siem-api/internal/store"
)

func TestListAlerts_FiltersByState(t *testing.T) {
    s, st := newTestServer(t)
    ctx := context.Background()

    rule, err := st.CreateRule(ctx, store.Rule{Name: "r", Shape: "absence", Severity: "low",
        Destinations: []string{"inapp"}, CooldownSec: 60, IntervalSec: 60, Enabled: true}, nil)
    if err != nil {
        t.Fatalf("CreateRule() error = %v", err)
    }
    now := time.Now().UTC()
    if _, err := st.InsertAlert(ctx, store.Alert{RuleID: rule.ID, GroupKey: "a", Severity: "low",
        Title: "t", Body: "b", EventCount: 1, Context: "{}", State: "open", FirstSeenAt: now, LastSeenAt: now}); err != nil {
        t.Fatalf("InsertAlert() error = %v", err)
    }
    if _, err := st.InsertAlert(ctx, store.Alert{RuleID: rule.ID, GroupKey: "b", Severity: "low",
        Title: "t", Body: "b", EventCount: 1, Context: "{}", State: "acked", FirstSeenAt: now, LastSeenAt: now}); err != nil {
        t.Fatalf("InsertAlert() error = %v", err)
    }

    token := authToken(t, st, "viewer", 100)
    req := httptest.NewRequest(http.MethodGet, "/alerts?state=open", nil)
    req.Header.Set("Authorization", "Bearer "+token)
    rec := httptest.NewRecorder()
    s.Handler().ServeHTTP(rec, req)

    if rec.Code != http.StatusOK {
        t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
    }
    var got []alertResponse
    if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
        t.Fatalf("json.Unmarshal() error = %v", err)
    }
    if len(got) != 1 || got[0].GroupKey != "a" {
        t.Fatalf("got = %+v, want one alert with GroupKey a", got)
    }
}

func TestAckAlert_ViewerForbidden(t *testing.T) {
    s, st := newTestServer(t)
    ctx := context.Background()
    rule, err := st.CreateRule(ctx, store.Rule{Name: "r", Shape: "absence", Severity: "low",
        Destinations: []string{"inapp"}, CooldownSec: 60, IntervalSec: 60, Enabled: true}, nil)
    if err != nil {
        t.Fatalf("CreateRule() error = %v", err)
    }
    now := time.Now().UTC()
    alert, err := st.InsertAlert(ctx, store.Alert{RuleID: rule.ID, GroupKey: "a", Severity: "low",
        Title: "t", Body: "b", EventCount: 1, Context: "{}", State: "open", FirstSeenAt: now, LastSeenAt: now})
    if err != nil {
        t.Fatalf("InsertAlert() error = %v", err)
    }

    token := authToken(t, st, "viewer", 100)
    req := httptest.NewRequest(http.MethodPost, "/alerts/"+itoa(alert.ID)+"/ack", nil)
    req.Header.Set("Authorization", "Bearer "+token)
    rec := httptest.NewRecorder()
    s.Handler().ServeHTTP(rec, req)

    if rec.Code != http.StatusForbidden {
        t.Fatalf("status = %d, want 403", rec.Code)
    }
}

func TestAckAlert_AnalystSucceedsAndAudits(t *testing.T) {
    s, st := newTestServer(t)
    ctx := context.Background()
    rule, err := st.CreateRule(ctx, store.Rule{Name: "r", Shape: "absence", Severity: "low",
        Destinations: []string{"inapp"}, CooldownSec: 60, IntervalSec: 60, Enabled: true}, nil)
    if err != nil {
        t.Fatalf("CreateRule() error = %v", err)
    }
    now := time.Now().UTC()
    alert, err := st.InsertAlert(ctx, store.Alert{RuleID: rule.ID, GroupKey: "a", Severity: "low",
        Title: "t", Body: "b", EventCount: 1, Context: "{}", State: "open", FirstSeenAt: now, LastSeenAt: now})
    if err != nil {
        t.Fatalf("InsertAlert() error = %v", err)
    }

    token := authToken(t, st, "analyst", 50)
    req := httptest.NewRequest(http.MethodPost, "/alerts/"+itoa(alert.ID)+"/ack", nil)
    req.Header.Set("Authorization", "Bearer "+token)
    rec := httptest.NewRecorder()
    s.Handler().ServeHTTP(rec, req)

    if rec.Code != http.StatusNoContent {
        t.Fatalf("status = %d, want 204, body=%s", rec.Code, rec.Body.String())
    }

    got, err := st.GetAlert(ctx, alert.ID)
    if err != nil {
        t.Fatalf("GetAlert() error = %v", err)
    }
    if got.State != "acked" {
        t.Errorf("State = %q, want acked", got.State)
    }

    entries, err := st.ListAudit(ctx, 10)
    if err != nil {
        t.Fatalf("ListAudit() error = %v", err)
    }
    found := false
    for _, e := range entries {
        if e.Action == "alert.ack" {
            found = true
        }
    }
    if !found {
        t.Error("no alert.ack audit entry found")
    }
}

func TestAlertsStream_PublishesFromHub(t *testing.T) {
    s, st := newTestServer(t)
    token := authToken(t, st, "viewer", 100)

    req := httptest.NewRequest(http.MethodGet, "/alerts/stream", nil)
    req.Header.Set("Authorization", "Bearer "+token)
    ctx, cancel := context.WithCancel(context.Background())
    req = req.WithContext(ctx)
    rec := httptest.NewRecorder()

    done := make(chan struct{})
    go func() {
        s.Handler().ServeHTTP(rec, req)
        close(done)
    }()

    for s.deps.Hub.subscriberCount("alerts") == 0 {
        time.Sleep(time.Millisecond)
    }
    s.deps.Hub.Publish("alerts", []byte(`{"id":1}`))
    time.Sleep(20 * time.Millisecond)
    cancel()

    select {
    case <-done:
    case <-time.After(time.Second):
        t.Fatal("handler did not return after context cancellation")
    }
    if !strings.Contains(rec.Body.String(), `"id":1`) {
        t.Errorf("body = %q, want it to contain the published alert", rec.Body.String())
    }
}

func itoa(n int64) string {
    if n == 0 {
        return "0"
    }
    var buf [20]byte
    i := len(buf)
    for n > 0 {
        i--
        buf[i] = byte('0' + n%10)
        n /= 10
    }
    return string(buf[i:])
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd siem-api && go test ./internal/api/... -run Alert -v`
Expected: FAIL — handlers/`alertResponse` undefined.

- [ ] **Step 3: Register the routes**

In `siem-api/internal/api/server.go`, extend `routes()`:
```go
    s.mux.Handle("GET /alerts", protect(s.deps.Verifier, s.deps.Store, "viewer", http.HandlerFunc(s.handleListAlerts)))
    s.mux.Handle("POST /alerts/{id}/ack", protect(s.deps.Verifier, s.deps.Store, "analyst", http.HandlerFunc(s.handleAckAlert)))
    s.mux.Handle("GET /alerts/stream", protect(s.deps.Verifier, s.deps.Store, "viewer", http.HandlerFunc(s.handleAlertsStream)))
```

- [ ] **Step 4: Write the handlers**

`siem-api/internal/api/alerts.go`:
```go
package api

import (
    "encoding/json"
    "net/http"
    "strconv"
    "time"

    "github.com/hibikipr/homeSIEM/siem-api/internal/auth"
    "github.com/hibikipr/homeSIEM/siem-api/internal/store"
)

type alertResponse struct {
    ID          int64      `json:"id"`
    RuleID      int64      `json:"rule_id"`
    GroupKey    string     `json:"group_key"`
    Severity    string     `json:"severity"`
    Title       string     `json:"title"`
    Body        string     `json:"body"`
    EventCount  int        `json:"event_count"`
    State       string     `json:"state"`
    FirstSeenAt time.Time  `json:"first_seen_at"`
    LastSeenAt  time.Time  `json:"last_seen_at"`
    AckedBy     *int64     `json:"acked_by,omitempty"`
    AckedAt     *time.Time `json:"acked_at,omitempty"`
}

func toAlertResponse(a store.Alert) alertResponse {
    return alertResponse{
        ID: a.ID, RuleID: a.RuleID, GroupKey: a.GroupKey, Severity: a.Severity,
        Title: a.Title, Body: a.Body, EventCount: a.EventCount, State: a.State,
        FirstSeenAt: a.FirstSeenAt, LastSeenAt: a.LastSeenAt, AckedBy: a.AckedBy, AckedAt: a.AckedAt,
    }
}

func (s *Server) handleListAlerts(w http.ResponseWriter, r *http.Request) {
    alertsList, err := s.deps.Store.ListAlerts(r.Context(), r.URL.Query().Get("state"))
    if err != nil {
        s.deps.Logger.Error("list alerts failed", "error", err)
        http.Error(w, "list alerts failed", http.StatusInternalServerError)
        return
    }

    resp := make([]alertResponse, len(alertsList))
    for i, a := range alertsList {
        resp[i] = toAlertResponse(a)
    }
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleAckAlert(w http.ResponseWriter, r *http.Request) {
    id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
    if err != nil {
        http.Error(w, "invalid alert id", http.StatusBadRequest)
        return
    }

    userID, _, ok := auth.UserFromContext(r.Context())
    if !ok {
        http.Error(w, "unauthenticated", http.StatusUnauthorized)
        return
    }

    if err := s.deps.Store.AckAlert(r.Context(), id, userID, time.Now().UTC()); err != nil {
        s.deps.Logger.Error("ack alert failed", "alert_id", id, "error", err)
        http.Error(w, "ack failed", http.StatusInternalServerError)
        return
    }
    w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAlertsStream(w http.ResponseWriter, r *http.Request) {
    s.deps.Hub.ServeHTTP("alerts", w, r)
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `cd siem-api && go test ./internal/api/... -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add siem-api/internal/api/server.go siem-api/internal/api/alerts.go siem-api/internal/api/alerts_test.go
git commit -m "Add siem-api api: GET /alerts, POST /alerts/{id}/ack, alert stream"
```

---

### Task 26: `internal/api` — `/rules` CRUD wired to the scheduler

**Files:**
- Modify: `siem-api/internal/api/server.go` (register routes; add `SchedulerCtx` to `Deps`)
- Create: `siem-api/internal/api/rules.go`
- Test: `siem-api/internal/api/rules_test.go`

**Interfaces:**
- Consumes: `store.Rule`/`store.Store.CreateRule`/`UpdateRule`/`DeleteRule`/`ListRules` (Task 6), `rules.Scheduler.StartRule`/`StopRule` (Task 21).
- Produces: `handleListRules`, `handleCreateRule`, `handleUpdateRule`, `handleDeleteRule`.

`Deps` gains one field:
```go
SchedulerCtx context.Context
```
A rule create/update/delete over HTTP must not derive the scheduler goroutine it starts
from `r.Context()` — that context is cancelled the moment the HTTP response finishes, which
would kill the rule's evaluation loop immediately. `SchedulerCtx` is a long-lived context
covering the server's whole run (set once in Task 29's `main.go`, cancelled only at
shutdown), used as the parent for every `Scheduler.StartRule` call this package makes.

Requires `analyst`+ to create, update, or delete a rule, per the handoff's role table
("siem-analysts... create and edit rules"); `GET /rules` only requires `viewer`+, matching
the code below — reading rule definitions is as permissive as reading anything else a
viewer can already see (Screen 4's alert detail view shows a fired rule's name and LogQL
to any authenticated role, so gating the rules list itself behind `analyst` would be an
inconsistent, not-actually-protective restriction). Create/update start (or restart) the
rule's scheduler goroutine
when `Enabled`, or stop it when not; delete always stops it.

- [ ] **Step 1: Write the failing test**

`siem-api/internal/api/rules_test.go`:
```go
package api

import (
    "bytes"
    "context"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/hibikipr/homeSIEM/siem-api/internal/rules"
    "github.com/hibikipr/homeSIEM/siem-api/internal/store"
)

// newSchedulerTestServer gives the base test server (Task 23) a real,
// otherwise-idle Scheduler with no registered evaluators — Task 21 already
// covers scheduler evaluation behavior; here we only need CreateRule /
// UpdateRule / DeleteRule to be able to call StartRule / StopRule on
// something real without erroring.
func newSchedulerTestServer(t *testing.T) *Server {
    t.Helper()
    s, st := newTestServer(t)
    s.deps.Scheduler = rules.NewScheduler(st, map[string]rules.Evaluator{}, nil, apiTestLogger())
    s.deps.SchedulerCtx = context.Background()
    return s
}

func TestCreateRule_RequiresAnalyst(t *testing.T) {
    s := newSchedulerTestServer(t)
    token := authToken(t, s.deps.Store, "viewer", 100)

    body := `{"name":"wan-portscan","shape":"threshold","logql":"{job=\"siem\"}","window_sec":60,"threshold":5,"group_by":["src_ip"],"severity":"critical","destinations":["inapp"],"cooldown_sec":3600,"interval_sec":60,"enabled":true}`
    req := httptest.NewRequest(http.MethodPost, "/rules", bytes.NewReader([]byte(body)))
    req.Header.Set("Authorization", "Bearer "+token)
    rec := httptest.NewRecorder()
    s.Handler().ServeHTTP(rec, req)

    if rec.Code != http.StatusForbidden {
        t.Fatalf("status = %d, want 403", rec.Code)
    }
}

func TestCreateAndListRules(t *testing.T) {
    s := newSchedulerTestServer(t)
    token := authToken(t, s.deps.Store, "analyst", 50)

    body := `{"name":"wan-portscan","shape":"threshold","logql":"{job=\"siem\"}","window_sec":60,"threshold":5,"group_by":["src_ip"],"severity":"critical","destinations":["inapp"],"cooldown_sec":3600,"interval_sec":60,"enabled":true}`
    req := httptest.NewRequest(http.MethodPost, "/rules", bytes.NewReader([]byte(body)))
    req.Header.Set("Authorization", "Bearer "+token)
    rec := httptest.NewRecorder()
    s.Handler().ServeHTTP(rec, req)

    if rec.Code != http.StatusCreated {
        t.Fatalf("status = %d, want 201, body=%s", rec.Code, rec.Body.String())
    }
    var created ruleResponse
    if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
        t.Fatalf("json.Unmarshal() error = %v", err)
    }
    if created.ID == 0 || created.Name != "wan-portscan" {
        t.Fatalf("created = %+v", created)
    }

    listReq := httptest.NewRequest(http.MethodGet, "/rules", nil)
    listReq.Header.Set("Authorization", "Bearer "+token)
    listRec := httptest.NewRecorder()
    s.Handler().ServeHTTP(listRec, listReq)

    var list []ruleResponse
    if err := json.Unmarshal(listRec.Body.Bytes(), &list); err != nil {
        t.Fatalf("json.Unmarshal() error = %v", err)
    }
    if len(list) != 1 {
        t.Fatalf("list = %+v, want 1 rule", list)
    }
}

func TestUpdateRule_DisablesStopsScheduler(t *testing.T) {
    s := newSchedulerTestServer(t)
    ctx := context.Background()
    token := authToken(t, s.deps.Store, "analyst", 50)

    created, err := s.deps.Store.CreateRule(ctx, store.Rule{
        Name: "r", Shape: "absence", Severity: "low", Destinations: []string{"inapp"},
        CooldownSec: 60, IntervalSec: 60, Enabled: true,
    }, nil)
    if err != nil {
        t.Fatalf("CreateRule() error = %v", err)
    }

    body := `{"name":"r","shape":"absence","severity":"low","destinations":["inapp"],"cooldown_sec":60,"interval_sec":60,"enabled":false}`
    req := httptest.NewRequest(http.MethodPut, "/rules/"+itoa(created.ID), bytes.NewReader([]byte(body)))
    req.Header.Set("Authorization", "Bearer "+token)
    rec := httptest.NewRecorder()
    s.Handler().ServeHTTP(rec, req)

    if rec.Code != http.StatusOK {
        t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
    }

    got, err := s.deps.Store.GetRule(ctx, created.ID)
    if err != nil {
        t.Fatalf("GetRule() error = %v", err)
    }
    if got.Enabled {
        t.Error("Enabled = true, want false after update")
    }
}

func TestDeleteRule(t *testing.T) {
    s := newSchedulerTestServer(t)
    ctx := context.Background()
    token := authToken(t, s.deps.Store, "analyst", 50)

    created, err := s.deps.Store.CreateRule(ctx, store.Rule{
        Name: "r", Shape: "absence", Severity: "low", Destinations: []string{"inapp"},
        CooldownSec: 60, IntervalSec: 60, Enabled: true,
    }, nil)
    if err != nil {
        t.Fatalf("CreateRule() error = %v", err)
    }

    req := httptest.NewRequest(http.MethodDelete, "/rules/"+itoa(created.ID), nil)
    req.Header.Set("Authorization", "Bearer "+token)
    rec := httptest.NewRecorder()
    s.Handler().ServeHTTP(rec, req)

    if rec.Code != http.StatusNoContent {
        t.Fatalf("status = %d, want 204, body=%s", rec.Code, rec.Body.String())
    }
    if _, err := s.deps.Store.GetRule(ctx, created.ID); err == nil {
        t.Fatal("GetRule() after delete: error = nil, want not found")
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd siem-api && go test ./internal/api/... -run Rule -v`
Expected: FAIL — handlers/`ruleResponse`/`Deps.SchedulerCtx`/`Deps.Scheduler` undefined
(`Scheduler` was already declared on `Deps` in Task 22; only `SchedulerCtx` is new).

- [ ] **Step 3: Register the routes and extend `Deps`**

In `siem-api/internal/api/server.go`, add `SchedulerCtx context.Context` to the `Deps`
struct (add `"context"` to the import block), and extend `routes()`:
```go
    s.mux.Handle("GET /rules", protect(s.deps.Verifier, s.deps.Store, "viewer", http.HandlerFunc(s.handleListRules)))
    s.mux.Handle("POST /rules", protect(s.deps.Verifier, s.deps.Store, "analyst", http.HandlerFunc(s.handleCreateRule)))
    s.mux.Handle("PUT /rules/{id}", protect(s.deps.Verifier, s.deps.Store, "analyst", http.HandlerFunc(s.handleUpdateRule)))
    s.mux.Handle("DELETE /rules/{id}", protect(s.deps.Verifier, s.deps.Store, "analyst", http.HandlerFunc(s.handleDeleteRule)))
```

- [ ] **Step 4: Write the handlers**

`siem-api/internal/api/rules.go`:
```go
package api

import (
    "encoding/json"
    "net/http"
    "strconv"
    "time"

    "github.com/hibikipr/homeSIEM/siem-api/internal/store"
)

type ruleResponse struct {
    ID           int64      `json:"id"`
    Name         string     `json:"name"`
    Shape        string     `json:"shape"`
    LogQL        string     `json:"logql"`
    WindowSec    int        `json:"window_sec"`
    Threshold    *int       `json:"threshold,omitempty"`
    GroupBy      []string   `json:"group_by"`
    Severity     string     `json:"severity"`
    Destinations []string   `json:"destinations"`
    CooldownSec  int        `json:"cooldown_sec"`
    IntervalSec  int        `json:"interval_sec"`
    Enabled      bool       `json:"enabled"`
    LastRunAt    *time.Time `json:"last_run_at,omitempty"`
}

func toRuleResponse(r store.Rule) ruleResponse {
    return ruleResponse{
        ID: r.ID, Name: r.Name, Shape: r.Shape, LogQL: r.LogQL, WindowSec: r.WindowSec,
        Threshold: r.Threshold, GroupBy: r.GroupBy, Severity: r.Severity,
        Destinations: r.Destinations, CooldownSec: r.CooldownSec, IntervalSec: r.IntervalSec,
        Enabled: r.Enabled, LastRunAt: r.LastRunAt,
    }
}

type ruleRequest struct {
    Name         string   `json:"name"`
    Shape        string   `json:"shape"`
    LogQL        string   `json:"logql"`
    WindowSec    int      `json:"window_sec"`
    Threshold    *int     `json:"threshold"`
    GroupBy      []string `json:"group_by"`
    Severity     string   `json:"severity"`
    Destinations []string `json:"destinations"`
    CooldownSec  int      `json:"cooldown_sec"`
    IntervalSec  int      `json:"interval_sec"`
    Enabled      bool     `json:"enabled"`
}

func (rq ruleRequest) toStoreRule() store.Rule {
    return store.Rule{
        Name: rq.Name, Shape: rq.Shape, LogQL: rq.LogQL, WindowSec: rq.WindowSec,
        Threshold: rq.Threshold, GroupBy: rq.GroupBy, Severity: rq.Severity,
        Destinations: rq.Destinations, CooldownSec: rq.CooldownSec, IntervalSec: rq.IntervalSec,
        Enabled: rq.Enabled,
    }
}

func (s *Server) handleListRules(w http.ResponseWriter, r *http.Request) {
    ruleList, err := s.deps.Store.ListRules(r.Context())
    if err != nil {
        http.Error(w, "list rules failed", http.StatusInternalServerError)
        return
    }
    resp := make([]ruleResponse, len(ruleList))
    for i, rl := range ruleList {
        resp[i] = toRuleResponse(rl)
    }
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleCreateRule(w http.ResponseWriter, r *http.Request) {
    var req ruleRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, "invalid json body", http.StatusBadRequest)
        return
    }

    created, err := s.deps.Store.CreateRule(r.Context(), req.toStoreRule(), nil)
    if err != nil {
        s.deps.Logger.Error("create rule failed", "error", err)
        http.Error(w, "create rule failed", http.StatusInternalServerError)
        return
    }

    if created.Enabled && s.deps.Scheduler != nil {
        s.deps.Scheduler.StartRule(s.deps.SchedulerCtx, created)
    }

    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusCreated)
    json.NewEncoder(w).Encode(toRuleResponse(created))
}

func (s *Server) handleUpdateRule(w http.ResponseWriter, r *http.Request) {
    id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
    if err != nil {
        http.Error(w, "invalid rule id", http.StatusBadRequest)
        return
    }

    var req ruleRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, "invalid json body", http.StatusBadRequest)
        return
    }
    ruleToUpdate := req.toStoreRule()
    ruleToUpdate.ID = id

    updated, err := s.deps.Store.UpdateRule(r.Context(), ruleToUpdate, nil)
    if err != nil {
        s.deps.Logger.Error("update rule failed", "rule_id", id, "error", err)
        http.Error(w, "update rule failed", http.StatusInternalServerError)
        return
    }

    if s.deps.Scheduler != nil {
        if updated.Enabled {
            s.deps.Scheduler.StartRule(s.deps.SchedulerCtx, updated)
        } else {
            s.deps.Scheduler.StopRule(updated.ID)
        }
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(toRuleResponse(updated))
}

func (s *Server) handleDeleteRule(w http.ResponseWriter, r *http.Request) {
    id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
    if err != nil {
        http.Error(w, "invalid rule id", http.StatusBadRequest)
        return
    }

    if err := s.deps.Store.DeleteRule(r.Context(), id, nil); err != nil {
        s.deps.Logger.Error("delete rule failed", "rule_id", id, "error", err)
        http.Error(w, "delete rule failed", http.StatusInternalServerError)
        return
    }

    if s.deps.Scheduler != nil {
        s.deps.Scheduler.StopRule(id)
    }
    w.WriteHeader(http.StatusNoContent)
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `cd siem-api && go test ./internal/api/... -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add siem-api/internal/api/server.go siem-api/internal/api/rules.go siem-api/internal/api/rules_test.go
git commit -m "Add siem-api api: /rules CRUD wired to the scheduler"
```

---

### Task 27: `internal/api` — `GET /sources`, `POST /sources/{id}/claim`

**Files:**
- Modify: `siem-api/internal/api/server.go` (register the routes)
- Create: `siem-api/internal/api/sources.go`
- Test: `siem-api/internal/api/sources_test.go`

**Interfaces:**
- Consumes: `store.Source`/`store.Store.ListSources`/`ClaimSource` (Task 5).
- Produces: `handleListSources`, `handleClaimSource`.

`GET /sources` is `viewer`+ (read-only, matches the Sources screen being reachable from the
main nav for any authenticated role). `POST /sources/{id}/claim` is `admin`+ — the handoff's
role table gives analysts search/tail/alerts/rules but reserves "sources, parsers, rules,
retention, auth" management for admins; claiming a sender is source management.

- [ ] **Step 1: Write the failing test**

`siem-api/internal/api/sources_test.go`:
```go
package api

import (
    "context"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/hibikipr/homeSIEM/siem-api/internal/store"
)

func TestListSources(t *testing.T) {
    s, st := newTestServer(t)
    ctx := context.Background()
    if _, err := st.UpsertSource(ctx, store.Source{
        Name: "udm-ultra", Address: "10.0.0.1", Transport: "udp/514", Parser: "unifi-os", HeartbeatSec: 900,
    }); err != nil {
        t.Fatalf("UpsertSource() error = %v", err)
    }

    token := authToken(t, st, "viewer", 100)
    req := httptest.NewRequest(http.MethodGet, "/sources", nil)
    req.Header.Set("Authorization", "Bearer "+token)
    rec := httptest.NewRecorder()
    s.Handler().ServeHTTP(rec, req)

    if rec.Code != http.StatusOK {
        t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
    }
    var got []sourceResponse
    if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
        t.Fatalf("json.Unmarshal() error = %v", err)
    }
    if len(got) != 1 || got[0].Name != "udm-ultra" {
        t.Fatalf("got = %+v", got)
    }
}

func TestClaimSource_RequiresAdmin(t *testing.T) {
    s, st := newTestServer(t)
    ctx := context.Background()
    src, err := st.UpsertSource(ctx, store.Source{
        Name: "unclaimed-host", Address: "10.0.0.2", Transport: "tcp/601", Parser: "rfc5424", HeartbeatSec: 900,
    })
    if err != nil {
        t.Fatalf("UpsertSource() error = %v", err)
    }

    token := authToken(t, st, "analyst", 50)
    req := httptest.NewRequest(http.MethodPost, "/sources/"+itoa(src.ID)+"/claim", nil)
    req.Header.Set("Authorization", "Bearer "+token)
    rec := httptest.NewRecorder()
    s.Handler().ServeHTTP(rec, req)

    if rec.Code != http.StatusForbidden {
        t.Fatalf("status = %d, want 403", rec.Code)
    }
}

func TestClaimSource_AdminSucceeds(t *testing.T) {
    s, st := newTestServer(t)
    ctx := context.Background()
    src, err := st.UpsertSource(ctx, store.Source{
        Name: "unclaimed-host", Address: "10.0.0.2", Transport: "tcp/601", Parser: "rfc5424", HeartbeatSec: 900,
    })
    if err != nil {
        t.Fatalf("UpsertSource() error = %v", err)
    }

    token := authToken(t, st, "admin", 10)
    req := httptest.NewRequest(http.MethodPost, "/sources/"+itoa(src.ID)+"/claim", nil)
    req.Header.Set("Authorization", "Bearer "+token)
    rec := httptest.NewRecorder()
    s.Handler().ServeHTTP(rec, req)

    if rec.Code != http.StatusNoContent {
        t.Fatalf("status = %d, want 204, body=%s", rec.Code, rec.Body.String())
    }

    sources, err := st.ListSources(ctx)
    if err != nil {
        t.Fatalf("ListSources() error = %v", err)
    }
    if !sources[0].Claimed {
        t.Error("Claimed = false after claim")
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd siem-api && go test ./internal/api/... -run Source -v`
Expected: FAIL — handlers/`sourceResponse` undefined.

- [ ] **Step 3: Register the routes**

In `siem-api/internal/api/server.go`, extend `routes()`:
```go
    s.mux.Handle("GET /sources", protect(s.deps.Verifier, s.deps.Store, "viewer", http.HandlerFunc(s.handleListSources)))
    s.mux.Handle("POST /sources/{id}/claim", protect(s.deps.Verifier, s.deps.Store, "admin", http.HandlerFunc(s.handleClaimSource)))
```

- [ ] **Step 4: Write the handlers**

`siem-api/internal/api/sources.go`:
```go
package api

import (
    "encoding/json"
    "net/http"
    "strconv"
    "time"

    "github.com/hibikipr/homeSIEM/siem-api/internal/store"
)

type sourceResponse struct {
    ID           int64      `json:"id"`
    Name         string     `json:"name"`
    Address      string     `json:"address"`
    Transport    string     `json:"transport"`
    Parser       string     `json:"parser"`
    Claimed      bool       `json:"claimed"`
    HeartbeatSec int        `json:"heartbeat_sec"`
    LastSeenAt   *time.Time `json:"last_seen_at,omitempty"`
}

func toSourceResponse(src store.Source) sourceResponse {
    return sourceResponse{
        ID: src.ID, Name: src.Name, Address: src.Address, Transport: src.Transport,
        Parser: src.Parser, Claimed: src.Claimed, HeartbeatSec: src.HeartbeatSec, LastSeenAt: src.LastSeenAt,
    }
}

func (s *Server) handleListSources(w http.ResponseWriter, r *http.Request) {
    sources, err := s.deps.Store.ListSources(r.Context())
    if err != nil {
        http.Error(w, "list sources failed", http.StatusInternalServerError)
        return
    }
    resp := make([]sourceResponse, len(sources))
    for i, src := range sources {
        resp[i] = toSourceResponse(src)
    }
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleClaimSource(w http.ResponseWriter, r *http.Request) {
    id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
    if err != nil {
        http.Error(w, "invalid source id", http.StatusBadRequest)
        return
    }
    if err := s.deps.Store.ClaimSource(r.Context(), id); err != nil {
        s.deps.Logger.Error("claim source failed", "source_id", id, "error", err)
        http.Error(w, "claim source failed", http.StatusInternalServerError)
        return
    }
    w.WriteHeader(http.StatusNoContent)
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `cd siem-api && go test ./internal/api/... -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add siem-api/internal/api/server.go siem-api/internal/api/sources.go siem-api/internal/api/sources_test.go
git commit -m "Add siem-api api: GET /sources, POST /sources/{id}/claim"
```

---

### Task 28: `internal/api` — `/settings/auth`, `POST /auth/session`, `POST /auth/local`

**Files:**
- Modify: `siem-api/internal/api/server.go` (register routes; add `OIDCIssuer`/`OIDCClientID`/`OIDCGroupsScope` to `Deps`)
- Create: `siem-api/internal/api/settings_auth.go`
- Create: `siem-api/internal/api/auth_session.go`
- Test: `siem-api/internal/api/settings_auth_test.go`
- Test: `siem-api/internal/api/auth_session_test.go`

**Interfaces:**
- Consumes: `store.RoleMapping`/`ListRoleMappings`/`UpsertRoleMapping` (Task 8), `auth.SessionEstablisher`/`LocalAuthenticator` (Task 12).
- Produces: `handleGetAuthSettings`, `handleUpdateAuthSettings`, `handleAuthSession`, `handleAuthLocal`.

`GET`/`PUT /settings/auth` are `admin`+: they read/edit `role_mappings` and display the
OIDC provider config (`OIDCIssuer`/`OIDCClientID`/`OIDCGroupsScope` — read-only here, since
those are deployment-time env vars per `docker-compose.yml`, not DB-editable secrets).

`POST /auth/session` and `POST /auth/local` are the two "addition beyond the handoff's
listed API surface" routes flagged in the design spec's Auth & RBAC section — both are
**unauthenticated at the HTTP layer** (no bearer token required), which is only safe because
`siem-api` sits on the `backend` Docker network with no route from the browser: only
`siem-web`'s BFF can reach either one. `/auth/session` is called after the BFF has already
verified an OIDC ID token against Pocket ID's JWKS itself; `/auth/local` is the break-glass
path. Both return `{user_id, role, display_name}` for the BFF to embed into the internal JWT
it mints — neither one mints a token itself (see Task 10's note on why minting lives in the
BFF, not here).

- [ ] **Step 1: Write the failing tests**

`siem-api/internal/api/settings_auth_test.go`:
```go
package api

import (
    "bytes"
    "context"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/hibikipr/homeSIEM/siem-api/internal/store"
)

func TestGetAuthSettings_RequiresAdmin(t *testing.T) {
    s, st := newTestServer(t)
    token := authToken(t, st, "analyst", 50)

    req := httptest.NewRequest(http.MethodGet, "/settings/auth", nil)
    req.Header.Set("Authorization", "Bearer "+token)
    rec := httptest.NewRecorder()
    s.Handler().ServeHTTP(rec, req)

    if rec.Code != http.StatusForbidden {
        t.Fatalf("status = %d, want 403", rec.Code)
    }
}

func TestGetAuthSettings_ReturnsConfigAndMappings(t *testing.T) {
    s, st := newTestServer(t)
    s.deps.OIDCIssuer = "https://pocketid.townsville.cc"
    s.deps.OIDCClientID = "homeSIEM"
    s.deps.OIDCGroupsScope = "groups"
    ctx := context.Background()
    if _, err := st.UpsertRoleMapping(ctx, store.RoleMapping{GroupClaim: "admins", Role: "admin", Priority: 10}); err != nil {
        t.Fatalf("UpsertRoleMapping() error = %v", err)
    }

    token := authToken(t, st, "admin", 5)
    req := httptest.NewRequest(http.MethodGet, "/settings/auth", nil)
    req.Header.Set("Authorization", "Bearer "+token)
    rec := httptest.NewRecorder()
    s.Handler().ServeHTTP(rec, req)

    if rec.Code != http.StatusOK {
        t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
    }
    var resp authSettingsResponse
    if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
        t.Fatalf("json.Unmarshal() error = %v", err)
    }
    if resp.OIDCIssuer != "https://pocketid.townsville.cc" {
        t.Errorf("OIDCIssuer = %q", resp.OIDCIssuer)
    }
    found := false
    for _, m := range resp.RoleMappings {
        if m.GroupClaim == "admins" && m.Role == "admin" {
            found = true
        }
    }
    if !found {
        t.Errorf("RoleMappings = %+v, want to contain admins->admin", resp.RoleMappings)
    }
}

func TestUpdateAuthSettings_UpsertsMappings(t *testing.T) {
    s, st := newTestServer(t)
    token := authToken(t, st, "admin", 5)

    body := `{"role_mappings":[{"group_claim":"homelab","role":"viewer","priority":100}]}`
    req := httptest.NewRequest(http.MethodPut, "/settings/auth", bytes.NewReader([]byte(body)))
    req.Header.Set("Authorization", "Bearer "+token)
    rec := httptest.NewRecorder()
    s.Handler().ServeHTTP(rec, req)

    if rec.Code != http.StatusNoContent {
        t.Fatalf("status = %d, want 204, body=%s", rec.Code, rec.Body.String())
    }

    mappings, err := st.ListRoleMappings(context.Background())
    if err != nil {
        t.Fatalf("ListRoleMappings() error = %v", err)
    }
    found := false
    for _, m := range mappings {
        if m.GroupClaim == "homelab" && m.Role == "viewer" {
            found = true
        }
    }
    if !found {
        t.Errorf("mappings = %+v, want to contain homelab->viewer", mappings)
    }
}
```

`siem-api/internal/api/auth_session_test.go`:
```go
package api

import (
    "bytes"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/hibikipr/homeSIEM/siem-api/internal/auth"
)

func withAuthDeps(s *Server) {
    s.deps.SessionEst = auth.NewSessionEstablisher(s.deps.Store, s.deps.Store)
    s.deps.LocalAuth = auth.NewLocalAuthenticator(s.deps.Store)
}

func TestAuthSession_MappedGroupSucceeds(t *testing.T) {
    s, st := newTestServer(t)
    withAuthDeps(s)
    if _, err := st.UpsertRoleMapping(context.Background(), store.RoleMapping{GroupClaim: "siem-analysts", Role: "analyst", Priority: 50}); err != nil {
        t.Fatalf("UpsertRoleMapping() error = %v", err)
    }

    body := `{"subject":"sub-1","email":"alice@townsville.cc","display_name":"Alice","groups":["siem-analysts"]}`
    req := httptest.NewRequest(http.MethodPost, "/auth/session", bytes.NewReader([]byte(body)))
    rec := httptest.NewRecorder()
    s.Handler().ServeHTTP(rec, req)

    if rec.Code != http.StatusOK {
        t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
    }
    var resp sessionResponse
    if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
        t.Fatalf("json.Unmarshal() error = %v", err)
    }
    if resp.Role != "analyst" || resp.UserID == 0 {
        t.Errorf("resp = %+v", resp)
    }
}

func TestAuthSession_UnmappedGroupDenied(t *testing.T) {
    s, _ := newTestServer(t)
    withAuthDeps(s)

    body := `{"subject":"sub-1","email":"a@b.c","display_name":"A","groups":["no-mapping"]}`
    req := httptest.NewRequest(http.MethodPost, "/auth/session", bytes.NewReader([]byte(body)))
    rec := httptest.NewRecorder()
    s.Handler().ServeHTTP(rec, req)

    if rec.Code != http.StatusForbidden {
        t.Fatalf("status = %d, want 403", rec.Code)
    }
}

func TestAuthLocal_ValidCredentialsSucceed(t *testing.T) {
    s, st := newTestServer(t)
    withAuthDeps(s)
    hash, err := bcrypt.GenerateFromPassword([]byte("correct-horse"), bcrypt.DefaultCost)
    if err != nil {
        t.Fatalf("GenerateFromPassword() error = %v", err)
    }
    if _, err := st.EnsureLocalAdmin(context.Background(), "admin", string(hash)); err != nil {
        t.Fatalf("EnsureLocalAdmin() error = %v", err)
    }

    body := `{"username":"admin","password":"correct-horse"}`
    req := httptest.NewRequest(http.MethodPost, "/auth/local", bytes.NewReader([]byte(body)))
    rec := httptest.NewRecorder()
    s.Handler().ServeHTTP(rec, req)

    if rec.Code != http.StatusOK {
        t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
    }
    var resp sessionResponse
    if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
        t.Fatalf("json.Unmarshal() error = %v", err)
    }
    if resp.Role != "admin" {
        t.Errorf("Role = %q, want admin", resp.Role)
    }
}

func TestAuthLocal_InvalidCredentialsRejected(t *testing.T) {
    s, _ := newTestServer(t)
    withAuthDeps(s)

    body := `{"username":"ghost","password":"anything"}`
    req := httptest.NewRequest(http.MethodPost, "/auth/local", bytes.NewReader([]byte(body)))
    rec := httptest.NewRecorder()
    s.Handler().ServeHTTP(rec, req)

    if rec.Code != http.StatusUnauthorized {
        t.Fatalf("status = %d, want 401", rec.Code)
    }
}
```

Add these imports to `auth_session_test.go`'s import block:
```go
import (
    "bytes"
    "context"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/hibikipr/homeSIEM/siem-api/internal/auth"
    "github.com/hibikipr/homeSIEM/siem-api/internal/store"
    "golang.org/x/crypto/bcrypt"
)
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd siem-api && go test ./internal/api/... -run "AuthSettings|AuthSession|AuthLocal" -v`
Expected: FAIL — handlers/types undefined.

- [ ] **Step 3: Register the routes and extend `Deps`**

In `siem-api/internal/api/server.go`, add `OIDCIssuer`, `OIDCClientID`, `OIDCGroupsScope
string` to `Deps`, and extend `routes()`:
```go
    s.mux.Handle("GET /settings/auth", protect(s.deps.Verifier, s.deps.Store, "admin", http.HandlerFunc(s.handleGetAuthSettings)))
    s.mux.Handle("PUT /settings/auth", protect(s.deps.Verifier, s.deps.Store, "admin", http.HandlerFunc(s.handleUpdateAuthSettings)))
    s.mux.HandleFunc("POST /auth/session", s.handleAuthSession)
    s.mux.HandleFunc("POST /auth/local", s.handleAuthLocal)
```

- [ ] **Step 4: Write the handlers**

`siem-api/internal/api/settings_auth.go`:
```go
package api

import (
    "encoding/json"
    "net/http"

    "github.com/hibikipr/homeSIEM/siem-api/internal/store"
)

type roleMappingResp struct {
    ID         int64  `json:"id"`
    GroupClaim string `json:"group_claim"`
    Role       string `json:"role"`
    Priority   int    `json:"priority"`
}

type authSettingsResponse struct {
    OIDCIssuer      string            `json:"oidc_issuer"`
    OIDCClientID    string            `json:"oidc_client_id"`
    OIDCGroupsScope string            `json:"oidc_groups_scope"`
    RoleMappings    []roleMappingResp `json:"role_mappings"`
}

func (s *Server) handleGetAuthSettings(w http.ResponseWriter, r *http.Request) {
    mappings, err := s.deps.Store.ListRoleMappings(r.Context())
    if err != nil {
        http.Error(w, "list role mappings failed", http.StatusInternalServerError)
        return
    }

    resp := authSettingsResponse{
        OIDCIssuer: s.deps.OIDCIssuer, OIDCClientID: s.deps.OIDCClientID, OIDCGroupsScope: s.deps.OIDCGroupsScope,
    }
    for _, m := range mappings {
        resp.RoleMappings = append(resp.RoleMappings, roleMappingResp{ID: m.ID, GroupClaim: m.GroupClaim, Role: m.Role, Priority: m.Priority})
    }
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(resp)
}

type updateAuthSettingsRequest struct {
    RoleMappings []roleMappingResp `json:"role_mappings"`
}

func (s *Server) handleUpdateAuthSettings(w http.ResponseWriter, r *http.Request) {
    var req updateAuthSettingsRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, "invalid json body", http.StatusBadRequest)
        return
    }

    for _, m := range req.RoleMappings {
        if _, err := s.deps.Store.UpsertRoleMapping(r.Context(), store.RoleMapping{
            GroupClaim: m.GroupClaim, Role: m.Role, Priority: m.Priority,
        }); err != nil {
            s.deps.Logger.Error("upsert role mapping failed", "group", m.GroupClaim, "error", err)
            http.Error(w, "update role mappings failed", http.StatusInternalServerError)
            return
        }
    }
    w.WriteHeader(http.StatusNoContent)
}
```

`siem-api/internal/api/auth_session.go`:
```go
package api

import (
    "encoding/json"
    "net/http"
)

type sessionResponse struct {
    UserID      int64  `json:"user_id"`
    Role        string `json:"role"`
    DisplayName string `json:"display_name"`
}

type sessionRequest struct {
    Subject     string   `json:"subject"`
    Email       string   `json:"email"`
    DisplayName string   `json:"display_name"`
    Groups      []string `json:"groups"`
}

func (s *Server) handleAuthSession(w http.ResponseWriter, r *http.Request) {
    var req sessionRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, "invalid json body", http.StatusBadRequest)
        return
    }

    user, err := s.deps.SessionEst.Establish(r.Context(), req.Subject, req.Email, req.DisplayName, req.Groups)
    if err != nil {
        http.Error(w, "denied", http.StatusForbidden)
        return
    }

    displayName := ""
    if user.DisplayName != nil {
        displayName = *user.DisplayName
    }
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(sessionResponse{UserID: user.ID, Role: user.Role, DisplayName: displayName})
}

type localLoginRequest struct {
    Username string `json:"username"`
    Password string `json:"password"`
}

func (s *Server) handleAuthLocal(w http.ResponseWriter, r *http.Request) {
    var req localLoginRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, "invalid json body", http.StatusBadRequest)
        return
    }

    user, err := s.deps.LocalAuth.Login(r.Context(), req.Username, req.Password)
    if err != nil {
        http.Error(w, "invalid credentials", http.StatusUnauthorized)
        return
    }

    displayName := ""
    if user.DisplayName != nil {
        displayName = *user.DisplayName
    }
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(sessionResponse{UserID: user.ID, Role: user.Role, DisplayName: displayName})
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd siem-api && go test ./internal/api/... -v`
Expected: PASS — every `internal/api` test from Tasks 22-28 passes together.

- [ ] **Step 6: Commit**

```bash
git add siem-api/internal/api/server.go siem-api/internal/api/settings_auth.go siem-api/internal/api/auth_session.go siem-api/internal/api/settings_auth_test.go siem-api/internal/api/auth_session_test.go
git commit -m "Add siem-api api: /settings/auth, POST /auth/session, POST /auth/local"
```

---

### Task 29: `cmd/siem-api` — wire everything together

**Files:**
- Modify: `siem-api/cmd/siem-api/main.go` (replace the Task 1 stub)

**Interfaces:**
- Consumes: every package built in Tasks 2-28.
- Produces: the runnable `siem-api` binary.

This task has no behavior of its own to unit-test — it's pure wiring, and every component
it wires together already has its own tests from Tasks 2-28. Verification here is: it
builds, `go vet` is clean, and (since the design's dev/test decision is to use the real
homelab Loki/Pocket ID/ntfy instances, not fakes) it actually starts against them.

- [ ] **Step 1: Write `main.go`**

`siem-api/cmd/siem-api/main.go`:
```go
package main

import (
    "context"
    "log/slog"
    "net/http"
    "os"
    "os/signal"
    "syscall"
    "time"

    "github.com/hibikipr/homeSIEM/siem-api/internal/alerts"
    "github.com/hibikipr/homeSIEM/siem-api/internal/api"
    "github.com/hibikipr/homeSIEM/siem-api/internal/auth"
    "github.com/hibikipr/homeSIEM/siem-api/internal/config"
    "github.com/hibikipr/homeSIEM/siem-api/internal/loki"
    "github.com/hibikipr/homeSIEM/siem-api/internal/ntfy"
    "github.com/hibikipr/homeSIEM/siem-api/internal/rules"
    "github.com/hibikipr/homeSIEM/siem-api/internal/sse"
    "github.com/hibikipr/homeSIEM/siem-api/internal/store"
)

func main() {
    logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

    cfg, err := config.Load()
    if err != nil {
        logger.Error("config load failed", "error", err)
        os.Exit(1)
    }

    db, err := store.Open(cfg.DatabaseURL)
    if err != nil {
        logger.Error("db open failed", "error", err)
        os.Exit(1)
    }
    defer db.Close()
    if err := store.Migrate(db); err != nil {
        logger.Error("db migrate failed", "error", err)
        os.Exit(1)
    }
    st := store.New(db)

    if cfg.LocalAdminUsername != "" && cfg.LocalAdminPasswordHash != "" {
        if _, err := st.EnsureLocalAdmin(context.Background(), cfg.LocalAdminUsername, cfg.LocalAdminPasswordHash); err != nil {
            logger.Error("ensure local admin failed", "error", err)
            os.Exit(1)
        }
    }

    lokiClient := loki.New(cfg.LokiURL, &http.Client{Timeout: 30 * time.Second})
    ntfyClient := ntfy.New(cfg.NtfyURL, cfg.NtfyTopic, cfg.NtfyToken, &http.Client{Timeout: 10 * time.Second})
    hub := sse.NewHub()
    alertsSvc := alerts.NewService(st, hub, ntfyClient, logger)

    evaluators := map[string]rules.Evaluator{
        "threshold":  &rules.ThresholdEvaluator{Querier: lokiClient},
        "first_seen": &rules.FirstSeenEvaluator{Querier: lokiClient, Seen: st},
        "absence":    &rules.AbsenceEvaluator{Sources: st},
    }
    scheduler := rules.NewScheduler(st, evaluators, alertsSvc, logger)

    verifier := auth.NewTokenVerifier(cfg.SessionSecret)
    sessionEst := auth.NewSessionEstablisher(st, st)
    localAuth := auth.NewLocalAuthenticator(st)

    appCtx, cancel := context.WithCancel(context.Background())
    defer cancel()

    if err := scheduler.Start(appCtx); err != nil {
        logger.Error("scheduler start failed", "error", err)
        os.Exit(1)
    }
    defer scheduler.Stop()

    go api.RunTailPoller(appCtx, lokiClient, cfg.LokiJobLabel, hub, time.Second, logger)

    server := api.NewServer(api.Deps{
        Store: st, Loki: lokiClient, JobLabel: cfg.LokiJobLabel, Hub: hub,
        Alerts: alertsSvc, Scheduler: scheduler, SchedulerCtx: appCtx,
        Verifier: verifier, SessionEst: sessionEst, LocalAuth: localAuth,
        FastpathToken: cfg.FastpathToken,
        OIDCIssuer: cfg.OIDCIssuer, OIDCClientID: cfg.OIDCClientID, OIDCGroupsScope: cfg.OIDCGroupsScope,
        Logger: logger,
    })

    httpServer := &http.Server{Addr: cfg.Addr, Handler: server.Handler()}

    go func() {
        logger.Info("siem-api listening", "addr", cfg.Addr)
        if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            logger.Error("http server failed", "error", err)
            os.Exit(1)
        }
    }()

    stop := make(chan os.Signal, 1)
    signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
    <-stop

    logger.Info("shutting down")
    cancel()

    shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer shutdownCancel()
    if err := httpServer.Shutdown(shutdownCtx); err != nil {
        logger.Error("http server shutdown error", "error", err)
    }
}
```

- [ ] **Step 2: Build and vet the whole module**

Run: `cd siem-api && go build ./... && go vet ./...`
Expected: both succeed with no output.

- [ ] **Step 3: Run the full test suite**

Run: `cd siem-api && go test ./... -v`
Expected: every test from Tasks 2-28 passes.

- [ ] **Step 4: Smoke-test against the real homelab instances**

Per the design spec's dev-environment decision (test against real Loki/Pocket ID/ntfy, not
fakes), run the binary locally with real values for the required env vars and confirm it
starts and serves `/healthz`:

```bash
cd siem-api
export DATABASE_URL="sqlite:///tmp/siem-smoketest.db"
export LOKI_URL="http://<homelab-loki-host>:3100"
export NTFY_URL="http://<homelab-ntfy-host>"
export NTFY_TOPIC="homesiem"
export OIDC_ISSUER="https://pocketid.townsville.cc"
export OIDC_CLIENT_ID="homeSIEM"
export SIEM_SESSION_SECRET=$(openssl rand -base64 32)
export SIEM_FASTPATH_TOKEN="smoketest-token"
go run ./cmd/siem-api &
sleep 1
curl -sf http://localhost:8080/healthz && echo " OK"
kill %1
rm -f /tmp/siem-smoketest.db
```

Expected: `ok OK` printed, process exits cleanly on `kill` (graceful shutdown log line, no
panic). If `LOKI_URL`/`NTFY_URL` aren't reachable from wherever this runs, `/healthz` still
succeeds (it doesn't check them) — the point of this step is confirming the binary starts
and the scheduler/tail-poller goroutines don't crash on launch, not exercising real queries.

- [ ] **Step 5: Commit**

```bash
git add siem-api/cmd/siem-api/main.go
git commit -m "Wire siem-api: config, store, clients, scheduler, HTTP server, graceful shutdown"
```

---

### Task 30: Verify the Dockerfile builds a real image

**Files:**
- Modify: `siem-api/Dockerfile` (only if Step 1 surfaces a problem — see below)

**Interfaces:**
- Consumes: the Dockerfile written in Task 1, now exercised against the real dependency set
  from Tasks 2-29 (`modernc.org/sqlite`, `golang-jwt/jwt/v5`, `golang.org/x/crypto/bcrypt`).

Task 1 wrote the Dockerfile before any dependencies existed; this task is purely
verification now that `go.sum` is real. `modernc.org/sqlite` is pure Go, so
`CGO_ENABLED=0` (already in the Dockerfile) needs no adjustment for it — this step exists
to catch anything unexpected, not because a specific problem is anticipated.

- [ ] **Step 1: Build the image**

Run: `cd siem-api && docker build -t siem-api:local .`
Expected: builds successfully. If it fails, fix `siem-api/Dockerfile` (most likely cause:
a missing `go.sum` — Docker's build context excludes it if `.dockerignore` was ever edited
carelessly; confirm `go.sum` is NOT in `.dockerignore`) and retry until it succeeds.

- [ ] **Step 2: Run the image and confirm `/healthz`**

```bash
docker run --rm -d --name siem-api-smoketest \
  -e DATABASE_URL="sqlite:///data/siem.db" \
  -e LOKI_URL="http://127.0.0.1:3100" \
  -e NTFY_URL="http://127.0.0.1" \
  -e NTFY_TOPIC="homesiem" \
  -e OIDC_ISSUER="https://pocketid.townsville.cc" \
  -e OIDC_CLIENT_ID="homeSIEM" \
  -e SIEM_SESSION_SECRET="$(openssl rand -base64 32)" \
  -e SIEM_FASTPATH_TOKEN="smoketest-token" \
  -p 18080:8080 \
  siem-api:local
sleep 1
curl -sf http://localhost:18080/healthz && echo " OK"
docker logs siem-api-smoketest
docker stop siem-api-smoketest
```

Expected: `ok OK`, and `docker logs` shows the "siem-api listening" line with no panics —
`LOKI_URL`/`NTFY_URL` pointing at nothing reachable is fine here, same as Task 29 Step 4.

- [ ] **Step 3 (best-effort): spot-check the arm64 cross-build**

Run: `cd siem-api && docker buildx build --platform linux/arm64 -t siem-api:arm64-check .`
Expected: builds successfully. If `docker buildx` isn't set up for multi-platform builds on
this machine, skip this step — full multi-arch build-and-push to `ghcr.io` is explicitly
owned by the later deployment-scaffold sub-project (per the design spec), this is only a
sanity check that nothing in the Go code itself is architecture-specific.

- [ ] **Step 4: Commit (only if Step 1 required a Dockerfile fix)**

```bash
git add siem-api/Dockerfile
git commit -m "Fix siem-api Dockerfile for the real dependency set"
```

If Step 1 succeeded without changes, there's nothing to commit — this task is verification-only.

---

### Task 31: End-to-end integration test — scheduler raises a real alert for a threshold rule

**Files:**
- Create: `siem-api/internal/rules/scheduler_integration_test.go`

**Interfaces:**
- Consumes: `Scheduler` (Task 21), `ThresholdEvaluator` (Task 18), `alerts.Service` (Task 17),
  a real `store.Store` (Tasks 3-9), a real `loki.Client` (Task 13) pointed at a fake Loki
  `httptest.Server`. No fakes for `alerts`/`store`/`loki` themselves — this is the one test
  in the suite that wires real components together end-to-end, per the design spec's
  testing section ("one integration test booting the real scheduler against a fake Loki").

This is the last task in the plan: if it passes, every layer — schema, store, auth, loki
client, alert lifecycle, rule evaluation, and the scheduler — is proven to compose
correctly, not just individually.

- [ ] **Step 1: Write the test**

`siem-api/internal/rules/scheduler_integration_test.go`:
```go
package rules

import (
    "context"
    "net/http"
    "net/http/httptest"
    "path/filepath"
    "testing"
    "time"

    "github.com/hibikipr/homeSIEM/siem-api/internal/alerts"
    "github.com/hibikipr/homeSIEM/siem-api/internal/loki"
    "github.com/hibikipr/homeSIEM/siem-api/internal/sse"
    "github.com/hibikipr/homeSIEM/siem-api/internal/store"
)

func TestSchedulerEndToEnd_ThresholdRuleRaisesRealAlert(t *testing.T) {
    fakeLoki := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        w.Write([]byte(`{"status":"success","data":{"result":[
            {"stream":{"job":"siem"},"values":[
                ["1700000000000000000", "{\"src_ip\":\"10.0.0.5\"}"],
                ["1700000001000000000", "{\"src_ip\":\"10.0.0.5\"}"],
                ["1700000002000000000", "{\"src_ip\":\"10.0.0.5\"}"]
            ]}
        ]}}`))
    }))
    defer fakeLoki.Close()

    dbPath := filepath.Join(t.TempDir(), "siem.db")
    db, err := store.Open("sqlite://" + dbPath)
    if err != nil {
        t.Fatalf("Open() error = %v", err)
    }
    defer db.Close()
    if err := store.Migrate(db); err != nil {
        t.Fatalf("Migrate() error = %v", err)
    }
    st := store.New(db)

    threshold := 3
    rule, err := st.CreateRule(context.Background(), store.Rule{
        Name: "wan-portscan", Shape: "threshold", LogQL: `{job="siem"}`,
        WindowSec: 60, Threshold: &threshold, GroupBy: []string{"src_ip"},
        Severity: "critical", Destinations: []string{"inapp"},
        CooldownSec: 3600, IntervalSec: 1, Enabled: true,
    }, nil)
    if err != nil {
        t.Fatalf("CreateRule() error = %v", err)
    }

    hub := sse.NewHub()
    alertsSvc := alerts.NewService(st, hub, nil, schedulerTestLogger())
    lokiClient := loki.New(fakeLoki.URL, fakeLoki.Client())
    evaluators := map[string]Evaluator{"threshold": &ThresholdEvaluator{Querier: lokiClient}}
    scheduler := NewScheduler(st, evaluators, alertsSvc, schedulerTestLogger())

    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()
    if err := scheduler.Start(ctx); err != nil {
        t.Fatalf("Start() error = %v", err)
    }
    defer scheduler.Stop()

    deadline := time.Now().Add(5 * time.Second)
    for time.Now().Before(deadline) {
        openAlerts, err := st.ListAlerts(context.Background(), "open")
        if err != nil {
            t.Fatalf("ListAlerts() error = %v", err)
        }
        if len(openAlerts) > 0 {
            if openAlerts[0].RuleID != rule.ID {
                t.Fatalf("RuleID = %d, want %d", openAlerts[0].RuleID, rule.ID)
            }
            if openAlerts[0].GroupKey != "10.0.0.5" {
                t.Fatalf("GroupKey = %q, want 10.0.0.5", openAlerts[0].GroupKey)
            }
            if openAlerts[0].Severity != "critical" {
                t.Fatalf("Severity = %q, want critical", openAlerts[0].Severity)
            }
            return // success
        }
        time.Sleep(50 * time.Millisecond)
    }
    t.Fatal("timed out waiting for the scheduler to raise a real alert end-to-end")
}
```

- [ ] **Step 2: Run the test**

Run: `cd siem-api && go test ./internal/rules/... -run EndToEnd -v`
Expected: PASS.

- [ ] **Step 3: Run the entire module's test suite one final time**

Run: `cd siem-api && go build ./... && go vet ./... && go test ./... -v`
Expected: every test across every package (Tasks 2-31) passes.

- [ ] **Step 4: Commit**

```bash
git add siem-api/internal/rules/scheduler_integration_test.go
git commit -m "Add end-to-end integration test: scheduler raises a real alert for a threshold rule"
```
