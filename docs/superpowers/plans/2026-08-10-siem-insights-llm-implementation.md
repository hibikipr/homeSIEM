# siem-insights: LLM-powered log review — Implementation Plan

**Design doc:** `docs/superpowers/specs/2026-08-10-siem-insights-llm-design.md`

**Goal:** A scheduled (plus on-demand) pass that gathers a deduplicated
summary of recent operational signal (open alerts, a severity×program
rollup, deduplicated `err`/`warning` log samples), sends it to a
locally-hosted LLM via Ollama, and stores the structured suggestions that
come back — surfaced as a compact panel on the Wall and a full `/insights`
history screen. Advisory only: the model never touches `vector.toml` or
rules, it only produces text a human reviews.

**Architecture:** New `siem-api/internal/ollama` package (a minimal HTTP
client, one method: `Chat`). New `siem-api/internal/insights` package
(prompt assembly, orchestration, a ticker-based scheduler) sitting
alongside `internal/rules` and `internal/alerts` as a peer, not nested
inside either. A new `insights` table via the existing always-run
migration step. Three new HTTP routes mirroring the existing
Settings/Alerts route shapes exactly. `siem-web` gets three new client
methods, three new same-origin proxy routes, a Wall panel component, and a
new `/insights` screen — all following the established per-screen pattern
(`lib/server/siemApiClient.ts` method → proxy route → page load → Svelte
component).

**Tech Stack:** Go (`siem-api`, SQLite via `modernc.org/sqlite`), SvelteKit
(Svelte 5 runes), TypeScript, Vitest. Ollama's HTTP API
(`POST /api/chat`, `stream: false`, non-streaming single-shot response).

## Global Constraints

- **No tool-calling, no agentic loop.** The model receives a prompt and
  returns text. Nothing in this feature ever writes to `vector.toml`,
  creates/edits rules, or takes any action beyond inserting rows into the
  `insights` table. If a future pass wants to add "create a rule from this
  suggestion," that's a distinct, separate feature — not built here.
- **Ollama is optional, matching `ntfy`'s existing degrade-gracefully
  posture.** `OLLAMA_URL` unset means: the scheduler never starts, and
  `POST /insights/generate` returns 400 (same shape as
  `handleTestNotification`'s existing not-configured check in
  `settings_notifications.go`). Never a hard failure at startup.
- **A malformed or unparseable model response produces zero insights for
  that pass, not a partial or best-effort one.** Log the raw response at
  `warn` level (for debugging prompt/model issues) and return cleanly —
  never crash the scheduler loop, never insert a malformed row.
- **`store.Migrate()`'s one-time `schema.sql` bootstrap is a no-op against
  any already-populated database** (gated on `sources` already existing).
  The new `insights` table goes in the existing always-run
  `migrations.sql` (already established by the `notification_settings`
  precedent — the mechanism already exists, this plan only adds to the
  file, it does not need to build the mechanism itself).
- **Not raw log lines.** Every data-gathering step in Task 4 must cap and
  deduplicate before it ever reaches the prompt — see the design doc's
  Data flow section for why (this repo's own manual severity-audit passes
  this whole feature automates learned that the hard way: raw volume is
  almost entirely repeats of a small number of distinct patterns).
- **This codebase has no Svelte component test infrastructure and none
  should be added** (see `docs/superpowers/plans/2026-08-07-nav-avatar-picture.md`'s
  Global Constraints for the established reasoning). Cover Svelte-adjacent
  logic with plain Vitest unit tests against `.ts` files; verify the new
  Wall panel and `/insights` screen manually in a browser once deployed.
- Real severity vocabulary is `info`/`warning`/`critical` only (see
  `RuleFromEventForm.svelte`'s `<select>`) — the model's own `severity`
  field on each returned insight must be validated against exactly this
  set server-side before insertion; treat anything else as `info` (this
  codebase's existing "unrecognized falls to the lowest tier" convention).

---

### Task 1: `siem-api/internal/ollama` — minimal HTTP client

**Files:**
- Create: `siem-api/internal/ollama/client.go`
- Create: `siem-api/internal/ollama/client_test.go`

**Interfaces produced:**
```go
type Client struct { /* baseURL, model string; httpClient *http.Client */ }
func New(baseURL, model string, httpClient *http.Client) *Client
func (c *Client) Chat(ctx context.Context, systemPrompt, userPrompt string) (string, error)
```

- [ ] **Step 1:** Write `client_test.go` first, mirroring
      `internal/ntfy/client_test.go`'s shape (`httptest.NewServer`, assert
      on the request the client sends, and on how it parses a canned
      response). Cover:
      - Request shape: `POST {baseURL}/api/chat`, JSON body with
        `model`, `messages: [{role: "system", content: systemPrompt},
        {role: "user", content: userPrompt}]`, `stream: false`.
      - Successful response parsing: Ollama's chat response shape is
        `{"message": {"role": "assistant", "content": "..."}, "done": true, ...}`
        — `Chat` returns just the `message.content` string.
      - A non-200 response returns an error (don't try to parse the body
        as a chat response).
      - A request that times out (use a slow/hanging test handler with a
        short client timeout) returns an error, not a hang.
- [ ] **Step 2:** Run `go test ./internal/ollama/... -v`, confirm every
      test fails (package doesn't exist yet).
- [ ] **Step 3:** Implement `client.go` to make the tests pass. Timeout
      should be configurable via the passed-in `*http.Client` (same
      pattern every other client in this codebase already uses — see how
      `main.go` constructs `ntfy.New`/`loki.New` with an explicit
      `&http.Client{Timeout: ...}`), not hardcoded inside the package. A
      27B-class model's response can genuinely take tens of seconds — the
      caller (Task 5) is responsible for picking a timeout long enough,
      not this package guessing one.
- [ ] **Step 4:** `go test ./internal/ollama/...` — verify green.
      `gofmt -l internal/ollama/` — verify clean.

---

### Task 2: Store — `insights` table, migration, CRUD methods

**Files:**
- Modify: `siem-api/internal/store/migrations.sql` (append, don't touch
  the existing `notification_settings` block)
- Create: `siem-api/internal/store/insights.go`
- Create: `siem-api/internal/store/insights_test.go`

**Interfaces produced:**
```go
type Insight struct {
    ID           int64
    CreatedAt    time.Time
    Title        string
    Detail       string
    Severity     string // info/warning/critical
    Category     string
    EvidenceJSON string // JSON array, opaque to the store layer
    Dismissed    bool
}

func (s *Store) InsertInsight(ctx context.Context, in Insight) (Insight, error)
func (s *Store) ListInsights(ctx context.Context, includeDismissed bool, limit int) ([]Insight, error)
func (s *Store) DismissInsight(ctx context.Context, id int64) error
```

- [ ] **Step 1:** Append to `migrations.sql` (after the existing
      `notification_settings` block, same file, same
      always-runs-every-startup semantics):
      ```sql
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
      ```
      No seed row needed here (unlike `notification_settings`, which is a
      single-row config table) — `insights` starts genuinely empty.
- [ ] **Step 2:** Write `insights_test.go` first:
      - `TestInsertInsight_RoundTrips`: insert, list, assert every field
        matches (including `Dismissed` defaulting to `false`).
      - `TestListInsights_ExcludesDismissedByDefault`: insert two, dismiss
        one, `ListInsights(ctx, false, 10)` returns only the other;
        `ListInsights(ctx, true, 10)` returns both.
      - `TestListInsights_RespectsLimit`.
      - `TestListInsights_OrdersNewestFirst`.
      - `TestDismissInsight_UnknownID_ReturnsError` (or a no-op — decide
        during implementation which matches `sql.Result.RowsAffected()`
        conventions already used elsewhere in this package, e.g.
        `AckAlert`'s handling of an unknown ID).
      - A `Migrate()` test in the same shape as
        `TestMigrate_AddsNotificationSettingsToExistingDatabase` (see
        `store_test.go`), confirming `insights` appears against a
        database that already has `sources` — this is the test that
        would catch a regression to the migration mechanism itself.
- [ ] **Step 3:** Run tests, confirm they fail (table/methods don't exist).
- [ ] **Step 4:** Implement `insights.go` against `migrations.sql`'s new
      table.
- [ ] **Step 5:** `go test ./internal/store/...` — verify green,
      including the full existing suite (not just the new tests) to catch
      any migration-ordering regression.

---

### Task 3: Config additions

**Files:**
- Modify: `siem-api/internal/config/config.go`
- Modify: `siem-api/internal/config/config_test.go`

**Interfaces produced:** four new optional `Config` fields —
`OllamaURL`, `OllamaModel`, `InsightsIntervalSec` (default `1800`),
`InsightsLookbackMin` (default `60`). None are in the `required` map —
this feature is entirely optional per the Global Constraints.

- [ ] **Step 1:** Add tests to `config_test.go` mirroring the existing
      `TestLoad_DefaultsApplied`/`TestLoad_AppURLReadFromEnv` shape:
      - Defaults apply when unset (`InsightsIntervalSec == 1800`,
        `InsightsLookbackMin == 60`, `OllamaURL == ""`, `OllamaModel ==
        ""`).
      - Explicit env values are read correctly.
- [ ] **Step 2:** Confirm the new tests fail.
- [ ] **Step 3:** Add the fields to `Config` and `Load()`, following the
      exact `getenv(key, fallback)` pattern already used for
      `LokiJobLabel`/`VectorGraphQLURL`. `InsightsIntervalSec`/
      `InsightsLookbackMin` are ints — check whether `getenv` needs an
      int-returning sibling or whether `Load()` should
      `strconv.Atoi(getenv(...))` inline (check if any existing config
      field is already numeric before deciding — if this is the first,
      keep the conversion inline in `Load()` rather than adding a new
      helper for a single use).
- [ ] **Step 4:** `go test ./internal/config/...` — verify green.

---

### Task 4: `siem-api/internal/insights` — prompt assembly (the dedup logic)

**Files:**
- Create: `siem-api/internal/insights/prompt.go`
- Create: `siem-api/internal/insights/prompt_test.go`

This is the piece most worth getting right — see Global Constraints on
why raw volume can't reach the prompt directly.

**Interfaces produced:**
```go
type PromptBuilder struct {
    Loki     LokiQuerier // QueryRange, QueryInstant - same interfaces rules/stats already use
    Alerts   AlertLister // a narrow interface: ListOpenAlerts(ctx) ([]store.Alert, error)
    JobLabel string
}

func (b *PromptBuilder) Build(ctx context.Context, lookback time.Duration) (systemPrompt, userPrompt string, err error)
```

- [ ] **Step 1:** Design and write `prompt_test.go` first, against fake
      `LokiQuerier`/`AlertLister` implementations (same
      fake-struct-in-the-test-file pattern used throughout this codebase,
      e.g. `fakeAlertStore` in `alerts/service_test.go`). Cover, as
      separate testable helper functions (not just one opaque `Build`
      call — extract each step so it's independently testable):
      - `dedupeSamples(entries []loki.LogEntry, capCount int) []sampleGroup`
        — groups by `(program, first 60 chars of message)`, counts
        occurrences per group, returns the top `capCount` by frequency.
        Test: more distinct patterns than the cap → only the most
        frequent survive; two entries with the same program but
        different message prefixes → two separate groups; entries
        differing only after char 60 → the same group (this is the exact
        behavior this whole feature is modeling on the manual audits'
        `msg[:60]` deduplication key used throughout this session).
      - `buildSeverityProgramRollup(...)` — the counts table, built from
        one or two `QueryInstant` calls with a `sum by (program)
        (count_over_time(...))` LogQL query per severity (`err`,
        `warning`) over `lookback` — note this is NOT the same query
        `stats.go`'s `queryHourlyBySource` runs (that one groups `by
        (source)` and buckets hourly for the heat grid; this wants a
        single rollup over the whole lookback window grouped `by
        (program)`), so this is new LogQL, not a reused function — see
        Global Constraints.
      - `formatAlertsSummary(alerts []store.Alert) string`.
      - The final `Build()` assembly: fixed system prompt (constrains the
        model to the given data only, fixed JSON schema in the
        instructions — see the design doc's Data flow section for the
        exact schema) + a user prompt that's the three sections
        concatenated in a stable, labeled order (`## Open alerts` / `##
        Severity by program` / `## Recent error/warning samples`) so
        output is deterministic given the same input data.
- [ ] **Step 2:** Confirm tests fail (nothing implemented yet).
- [ ] **Step 3:** Implement `prompt.go`.
- [ ] **Step 4:** `go test ./internal/insights/...` — verify green.
      `gofmt -l internal/insights/`.

---

### Task 5: `siem-api/internal/insights` — service + scheduler

**Files:**
- Create: `siem-api/internal/insights/service.go`
- Create: `siem-api/internal/insights/service_test.go`
- Create: `siem-api/internal/insights/scheduler.go`

**Interfaces produced:**
```go
type Chatter interface {
    Chat(ctx context.Context, systemPrompt, userPrompt string) (string, error)
}
type InsightStore interface {
    InsertInsight(ctx context.Context, in store.Insight) (store.Insight, error)
}

type Service struct { /* PromptBuilder, Chatter, InsightStore, Lookback time.Duration, Logger *slog.Logger */ }
func NewService(...) *Service
func (s *Service) GenerateNow(ctx context.Context) error

type Scheduler struct { /* Service, IntervalSec int */ }
func NewScheduler(svc *Service, intervalSec int, logger *slog.Logger) *Scheduler
func (sch *Scheduler) Start(ctx context.Context) // blocks until ctx.Done(), same shape as how main.go already runs the rules scheduler in a goroutine
```

- [ ] **Step 1:** Write `service_test.go` first, against a fake
      `Chatter` and fake `InsightStore` (local fakes in the test file,
      matching every other package's testing convention in this
      codebase — do not introduce a mocking library). Cover:
      - `GenerateNow` happy path: fake `Chatter.Chat` returns a valid
        JSON array matching the schema → each element becomes one
        `InsertInsight` call with the right fields.
      - Malformed JSON response → zero `InsertInsight` calls, no error
        returned to the caller (logged, not propagated as a hard
        failure — this must not crash the scheduler loop; see Global
        Constraints), but confirm the test can observe the warning was
        logged (inject a test logger, same pattern as
        `alerts/service_test.go`'s `testLogger()`).
      - A response element with an invalid `severity` value (not
        `info`/`warning`/`critical`) gets coerced to `info` before
        insertion, not rejected outright (matches the "unrecognized
        falls to the lowest tier" convention — Global Constraints).
      - `Chatter.Chat` returning an error → `GenerateNow` returns that
        error (the caller — the scheduler — decides how to log/retry),
        zero `InsertInsight` calls.
      - `PromptBuilder.Build` returning an error (e.g. Loki unreachable)
        → same shape, propagated up, zero inserts.
- [ ] **Step 2:** Confirm tests fail.
- [ ] **Step 3:** Implement `service.go`. Response parsing: the model may
      wrap the JSON array in prose or markdown code fences despite the
      system prompt's instructions (real-world LLM behavior) — extract
      the first `[...]` substring before `json.Unmarshal` rather than
      assuming the entire response is bare JSON; if no `[` is found,
      that's the malformed-response path above, not a panic.
- [ ] **Step 4:** `go test ./internal/insights/...` — verify green.
- [ ] **Step 5:** Implement `scheduler.go` — a plain `time.Ticker` loop
      calling `GenerateNow` and logging any error at `warn` (never fatal
      — one failed pass must not stop future ticks), same "one job on an
      interval" shape as the simplest case `rules.Scheduler` handles per
      rule, but without that type's per-rule concurrency machinery (this
      is one job, not N). No dedicated test file for the ticker loop
      itself is required (matches this codebase's precedent of not unit
      testing bare ticker plumbing — `GenerateNow` is where the real
      logic lives and is already covered); a manual smoke check once
      wired into `main.go` (Task 6) is sufficient.

---

### Task 6: `siem-api` HTTP routes + `main.go` wiring

**Files:**
- Create: `siem-api/internal/api/insights.go`
- Create: `siem-api/internal/api/insights_test.go`
- Modify: `siem-api/internal/api/server.go` (`Deps` struct + route
  registration)
- Modify: `siem-api/cmd/siem-api/main.go`

**Interfaces produced:**
- `GET /insights` (`RoleViewer`) — query param `?all=true` for
  `includeDismissed`.
- `POST /insights/generate` (`RoleAnalyst`) — calls `Service.GenerateNow`
  synchronously (see design doc's Known gaps on this being a v1
  simplification), returns the newly-created insights or an empty array.
- `PUT /insights/{id}/dismiss` (`RoleAnalyst`).

- [ ] **Step 1:** Write `insights_test.go` first, against `newTestServer`
      (same helper every other `internal/api` test file uses), mirroring
      `settings_notifications_test.go`'s shape for the
      not-configured-returns-400 case:
      - `GET /insights` as viewer → 200, correct JSON shape.
      - `GET /insights?all=true` includes dismissed.
      - `POST /insights/generate` as viewer → 403 (below `RoleAnalyst`).
      - `POST /insights/generate` when `Deps.Insights` is nil (Ollama not
        configured) → 400, matching `handleTestNotification`'s existing
        not-configured shape exactly.
      - `PUT /insights/{id}/dismiss` as analyst → 204; unknown ID → 404.
- [ ] **Step 2:** Confirm tests fail.
- [ ] **Step 3:** Implement `insights.go` handlers, add `Insights
      *insights.Service` to `Deps` in `server.go`, register the three
      routes in `routes()` (find that method — the route list shown in
      Global Constraints research is in `server.go`'s route-registration
      block) with the roles above, following the `protect(s.deps.Verifier,
      s.deps.Store, auth.RoleX, http.HandlerFunc(...))` wrapper shape
      used by every existing protected route.
- [ ] **Step 4:** `go test ./internal/api/...` — verify green.
- [ ] **Step 5:** Wire into `main.go`: construct `ollama.New(cfg.OllamaURL,
      cfg.OllamaModel, &http.Client{Timeout: ...})` (pick a generous
      timeout — see Task 1, this is where it's actually chosen, e.g. 120s),
      the `insights.PromptBuilder`, `insights.NewService(...)`, and —
      **only when `cfg.OllamaURL != ""`** — `insights.NewScheduler(...)`
      started in a goroutine the same way `main.go` already starts the
      rules scheduler. When unset, `Deps.Insights` stays `nil` and the
      handlers' nil-check (Step 3) takes over.
- [ ] **Step 6:** `go build ./... && go vet ./...` — full build check
      across the whole module, not just the new packages, to catch any
      `Deps` struct-literal call site elsewhere that needs updating.

---

### Task 7: `siem-web` client methods + proxy routes

**Files:**
- Modify: `siem-web/src/lib/server/siemApiClient.ts`
- Modify: `siem-web/src/lib/server/siemApiClient.test.ts`
- Create: `siem-web/src/routes/api/insights/+server.ts` (GET)
- Create: `siem-web/src/routes/api/insights/generate/+server.ts` (POST)
- Create: `siem-web/src/routes/api/insights/[id]/+server.ts` (PUT dismiss)

**Interfaces produced:**
```ts
export interface Insight {
    id: number;
    created_at: string;
    title: string;
    detail: string;
    severity: string;
    category: string;
    evidence: { program: string; sample_message: string; count: number }[];
    dismissed: boolean;
}
async getInsights(sessionToken: string, includeDismissed?: boolean): Promise<Insight[]>
async generateInsightsNow(sessionToken: string): Promise<Insight[]>
async dismissInsight(sessionToken: string, id: number): Promise<void>
```

- [ ] **Step 1:** Add `siemApiClient.test.ts` cases for the three new
      methods, mirroring the existing `getNavSummary`/`updateRule` test
      shape exactly (fake `fetch`, assert URL/method/headers, assert
      response parsing).
- [ ] **Step 2:** Confirm they fail (methods don't exist).
- [ ] **Step 3:** Implement the three client methods.
- [ ] **Step 4:** Implement the three proxy routes, each following
      `routes/api/rules/[id]/+server.ts`'s exact shape (same-origin proxy
      holding the session token server-side, surfacing the upstream
      status/error on failure rather than swallowing it).
- [ ] **Step 5:** `pnpm exec vitest run` — verify green. `pnpm lint`.
      `pnpm exec svelte-check`.

---

### Task 8: Wall panel

**Files:**
- Create: `siem-web/src/lib/components/InsightsPanel.svelte`
- Modify: `siem-web/src/routes/+page.server.ts` (load: also fetch
  `getInsights(token)`, non-dismissed, capped — same
  supplementary-not-gating pattern `+layout.server.ts`'s nav summary
  already uses: a fetch failure here must not break the Wall)
- Modify: `siem-web/src/routes/+page.svelte` (render the panel)

- [ ] **Step 1:** Add/extend `+page.server.ts`'s existing test file for
      the new load-time call, including the fetch-failure-degrades-not-throws
      case (same shape as the `+layout.server.ts` nav-summary test's
      "falls back to zeros without throwing" case).
- [ ] **Step 2:** Confirm it fails.
- [ ] **Step 3:** Implement the load addition.
- [ ] **Step 4:** Build `InsightsPanel.svelte`: props `insights:
      Insight[]`, `onDismiss: (id: number) => void`; renders the top 3-5,
      severity-color-coded (reuse `severityColor` from `$lib/tail`, same
      as every other severity-colored dot in this app), title + one-line
      detail truncated, a dismiss button per row, and a "View all →" link
      to `/insights` (Task 9). Empty state: a quiet "No insights yet" —
      not an error, matches this app's established empty-state tone
      (`TailViewport`'s "Waiting for events…" precedent) — this is
      expected on a fresh deployment or before the first scheduled pass
      has run.
- [ ] **Step 5:** Wire the dismiss button to `dismissInsight` (Task 7)
      with an optimistic UI removal, matching how `AlertRow`'s
      ack/mute buttons already update local state before/alongside the
      server round-trip (check `AlertInbox.svelte`/`AlertRow.svelte` for
      the exact established pattern before inventing a new one).
- [ ] **Step 6:** Manual browser check once deployed (no component test
      infra — Global Constraints).

---

### Task 9: `/insights` screen + Nav entry

**Files:**
- Create: `siem-web/src/routes/insights/+page.server.ts`
- Create: `siem-web/src/routes/insights/+page.svelte`
- Modify: `siem-web/src/lib/components/Nav.svelte`

- [ ] **Step 1:** `+page.server.ts` load: `getInsights(token, true)`
      (includes dismissed, for the full history view) — this one IS
      gating content (a real page, not supplementary chrome), so follows
      the same auth-gate/error-handling shape every other screen's own
      `+page.server.ts` uses (e.g. `search/+page.server.ts`), not the
      degrade-quietly shape from Task 8.
- [ ] **Step 2:** `+page.svelte`: a simple list (reuse severity coloring
      + category as a small tag), each item expandable to show `detail`
      and the `evidence` array in full (program / sample message / count
      per evidence row — a small table, not prose). Dismissed items shown
      with reduced opacity or a "Dismissed" badge rather than hidden, so
      the "full history" promise from the design doc actually holds. A
      "Generate now" button calling `generateInsightsNow` (Task 7),
      disabled while a request is in flight, appending results to the
      top of the list on success.
- [ ] **Step 3:** Add `{ label: 'Insights', href: '/insights' }` to
      `Nav.svelte`'s `navItems` array, visible to every role (same
      visibility as Alerts/Search/Live tail/Sources — only Settings is
      role-gated in the existing `visibleNavItems` filter).
- [ ] **Step 4:** Manual browser check (Global Constraints).

---

### Task 10: Deployment config

**Files:**
- Modify: `.env.example` (root)
- Modify: `docker-compose.yml` (root) — `siem-api` service environment
  block
- Modify: `siem-ingest/../README.md`? No — modify the **root**
  `README.md`'s Architecture table (siem-api's one-line description) and
  `siem-web/README.md`'s "What's built" list, matching how every prior
  feature in this repo updated both on landing (see the README-refresh
  precedent from earlier this session, PRs #52/#57).

- [ ] **Step 1:** Add `OLLAMA_URL`, `OLLAMA_MODEL`,
      `INSIGHTS_INTERVAL_SEC`, `INSIGHTS_LOOKBACK_MIN` to `.env.example`
      with a comment explaining they're all optional and what happens
      when unset (matches `SIEM_APP_URL`'s existing comment style).
- [ ] **Step 2:** Add the corresponding `environment:` lines to the root
      `docker-compose.yml`'s `siem-api` service block (`- OLLAMA_URL=${OLLAMA_URL}`
      etc. — no defaults via `:-`, since unset is the valid "feature off"
      state, unlike `TZ`'s `:-UTC` pattern).
- [ ] **Step 3:** Note in the PR description (not necessarily the repo
      itself) that the **live** `homelab-siem` stack's `docker-compose.yml`
      (in the separate `homelab` infra repo) needs the same env vars added
      by hand before this feature does anything in production — this repo
      only ships the reference compose file, matching every prior
      deployment-config change's precedent this session.
- [ ] **Step 4:** Update root `README.md`'s Architecture table row for
      `siem-api` and `siem-web/README.md`'s "What's built" section to
      mention Insights, matching the exact prose style already
      established there.

---

## Testing summary (cross-reference)

- `internal/ollama`: fake HTTP server, request/response shape, error
  paths.
- `internal/store`: round-trip + migration idempotency for `insights`.
- `internal/config`: defaults + env overrides for the four new fields.
- `internal/insights`: dedup/rollup logic in isolation (`prompt_test.go`);
  orchestration against fake `Chatter`/`InsightStore`, including the
  malformed-response and severity-coercion paths (`service_test.go`).
- `internal/api`: handler tests for all three routes, including the
  not-configured-400 and role-gating cases.
- `siem-web`: `siemApiClient.test.ts` additions; `+page.server.ts` test
  additions for both the Wall's supplementary fetch and the `/insights`
  screen's gating load.
- Manual: Wall panel and `/insights` screen in a browser once deployed —
  no component test infra in this codebase (Global Constraints).

## Known gaps after this pass

Carried from the design doc — not resolved by this plan, intentionally
deferred:

- On-demand generation blocks the HTTP request for the model's full
  response time.
- No feedback loop from dismiss/accept into future prompts.
- Single global model/interval, no per-category tuning.
- No retry queue for a pass that fails because Ollama was unreachable —
  it's simply skipped until the next tick.
