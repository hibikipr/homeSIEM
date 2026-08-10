# siem-insights: LLM-powered log review — design

Status: proposed (design only — not yet approved for implementation)
Scope: new capability spanning `siem-api` (an Ollama client, an orchestration
service, storage, an HTTP surface) and `siem-web` (a Wall panel plus a new
`/insights` screen for history/detail). This is the first outbound-LLM
integration anywhere in this codebase — no existing precedent to build on
beyond the general "external optional dependency, env-var configured,
degrades gracefully when unset" shape already established by `ntfy`/OIDC.

## Context

Over the last several sessions, fixing severity misclassification bugs in
`siem-ingest`'s `enrich_geo` cascade has followed the same manual workflow
every time (see PRs #41, #42, #50, #54, #59, #60, #61): query Loki for
`severity=err`/`warning` over a recent window, group by `program`, dedupe by
message shape, sample distinct patterns, and reason about which are genuine
and which are self-reported levels the pipeline isn't detecting yet. That
workflow is effective but entirely manual and ad hoc — nothing in the app
itself surfaces "here's what looks off in the last N hours" without a human
running those queries by hand.

This design automates the same shape of review on a schedule (plus
on-demand), using a locally-hosted LLM via [Ollama](https://ollama.com), and
surfaces the results back into the app as reviewable suggestions — matching
the existing Alerts philosophy (the system proposes, an operator triages),
not an autonomous-remediation system.

**Where Ollama runs:** on the user's MacBook Pro (M5 Max, 32GB unified
memory), not the Raspberry Pi that hosts homeSIEM itself — the Pi has
nowhere near the compute for a 24-30B model. `siem-api` reaches it as a
plain HTTP dependency over the LAN, the same relationship it already has
with `ntfy`.

## Goals

- A scheduled background pass (interval configurable, default on the order
  of 30-60 minutes — this is not a per-minute rule) gathers a summary of
  recent operational signal, sends it to Ollama, and stores whatever
  structured suggestions come back.
- An on-demand "Analyze now" trigger for the same pass, outside the
  schedule.
- A compact panel on the Wall dashboard: the most recent few suggestions,
  severity-coded, with a dismiss action — same relationship `TriageCard`
  already has to the full Alerts screen.
- A new `/insights` screen: full history (including dismissed), each
  suggestion's full detail and the evidence it was based on.
- Ollama's URL and model are env-var configured only, matching the
  OIDC/ntfy precedent of "connection settings aren't UI-editable deployment
  secrets."

## Non-goals (this pass)

- **No agentic tool-calling or auto-remediation.** The model never edits
  `vector.toml`, never creates/modifies rules, never touches anything. It
  produces text; a human decides whether to act on it, the same as any
  other alert in this app.
- **No conversational/chat interface.** One-shot structured output per
  pass, not a back-and-forth. A "why do you think that" follow-up chat is a
  plausible future extension, not this pass.
- **No per-user Ollama configuration.** One global instance, one model,
  same as there's one global ntfy topic.
- **No fallback to a cloud LLM.** If Ollama is unreachable, that pass's
  generation just fails and is skipped/retried next cycle — matches the
  existing "ntfy not configured → notifications silently don't send"
  degrade-gracefully posture, not a hard failure.
- **No feedback loop.** Dismissing a suggestion doesn't influence future
  passes. Each pass is stateless given its input data.

## Model recommendation: `qwen3(.6):27b`, not `devstral:24b`

Devstral is Mistral's agentic-*coding* model — tuned for tool-calling and
code edits, not general analytical reasoning. It's the wrong tool here:
this task is "read structured data, find patterns, write a structured
recommendation," squarely a general-instruction-following task, where
Qwen3 at this size is meaningfully stronger — better structured-output
reliability, and better at staying grounded in the data it's given rather
than confabulating beyond it.

On the target hardware: a 27B model at Ollama's default `Q4_K_M`
quantization is roughly 16-18GB of weights. On 32GB unified memory that
leaves room for macOS plus a reasonable KV cache (8-16k context is plenty
for the aggregated-summary approach below) — but it's the majority of the
machine, so this isn't something to run concurrently with another
GPU-heavy workload on the same Mac.

If the scheduled cadence ends up wanting to run more often than the dense
27B comfortably supports, a Qwen3 MoE variant (`30b-a3b`) activates far
fewer parameters per token despite a similar on-disk footprint, so it's
noticeably faster per request — worth keeping as a second pull, reserved
for the scheduled pass, with the dense 27B reserved for on-demand "look
deeper" requests. Not built as a two-tier system in this pass (see
Non-goals) — `OLLAMA_MODEL` is one value — but the config is intentionally
a single env var specifically so switching models later is a one-line
change, not a code change.

## Data flow / prompt design

Not raw log lines — that doesn't scale to a context window and is exactly
the firehose the manual workflow above already learned to avoid. Each pass
gathers the same *shape* of data that workflow uses by hand:

1. **Open alerts summary** — counts by severity, titles, ages. Reuses
   `AlertStore`'s existing list query, no new Loki access needed.
2. **Severity × program rollup** for the lookback window — the same
   aggregate the Wall's heat grid already computes
   (`internal/api/stats.go`'s `queryHourlyBySource`-adjacent query), reused
   rather than re-implemented.
3. **Deduplicated `err`/`warning` samples** — one Loki query per severity,
   grouped by `(program, first ~60 chars of message)`, capped at some
   bounded count of distinct patterns (e.g. top 50 by frequency) rather
   than every raw line. This is the piece that mattered most in practice
   during the manual passes — volume is almost always repeats of the same
   underlying pattern, and what's actually informative is the *distinct
   shapes*, not the count.

The system prompt constrains the model to: cite only programs/patterns
present in the given data, no speculation beyond it, and respond with a
JSON array matching a fixed schema:

```json
[
  {
    "title": "string, one line",
    "detail": "string, a few sentences",
    "severity": "info|warning|critical",
    "category": "severity-misclassification|operational|security|other",
    "evidence": [{"program": "string", "sample_message": "string", "count": 0}]
  }
]
```

A response that fails to parse as that schema is logged and the pass
produces zero insights rather than a malformed row — never a best-effort
partial parse.

## Backend structure

```text
siem-api/internal/
  ollama/
    client.go        # Client.Chat(ctx, systemPrompt, userPrompt string) (string, error)
    client_test.go    # fake HTTP server, matches ntfy's client_test.go shape
  insights/
    prompt.go         # gathers the three data sources above, builds the prompt
    prompt_test.go     # the dedup/capping logic is the most complex pure-logic
                        # piece here and gets the most thorough coverage
    service.go         # GenerateNow(ctx): prompt.Build -> ollama.Chat -> parse -> store
    service_test.go
    scheduler.go       # ticker wrapping GenerateNow on INSIGHTS_INTERVAL_SEC,
                        # same shape as rules.Scheduler but simpler (one job,
                        # not N per-rule intervals)
```

### Store

New table, added via the existing always-run migration step
(`store.Migrate()`'s post-`schema.sql` block — see the
`notification_settings` precedent in
`2026-08-08-settings-notifications-design.md` for why this can't just go in
`schema.sql`):

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

`internal/store/insights.go` (new file, matching the one-concept-per-file
convention): `InsertInsight`, `ListInsights(ctx, includeDismissed bool,
limit int)`, `DismissInsight(ctx, id)`.

### `siem-api` HTTP surface

- `GET /insights` (`viewer`+): recent, non-dismissed by default;
  `?all=true` includes dismissed for the history view.
- `POST /insights/generate` (`analyst`+/`admin`): triggers `GenerateNow`
  synchronously. A 24-27B model on the target hardware should return in
  well under a minute for the response sizes this schema implies, so a
  blocking request is the v1 simplification — see Known gaps if this turns
  out to be too slow in practice.
- `PUT /insights/{id}/dismiss` (`analyst`+/`admin`).

### Config

New `Config` fields (`internal/config/config.go`), all optional — unset
`OllamaURL` means the scheduler never starts and `/insights/generate`
404s/400s the same way ntfy's test-notification route does when
unconfigured:

- `OLLAMA_URL`
- `OLLAMA_MODEL`
- `INSIGHTS_INTERVAL_SEC` (default e.g. `1800`)
- `INSIGHTS_LOOKBACK_MIN` (default e.g. `60` — how far back each pass looks)

### `siem-web`

- `lib/server/siemApiClient.ts`: `getInsights`, `generateInsightsNow`,
  `dismissInsight`, matching the existing method-per-endpoint convention.
- `lib/components/InsightsPanel.svelte`: Wall card, top 3-5 non-dismissed,
  severity-coded, each with a dismiss button — same interaction shape as
  `TriageCard`.
- `routes/insights/+page.server.ts` + `+page.svelte`: full list (including
  dismissed) and per-item detail (the evidence array, full text).
- `routes/api/insights/+server.ts` (GET) and
  `routes/api/insights/generate/+server.ts` (POST) and
  `routes/api/insights/[id]/+server.ts` (PUT dismiss): same-origin proxy
  routes, mirroring every other screen's existing pattern.
- `Nav.svelte`: new "Insights" nav item, visible to all roles (same
  visibility as Alerts) — added once the full screen exists, not left as a
  Wall-only feature with no other entry point.

## Testing

- `internal/ollama`: fake HTTP server tests (request shape, response
  parsing, timeout/error handling) — matches `ntfy/client_test.go`.
- `internal/insights`: `prompt_test.go` covers the dedup/capping logic in
  isolation (this is the piece most worth getting right); `service_test.go`
  covers `GenerateNow` against a fake store + fake Ollama client, including
  the malformed-JSON-response path (must log and produce zero insights, not
  crash or partially insert).
- `internal/store`: round-trip test for the three `insights` methods, plus
  a `Migrate()` test confirming the table appears against a database that
  already has `sources` (simulating an existing deployment) — same shape as
  the `notification_settings` migration test.
- `internal/api`: handler tests for the three routes (including the
  not-configured 400 and the invalid-id-on-dismiss 404).
- `siem-web`: `siemApiClient.test.ts` additions for the three new client
  methods; `+page.server.ts` test additions for the new load. Manual
  browser verification for `InsightsPanel` and the new screen (this
  project's established no-component-test-infrastructure constraint).

## Known gaps after this pass

- On-demand generation blocks the HTTP request for the model's full
  response time — no progress indicator, no async job + poll. Fine at
  current model speed; revisit if it becomes a real UX problem.
- No feedback loop — see Non-goals. A future pass could let
  dismiss/accept feed back into what gets included in later prompts.
- Single global model and interval — no per-category tuning (e.g. a
  faster/cheaper model for routine scheduled passes vs. a slower one
  reserved for on-demand deep dives). Noted as a plausible future
  refinement, not built here — see the model recommendation section.
- No retry queue for a pass that fails because Ollama was temporarily
  unreachable or overloaded — it's simply skipped, and the next scheduled
  tick tries again.
