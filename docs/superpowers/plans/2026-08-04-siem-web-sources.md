# siem-web: Sources screen — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship the Sources screen (`/sources`) — a table of every source siem-ingest has
heartbeated, a parser preview, an ingest-health panel backed by Vector's GraphQL API, and
an unclaimed-senders/claim flow — per
`docs/superpowers/specs/2026-08-04-siem-web-sources-design.md`.

**Architecture:** Three small siem-api additions (two new response fields on the existing
`GET /sources`, one new `GET /sources/ingest-health` endpoint backed by a new Vector
GraphQL client) plus a new SvelteKit route that SSR-loads all three siem-api calls once per
visit — no polling, no SSE, following the Alerts screen's query-param-driven-selection
precedent (`?preview=<name>`) instead of a new proxy route for row selection.

**Tech Stack:** Go 1.x (siem-api), SvelteKit 5 + TypeScript + Vitest (siem-web). No new
dependencies — the Vector GraphQL client is a plain `net/http` POST, matching
`internal/loki/client.go`'s existing shape.

## Global Constraints

- Labels/response field names are `snake_case` JSON (matches every existing siem-api
  response type — `sourceResponse`, `statsResponse`, etc.).
- Every new siem-api route requires at least `viewer` role via the existing `protect(...)`
  middleware wrapper — never register a route without it (except the two pre-existing
  unauthenticated exceptions, `/healthz` and the fastpath-token-gated `/sources/heartbeat`,
  which this plan does not touch).
- `GET /sources/ingest-health` must never 502 the whole page over Vector being unreachable
  — return `200` with `"degraded": true` and zeroed metrics instead (per spec). A hard
  502/error is reserved for the primary `GET /sources` and auth failures only.
- No polling, no `setInterval`, no new SSE endpoint for this screen — SSR-load-once per
  visit (per spec's "Refresh strategy" decision).
- `internal/vector`'s GraphQL query is verified against a real `timberio/vector:0.49.0-alpine`
  instance's introspected schema (done during planning) — `sources { nodes { componentId
  metrics { receivedEventsTotal { receivedEventsTotal } } } }` and equivalent for `sinks`/
  `sentEventsTotal`. Component error counts are **not** queryable over one-shot HTTP
  (`componentErrorsTotals` is a `Subscription`-only field) — do not attempt to fetch them.
- Claim button and its proxy route are gated the same way the codebase already gates
  admin-only UI (Alerts' mute/ack): hidden for non-admin roles client-side, and the
  underlying siem-api route already enforces `admin`+ server-side — this plan does not
  relax that.

---

### Task 1: siem-api — Vector GraphQL client + config

**Files:**
- Create: `siem-api/internal/vector/client.go`
- Create: `siem-api/internal/vector/client_test.go`
- Modify: `siem-api/internal/config/config.go`
- Modify: `siem-api/internal/config/config_test.go`

**Interfaces:**
- Produces: `vector.New(baseURL string, httpClient *http.Client) *vector.Client` and
  `(*vector.Client) Query(ctx context.Context, query string) (json.RawMessage, error)` —
  POSTs `{"query": query}` to `{baseURL}/graphql`, returns the response's `data` field as
  raw JSON, or an error if the HTTP status isn't 200 or the response carries a GraphQL
  `errors` array. Task 3 calls this with a fixed query string and unmarshals the raw JSON
  itself (mirrors how `loki.Client.QueryRange`/`QueryMatrix` are plain request/parse
  methods that callers in `internal/api` compose).
- Produces: `config.Config.VectorGraphQLURL string`, defaulting to `http://siem-ingest:8686`
  via `getenv("VECTOR_GRAPHQL_URL", "http://siem-ingest:8686")` — not added to the
  `required` map (an unset/wrong value degrades `GET /sources/ingest-health` per Global
  Constraints, it doesn't fail startup).

- [ ] **Step 1: Write the failing client test**

Create `siem-api/internal/vector/client_test.go`:

```go
package vector

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestQuery_ReturnsData(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/graphql" {
			t.Errorf("request = %s %s, want POST /graphql", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":{"sources":{"nodes":[{"componentId":"unifi"}]}}}`))
	}))
	defer srv.Close()

	c := New(srv.URL, srv.Client())
	data, err := c.Query(context.Background(), `{ sources { nodes { componentId } } }`)
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if !strings.Contains(string(data), `"componentId":"unifi"`) {
		t.Fatalf("data = %s, want it to contain componentId unifi", data)
	}
}

func TestQuery_SurfacesGraphQLErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":null,"errors":[{"message":"field not found"}]}`))
	}))
	defer srv.Close()

	c := New(srv.URL, srv.Client())
	if _, err := c.Query(context.Background(), `{ bogus }`); err == nil {
		t.Fatal("Query() error = nil, want error for a GraphQL errors[] response")
	}
}

func TestQuery_SurfacesHTTPErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	c := New(srv.URL, srv.Client())
	if _, err := c.Query(context.Background(), `{ sources { nodes { componentId } } }`); err == nil {
		t.Fatal("Query() error = nil, want error for a non-200 response")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd siem-api && go test ./internal/vector/...`
Expected: FAIL — `package vector: no Go files in ...` (the package doesn't exist yet).

- [ ] **Step 3: Write the client implementation**

Create `siem-api/internal/vector/client.go`:

```go
package vector

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

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

type graphQLRequest struct {
	Query string `json:"query"`
}

type graphQLError struct {
	Message string `json:"message"`
}

type graphQLResponse struct {
	Data   json.RawMessage `json:"data"`
	Errors []graphQLError  `json:"errors"`
}

// Query POSTs a GraphQL query to Vector's API and returns the raw `data`
// field for the caller to unmarshal into its own shape.
func (c *Client) Query(ctx context.Context, query string) (json.RawMessage, error) {
	body, err := json.Marshal(graphQLRequest{Query: query})
	if err != nil {
		return nil, fmt.Errorf("vector: marshal query: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/graphql", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("vector: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("vector: request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("vector: request failed: status=%d", resp.StatusCode)
	}

	var parsed graphQLResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("vector: decode response: %w", err)
	}
	if len(parsed.Errors) > 0 {
		return nil, fmt.Errorf("vector: graphql error: %s", parsed.Errors[0].Message)
	}
	return parsed.Data, nil
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd siem-api && go test ./internal/vector/...`
Expected: PASS (3 tests).

- [ ] **Step 5: Add the config field, test-first**

In `siem-api/internal/config/config_test.go`, add an assertion to
`TestLoad_DefaultsApplied` (after the existing `OIDCGroupsScope` check):

```go
	if cfg.VectorGraphQLURL != "http://siem-ingest:8686" {
		t.Errorf("VectorGraphQLURL = %q, want http://siem-ingest:8686", cfg.VectorGraphQLURL)
	}
```

Run: `cd siem-api && go test ./internal/config/...`
Expected: FAIL — `cfg.VectorGraphQLURL undefined`.

- [ ] **Step 6: Add the field to Config**

In `siem-api/internal/config/config.go`, add to the `Config` struct (after `LokiJobLabel`):

```go
	VectorGraphQLURL       string
```

And in `Load()`'s `cfg := Config{...}` literal (after `LokiJobLabel`):

```go
		VectorGraphQLURL: getenv("VECTOR_GRAPHQL_URL", "http://siem-ingest:8686"),
```

- [ ] **Step 7: Run the config test to verify it passes**

Run: `cd siem-api && go test ./internal/config/...`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add siem-api/internal/vector siem-api/internal/config/config.go siem-api/internal/config/config_test.go
git commit -m "Add Vector GraphQL client and VECTOR_GRAPHQL_URL config"
```

---

### Task 2: siem-api — `GET /sources` gains `status` and `events_per_min`

**Files:**
- Modify: `siem-api/internal/api/sources.go`
- Modify: `siem-api/internal/api/sources_test.go`

**Interfaces:**
- Consumes: `s.deps.Loki.QueryMatrix(ctx, logql string, start, end time.Time, step time.Duration) (loki.MatrixResult, error)`
  where `MatrixResult{Series []MatrixSeries}`, `MatrixSeries{Labels map[string]string,
  Samples []MatrixSample}`, `MatrixSample{Timestamp time.Time, Value float64}` — already
  used identically in `stats.go`'s `queryHourlyBySource`. `s.deps.Loki` may be `nil` in
  tests that don't set it (see `newTestServer`) — must not panic in that case.
- Produces: `sourceResponse` gains `Status string` (`"healthy"` or `"silent"`) and
  `EventsPerMin float64`, both always present in the JSON (no `omitempty`) so the frontend
  never has to special-case a missing field.

- [ ] **Step 1: Write the failing tests**

In `siem-api/internal/api/sources_test.go`, add `"time"` and
`"github.com/hibikipr/homeSIEM/siem-api/internal/loki"` to the imports, then add:

```go
func TestListSources_HealthyWhenWithinHeartbeat(t *testing.T) {
	s, st := newTestServer(t)
	ctx := context.Background()
	src, err := st.UpsertSource(ctx, store.Source{
		Name: "udm-ultra", Address: "10.0.0.1", Transport: "udp/514", Parser: "unifi-os", HeartbeatSec: 900,
	})
	if err != nil {
		t.Fatalf("UpsertSource() error = %v", err)
	}
	if err := st.TouchSourceLastSeen(ctx, src.Name, time.Now().UTC()); err != nil {
		t.Fatalf("TouchSourceLastSeen() error = %v", err)
	}

	token := authToken(t, st, "viewer", 100)
	req := httptest.NewRequest(http.MethodGet, "/sources", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	var got []sourceResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if len(got) != 1 || got[0].Status != "healthy" {
		t.Fatalf("got = %+v, want status=healthy", got)
	}
}

func TestListSources_SilentWhenNeverSeen(t *testing.T) {
	s, st := newTestServer(t)
	ctx := context.Background()
	if _, err := st.UpsertSource(ctx, store.Source{
		Name: "unclaimed-host", Address: "10.0.0.2", Transport: "tcp/601", Parser: "rfc5424", HeartbeatSec: 900,
	}); err != nil {
		t.Fatalf("UpsertSource() error = %v", err)
	}

	token := authToken(t, st, "viewer", 100)
	req := httptest.NewRequest(http.MethodGet, "/sources", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	var got []sourceResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if len(got) != 1 || got[0].Status != "silent" {
		t.Fatalf("got = %+v, want status=silent (never seen)", got)
	}
}

func TestListSources_SilentWhenPastHeartbeat(t *testing.T) {
	s, st := newTestServer(t)
	ctx := context.Background()
	src, err := st.UpsertSource(ctx, store.Source{
		Name: "udm-ultra", Address: "10.0.0.1", Transport: "udp/514", Parser: "unifi-os", HeartbeatSec: 60,
	})
	if err != nil {
		t.Fatalf("UpsertSource() error = %v", err)
	}
	if err := st.TouchSourceLastSeen(ctx, src.Name, time.Now().UTC().Add(-10*time.Minute)); err != nil {
		t.Fatalf("TouchSourceLastSeen() error = %v", err)
	}

	token := authToken(t, st, "viewer", 100)
	req := httptest.NewRequest(http.MethodGet, "/sources", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	var got []sourceResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if len(got) != 1 || got[0].Status != "silent" {
		t.Fatalf("got = %+v, want status=silent (10m since last seen, 60s heartbeat)", got)
	}
}

func TestListSources_IncludesEventsPerMin(t *testing.T) {
	fakeLoki := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"success","data":{"resultType":"matrix","result":[
			{"metric":{"source":"udm-ultra"},"values":[[1700000000,"25"],[1700000300,"50"]]}
		]}}`))
	}))
	defer fakeLoki.Close()

	s, st := newTestServer(t)
	s.deps.Loki = loki.New(fakeLoki.URL, fakeLoki.Client())
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

	var got []sourceResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if len(got) != 1 || got[0].EventsPerMin != 10 {
		t.Fatalf("got = %+v, want events_per_min=10 (50/5)", got)
	}
}

func TestListSources_EventsPerMinZeroWhenLokiUnset(t *testing.T) {
	// Regression guard: handleListSources must not panic when s.deps.Loki is
	// nil, which is the case for every other existing sources_test.go case
	// (newTestServer doesn't set Loki unless a test opts in).
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
	if len(got) != 1 || got[0].EventsPerMin != 0 {
		t.Fatalf("got = %+v, want events_per_min=0 when Loki is unset", got)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd siem-api && go test ./internal/api/... -run TestListSources`
Expected: FAIL — `sourceResponse` has no field `Status`/`EventsPerMin` (compile error).

- [ ] **Step 3: Implement the handler changes**

In `siem-api/internal/api/sources.go`, replace the `sourceResponse` struct, `toSourceResponse`,
and `handleListSources` with:

```go
type sourceResponse struct {
	ID           int64      `json:"id"`
	Name         string     `json:"name"`
	Address      string     `json:"address"`
	Transport    string     `json:"transport"`
	Parser       string     `json:"parser"`
	Claimed      bool       `json:"claimed"`
	HeartbeatSec int        `json:"heartbeat_sec"`
	LastSeenAt   *time.Time `json:"last_seen_at,omitempty"`
	Status       string     `json:"status"`
	EventsPerMin float64    `json:"events_per_min"`
}

func toSourceResponse(src store.Source, now time.Time, eventsPerMin float64) sourceResponse {
	return sourceResponse{
		ID: src.ID, Name: src.Name, Address: src.Address, Transport: src.Transport,
		Parser: src.Parser, Claimed: src.Claimed, HeartbeatSec: src.HeartbeatSec, LastSeenAt: src.LastSeenAt,
		Status:       sourceStatus(src, now),
		EventsPerMin: eventsPerMin,
	}
}

// sourceStatus reimplements the same threshold store.StaleSources uses
// internally, as a plain comparison against fields handleListSources
// already fetched — not a second SQL round trip.
func sourceStatus(src store.Source, now time.Time) string {
	if src.LastSeenAt == nil {
		return "silent"
	}
	if now.Sub(*src.LastSeenAt) > time.Duration(src.HeartbeatSec)*time.Second {
		return "silent"
	}
	return "healthy"
}

func (s *Server) handleListSources(w http.ResponseWriter, r *http.Request) {
	sources, err := s.deps.Store.ListSources(r.Context())
	if err != nil {
		http.Error(w, "list sources failed", http.StatusInternalServerError)
		return
	}

	eventsPerMin := map[string]float64{}
	if s.deps.Loki != nil {
		rates, err := s.queryEventsPerMinBySource(r.Context())
		if err != nil {
			s.deps.Logger.Error("list sources: events-per-min query failed", "error", err)
		} else {
			eventsPerMin = rates
		}
	}

	now := time.Now().UTC()
	resp := make([]sourceResponse, len(sources))
	for i, src := range sources {
		resp[i] = toSourceResponse(src, now, eventsPerMin[src.Name])
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// queryEventsPerMinBySource returns a 5-minute rolling events/min rate per
// source, the same query shape as stats.go's queryHourlyBySource but over a
// short window with no severity filter.
func (s *Server) queryEventsPerMinBySource(ctx context.Context) (map[string]float64, error) {
	end := time.Now().UTC()
	start := end.Add(-5 * time.Minute)
	logql := fmt.Sprintf(`sum by (source) (count_over_time({job=%q}[5m]))`, s.deps.JobLabel)

	result, err := s.deps.Loki.QueryMatrix(ctx, logql, start, end, 5*time.Minute)
	if err != nil {
		return nil, err
	}

	out := map[string]float64{}
	for _, series := range result.Series {
		if len(series.Samples) == 0 {
			continue
		}
		latest := series.Samples[len(series.Samples)-1].Value
		out[series.Labels["source"]] = latest / 5
	}
	return out, nil
}
```

Add `"context"` and `"fmt"` to `sources.go`'s import block (both new — the file currently
imports `encoding/json`, `net/http`, `strconv`, `time`, and the `store` package).

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd siem-api && go test ./internal/api/...`
Expected: PASS (all sources_test.go and stats_test.go cases, including the pre-existing
`TestListSources`/`TestClaimSource_*` cases — `EventsPerMinZeroWhenLokiUnset` is a
regression guard for exactly that risk).

- [ ] **Step 5: Commit**

```bash
git add siem-api/internal/api/sources.go siem-api/internal/api/sources_test.go
git commit -m "Add status and events_per_min to GET /sources"
```

---

### Task 3: siem-api — `GET /sources/ingest-health`

**Files:**
- Create: `siem-api/internal/api/ingest_health.go`
- Create: `siem-api/internal/api/ingest_health_test.go`
- Modify: `siem-api/internal/api/server.go`
- Modify: `siem-api/cmd/siem-api/main.go` (or wherever `Deps` is constructed — see Step 5)

**Interfaces:**
- Consumes: `Task 1`'s `vector.Client.Query(ctx, query string) (json.RawMessage, error)`.
- Consumes: `Deps.Vector *vector.Client` (new field, parallel to the existing `Deps.Loki
  *loki.Client`).
- Produces: `GET /sources/ingest-health` (`viewer`+), response shape:
  ```json
  {
    "received_events_per_source": {"unifi": 1234, "hosts_tcp": 56},
    "loki_sent_events_total": 1290,
    "degraded": false
  }
  ```
  `degraded: true` (with the other two fields zeroed) when `Deps.Vector` is `nil` or the
  GraphQL call fails — this endpoint never 502s.

- [ ] **Step 1: Write the failing tests**

Create `siem-api/internal/api/ingest_health_test.go`:

```go
package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hibikipr/homeSIEM/siem-api/internal/vector"
)

func TestIngestHealth_ReturnsMetricsFromVector(t *testing.T) {
	fakeVector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":{
			"sources":{"nodes":[
				{"componentId":"unifi","metrics":{"receivedEventsTotal":{"receivedEventsTotal":1234}}},
				{"componentId":"hosts_tcp","metrics":{"receivedEventsTotal":{"receivedEventsTotal":56}}}
			]},
			"sinks":{"nodes":[
				{"componentId":"loki","metrics":{"sentEventsTotal":{"sentEventsTotal":1290}}},
				{"componentId":"siem_api","metrics":{"sentEventsTotal":null}}
			]}
		}}`))
	}))
	defer fakeVector.Close()

	s, st := newTestServer(t)
	s.deps.Vector = vector.New(fakeVector.URL, fakeVector.Client())

	token := authToken(t, st, "viewer", 100)
	req := httptest.NewRequest(http.MethodGet, "/sources/ingest-health", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	var got ingestHealthResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if got.Degraded {
		t.Fatal("Degraded = true, want false on a successful Vector response")
	}
	if got.ReceivedEventsPerSource["unifi"] != 1234 || got.ReceivedEventsPerSource["hosts_tcp"] != 56 {
		t.Fatalf("ReceivedEventsPerSource = %+v", got.ReceivedEventsPerSource)
	}
	if got.LokiSentEventsTotal != 1290 {
		t.Fatalf("LokiSentEventsTotal = %v, want 1290", got.LokiSentEventsTotal)
	}
}

func TestIngestHealth_DegradedWhenVectorUnset(t *testing.T) {
	s, st := newTestServer(t) // s.deps.Vector left nil

	token := authToken(t, st, "viewer", 100)
	req := httptest.NewRequest(http.MethodGet, "/sources/ingest-health", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (never 502 for a missing Vector client), body=%s", rec.Code, rec.Body.String())
	}
	var got ingestHealthResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if !got.Degraded {
		t.Fatal("Degraded = false, want true when Deps.Vector is nil")
	}
}

func TestIngestHealth_DegradedWhenVectorUnreachable(t *testing.T) {
	s, st := newTestServer(t)
	s.deps.Vector = vector.New("http://127.0.0.1:1", nil) // nothing listens here

	token := authToken(t, st, "viewer", 100)
	req := httptest.NewRequest(http.MethodGet, "/sources/ingest-health", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	var got ingestHealthResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if !got.Degraded {
		t.Fatal("Degraded = false, want true when the Vector request fails")
	}
}

func TestIngestHealth_RequiresViewerRole(t *testing.T) {
	s, st := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/sources/ingest-health", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 with no Authorization header, body=%s", rec.Code, rec.Body.String())
	}
	_ = st
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd siem-api && go test ./internal/api/... -run TestIngestHealth`
Expected: FAIL — `ingestHealthResponse` undefined, `s.deps.Vector` undefined (compile
errors).

- [ ] **Step 3: Implement the handler**

Create `siem-api/internal/api/ingest_health.go`:

```go
package api

import (
	"encoding/json"
	"net/http"
)

type ingestHealthResponse struct {
	ReceivedEventsPerSource map[string]float64 `json:"received_events_per_source"`
	LokiSentEventsTotal     float64            `json:"loki_sent_events_total"`
	Degraded                bool               `json:"degraded"`
}

const ingestHealthQuery = `{
	sources { nodes { componentId metrics { receivedEventsTotal { receivedEventsTotal } } } }
	sinks { nodes { componentId metrics { sentEventsTotal { sentEventsTotal } } } }
}`

type ingestHealthGraphQLData struct {
	Sources struct {
		Nodes []struct {
			ComponentID string `json:"componentId"`
			Metrics     struct {
				ReceivedEventsTotal *struct {
					ReceivedEventsTotal float64 `json:"receivedEventsTotal"`
				} `json:"receivedEventsTotal"`
			} `json:"metrics"`
		} `json:"nodes"`
	} `json:"sources"`
	Sinks struct {
		Nodes []struct {
			ComponentID string `json:"componentId"`
			Metrics     struct {
				SentEventsTotal *struct {
					SentEventsTotal float64 `json:"sentEventsTotal"`
				} `json:"sentEventsTotal"`
			} `json:"metrics"`
		} `json:"nodes"`
	} `json:"sinks"`
}

func (s *Server) handleIngestHealth(w http.ResponseWriter, r *http.Request) {
	resp := ingestHealthResponse{ReceivedEventsPerSource: map[string]float64{}}

	if s.deps.Vector == nil {
		resp.Degraded = true
		writeJSON(w, resp)
		return
	}

	raw, err := s.deps.Vector.Query(r.Context(), ingestHealthQuery)
	if err != nil {
		s.deps.Logger.Error("ingest health: vector query failed", "error", err)
		resp.Degraded = true
		writeJSON(w, resp)
		return
	}

	var data ingestHealthGraphQLData
	if err := json.Unmarshal(raw, &data); err != nil {
		s.deps.Logger.Error("ingest health: decode vector response failed", "error", err)
		resp.Degraded = true
		writeJSON(w, resp)
		return
	}

	for _, node := range data.Sources.Nodes {
		if node.Metrics.ReceivedEventsTotal != nil {
			resp.ReceivedEventsPerSource[node.ComponentID] = node.Metrics.ReceivedEventsTotal.ReceivedEventsTotal
		}
	}
	for _, node := range data.Sinks.Nodes {
		if node.ComponentID == "loki" && node.Metrics.SentEventsTotal != nil {
			resp.LokiSentEventsTotal = node.Metrics.SentEventsTotal.SentEventsTotal
		}
	}

	writeJSON(w, resp)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
```

- [ ] **Step 4: Wire `Deps.Vector` and the route**

In `siem-api/internal/api/server.go`, add to the `Deps` struct (after the existing `Loki
*loki.Client` field):

```go
	Vector          *vector.Client
```

Add `"github.com/hibikipr/homeSIEM/siem-api/internal/vector"` to `server.go`'s import
block.

In `routes()`, add (next to the existing `GET /sources` line):

```go
	s.mux.Handle("GET /sources/ingest-health", protect(s.deps.Verifier, s.deps.Store, auth.RoleViewer, http.HandlerFunc(s.handleIngestHealth)))
```

- [ ] **Step 5: Wire the real Vector client at startup**

In `siem-api/cmd/siem-api/main.go`, add a line building the real Vector client right after
the existing `lokiClient` line (line 51):

```go
	lokiClient := loki.New(cfg.LokiURL, &http.Client{Timeout: 30 * time.Second})
	vectorClient := vector.New(cfg.VectorGraphQLURL, &http.Client{Timeout: 10 * time.Second})
```

Then add `Vector: vectorClient,` to the `api.Deps{...}` literal (around line 79, next to
the existing `Loki: lokiClient,`):

```go
	server := api.NewServer(api.Deps{
		Store: st, Loki: lokiClient, Vector: vectorClient, JobLabel: cfg.LokiJobLabel, Hub: hub,
```

Add `"github.com/hibikipr/homeSIEM/siem-api/internal/vector"` to `main.go`'s import block.

- [ ] **Step 6: Run the tests to verify they pass**

Run: `cd siem-api && go test ./...`
Expected: PASS (whole suite, including the new ingest-health tests).

- [ ] **Step 7: Commit**

```bash
git add siem-api/internal/api/ingest_health.go siem-api/internal/api/ingest_health_test.go siem-api/internal/api/server.go siem-api/cmd
git commit -m "Add GET /sources/ingest-health backed by Vector's GraphQL API"
```

---

### Task 4: siem-web — `SiemApiClient` additions

**Files:**
- Modify: `siem-web/src/lib/server/siemApiClient.ts`
- Modify: `siem-web/src/lib/server/siemApiClient.test.ts`

**Interfaces:**
- Produces: `SourceResponse`, `IngestHealthResponse` types; `SiemApiClient.getSources`,
  `getIngestHealth`, `claimSource` methods, following the exact shape of the existing
  `getAlerts`/`getRules`/`muteAlert` methods in this file (see Task 5/6/7 for consumers).

- [ ] **Step 1: Write the failing tests**

In `siem-web/src/lib/server/siemApiClient.test.ts`, add:

```ts
it('getSources attaches Authorization and parses the response', async () => {
	const fetchFn = fakeFetch([
		{
			id: 1,
			name: 'udm-ultra',
			address: '10.0.0.1',
			transport: 'udp/514',
			parser: 'unifi-os',
			claimed: true,
			heartbeat_sec: 900,
			status: 'healthy',
			events_per_min: 12
		}
	]);
	const client = new SiemApiClient({ baseUrl: 'http://siem-api:8080' }, fetchFn);

	const result = await client.getSources('token-123');

	expect(result).toHaveLength(1);
	expect(result[0].status).toBe('healthy');
	const [url, init] = fetchFn.mock.calls[0];
	expect(url).toBe('http://siem-api:8080/sources');
	expect((init?.headers as Record<string, string>).Authorization).toBe('Bearer token-123');
});

it('getIngestHealth attaches Authorization and parses the response', async () => {
	const fetchFn = fakeFetch({
		received_events_per_source: { unifi: 1234 },
		loki_sent_events_total: 1290,
		degraded: false
	});
	const client = new SiemApiClient({ baseUrl: 'http://siem-api:8080' }, fetchFn);

	const result = await client.getIngestHealth('token-123');

	expect(result.loki_sent_events_total).toBe(1290);
	const [url] = fetchFn.mock.calls[0];
	expect(url).toBe('http://siem-api:8080/sources/ingest-health');
});

it('claimSource POSTs to the claim endpoint with Authorization', async () => {
	const fetchFn = fakeFetch(null, 204);
	const client = new SiemApiClient({ baseUrl: 'http://siem-api:8080' }, fetchFn);

	await client.claimSource('token-123', 7);

	const [url, init] = fetchFn.mock.calls[0];
	expect(url).toBe('http://siem-api:8080/sources/7/claim');
	expect(init?.method).toBe('POST');
	expect((init?.headers as Record<string, string>).Authorization).toBe('Bearer token-123');
});
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd siem-web && npm run test:unit -- --run siemApiClient`
Expected: FAIL — `client.getSources is not a function` (and similarly for the other two).

- [ ] **Step 3: Implement the client additions**

In `siem-web/src/lib/server/siemApiClient.ts`, add near the other response interfaces
(after `SearchResponse`):

```ts
export interface SourceResponse {
	id: number;
	name: string;
	address: string;
	transport: string;
	parser: string;
	claimed: boolean;
	heartbeat_sec: number;
	last_seen_at?: string;
	status: 'healthy' | 'silent';
	events_per_min: number;
}

export interface IngestHealthResponse {
	received_events_per_source: Record<string, number>;
	loki_sent_events_total: number;
	degraded: boolean;
}
```

Add to the `SiemApiClient` class (after `getRules`):

```ts
	async getSources(sessionToken: string): Promise<SourceResponse[]> {
		return this.request<SourceResponse[]>('/sources', this.authInit(sessionToken));
	}

	async getIngestHealth(sessionToken: string): Promise<IngestHealthResponse> {
		return this.request<IngestHealthResponse>('/sources/ingest-health', this.authInit(sessionToken));
	}

	async claimSource(sessionToken: string, id: number): Promise<void> {
		return this.requestNoContent(`/sources/${id}/claim`, {
			method: 'POST',
			...this.authInit(sessionToken)
		});
	}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd siem-web && npm run test:unit -- --run siemApiClient`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add siem-web/src/lib/server/siemApiClient.ts siem-web/src/lib/server/siemApiClient.test.ts
git commit -m "Add getSources, getIngestHealth, claimSource to SiemApiClient"
```

---

### Task 5: siem-web — `sources.ts` data-shaping helpers

**Files:**
- Create: `siem-web/src/lib/sources.ts`
- Create: `siem-web/src/lib/sources.test.ts`

**Interfaces:**
- Consumes: `SourceResponse` from `Task 4`.
- Produces: `splitClaimedUnclaimed(sources)`, `formatEventsPerMin(n)`,
  `formatLastSeen(iso)` — consumed by `Task 7`'s load function and `Task 8`/`9`'s
  components.

- [ ] **Step 1: Write the failing tests**

Create `siem-web/src/lib/sources.test.ts`:

```ts
import { describe, it, expect } from 'vitest';
import { splitClaimedUnclaimed, formatEventsPerMin, formatLastSeen } from './sources';
import type { SourceResponse } from './server/siemApiClient';

function fakeSource(overrides: Partial<SourceResponse> = {}): SourceResponse {
	return {
		id: 1,
		name: 'udm-ultra',
		address: '10.0.0.1',
		transport: 'udp/514',
		parser: 'unifi-os',
		claimed: true,
		heartbeat_sec: 900,
		status: 'healthy',
		events_per_min: 0,
		...overrides
	};
}

describe('splitClaimedUnclaimed', () => {
	it('splits sources by claimed status', () => {
		const claimed = fakeSource({ id: 1, claimed: true });
		const unclaimed = fakeSource({ id: 2, claimed: false });

		const result = splitClaimedUnclaimed([claimed, unclaimed]);

		expect(result.claimed).toEqual([claimed]);
		expect(result.unclaimed).toEqual([unclaimed]);
	});
});

describe('formatEventsPerMin', () => {
	it('rounds values of 1 or more to the nearest integer', () => {
		expect(formatEventsPerMin(12.6)).toBe('13');
	});

	it('shows one decimal place for sub-1 rates instead of rounding to 0', () => {
		expect(formatEventsPerMin(0.4)).toBe('0.4');
	});
});

describe('formatLastSeen', () => {
	it('returns "never" when last_seen_at is undefined', () => {
		expect(formatLastSeen(undefined)).toBe('never');
	});

	it('formats a recent timestamp in minutes', () => {
		const iso = new Date(Date.now() - 5 * 60_000).toISOString();
		expect(formatLastSeen(iso)).toBe('5m ago');
	});

	it('formats an older timestamp in hours', () => {
		const iso = new Date(Date.now() - 3 * 3_600_000).toISOString();
		expect(formatLastSeen(iso)).toBe('3h ago');
	});
});
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd siem-web && npm run test:unit -- --run sources.test`
Expected: FAIL — `Cannot find module './sources'`.

- [ ] **Step 3: Implement the helpers**

Create `siem-web/src/lib/sources.ts`:

```ts
import type { SourceResponse } from './server/siemApiClient';

export function splitClaimedUnclaimed(sources: SourceResponse[]): {
	claimed: SourceResponse[];
	unclaimed: SourceResponse[];
} {
	return {
		claimed: sources.filter((s) => s.claimed),
		unclaimed: sources.filter((s) => !s.claimed)
	};
}

export function formatEventsPerMin(eventsPerMin: number): string {
	if (eventsPerMin < 1) return eventsPerMin.toFixed(1);
	return Math.round(eventsPerMin).toString();
}

export function formatLastSeen(lastSeenAt: string | undefined): string {
	if (!lastSeenAt) return 'never';
	const minutes = Math.floor((Date.now() - new Date(lastSeenAt).getTime()) / 60_000);
	if (minutes < 1) return 'just now';
	if (minutes < 60) return `${minutes}m ago`;
	const hours = Math.floor(minutes / 60);
	if (hours < 24) return `${hours}h ago`;
	return `${Math.floor(hours / 24)}d ago`;
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd siem-web && npm run test:unit -- --run sources.test`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add siem-web/src/lib/sources.ts siem-web/src/lib/sources.test.ts
git commit -m "Add sources.ts data-shaping helpers"
```

---

### Task 6: siem-web — claim proxy route

**Files:**
- Create: `siem-web/src/routes/api/sources/[id]/claim/+server.ts`
- Create: `siem-web/src/routes/api/sources/[id]/claim/server.test.ts`

**Interfaces:**
- Consumes: `Task 4`'s `SiemApiClient.claimSource`.
- Produces: `POST /api/sources/{id}/claim`, consumed by `Task 9`'s `UnclaimedSenders.svelte`.

- [ ] **Step 1: Write the failing test**

Create `siem-web/src/routes/api/sources/[id]/claim/server.test.ts` (mirrors the existing
`routes/api/alerts/[id]/mute/server.test.ts` exactly):

```ts
import { describe, it, expect, vi } from 'vitest';
import { POST } from './+server';
import * as siemApiClientModule from '$lib/server/siemApiClient';

vi.mock('$env/dynamic/private', () => ({ env: { API_URL: 'http://siem-api:8080' } }));

vi.mock('$lib/server/siemApiClient', async (importOriginal) => {
	const actual = await importOriginal<typeof import('$lib/server/siemApiClient')>();
	return { ...actual, SiemApiClient: vi.fn() };
});

describe('POST /api/sources/[id]/claim', () => {
	it('calls claimSource with the session token and returns 204', async () => {
		const claimSourceMock = vi.fn().mockResolvedValue(undefined);
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return { claimSource: claimSourceMock };
		});

		const response = await POST({
			params: { id: '7' },
			locals: { sessionToken: 'token-123' }
		} as never);

		expect(claimSourceMock).toHaveBeenCalledWith('token-123', 7);
		expect(response.status).toBe(204);
	});

	it('propagates a SiemApiError status code as a JSON error response', async () => {
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return {
				claimSource: vi.fn().mockRejectedValue(new siemApiClientModule.SiemApiError(403, 'denied'))
			};
		});

		const response = await POST({
			params: { id: '7' },
			locals: { sessionToken: 'token-123' }
		} as never);

		expect(response.status).toBe(403);
	});
});
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd siem-web && npm run test:unit -- --run routes/api/sources`
Expected: FAIL — `Cannot find module './+server'`.

- [ ] **Step 3: Implement the route**

Create `siem-web/src/routes/api/sources/[id]/claim/+server.ts` (mirrors
`routes/api/alerts/[id]/mute/+server.ts` exactly, swapping `muteAlert` for `claimSource`):

```ts
import { json } from '@sveltejs/kit';
import { env } from '$env/dynamic/private';
import type { RequestHandler } from './$types';
import { SiemApiClient, SiemApiError } from '$lib/server/siemApiClient';

export const POST: RequestHandler = async ({ params, locals }) => {
	const client = new SiemApiClient({ baseUrl: env.API_URL as string });
	const token = locals.sessionToken as string;
	const id = Number(params.id);

	try {
		await client.claimSource(token, id);
	} catch (err) {
		if (err instanceof SiemApiError) {
			return json({ error: err.message }, { status: err.status });
		}
		throw err;
	}

	return new Response(null, { status: 204 });
};
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd siem-web && npm run test:unit -- --run routes/api/sources`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add siem-web/src/routes/api/sources
git commit -m "Add POST /api/sources/[id]/claim proxy route"
```

---

### Task 7: siem-web — `/sources` load function

**Files:**
- Create: `siem-web/src/routes/sources/+page.server.ts`
- Create: `siem-web/src/routes/sources/page.server.test.ts`

**Interfaces:**
- Consumes: `Task 4`'s `getSources`/`getIngestHealth`/`search`, `Task 5`'s
  `splitClaimedUnclaimed`.
- Produces: the `PageData` shape `Task 10`'s `+page.svelte` renders: `{ sources,
  claimedSources, unclaimedSources, previewName, previewSample, health }`.

- [ ] **Step 1: Write the failing tests**

Create `siem-web/src/routes/sources/page.server.test.ts` (mirrors
`routes/alerts/page.server.test.ts`'s mocking setup):

```ts
import { describe, it, expect, vi } from 'vitest';
import { load } from './+page.server';
import * as siemApiClientModule from '$lib/server/siemApiClient';
import { SiemApiError } from '$lib/server/siemApiClient';

vi.mock('$env/dynamic/private', () => ({ env: { API_URL: 'http://siem-api:8080' } }));

vi.mock('$lib/server/siemApiClient', async (importOriginal) => {
	const actual = await importOriginal<typeof import('$lib/server/siemApiClient')>();
	return { ...actual, SiemApiClient: vi.fn() };
});

function fakeSource(overrides: Record<string, unknown> = {}) {
	return {
		id: 1,
		name: 'udm-ultra',
		address: '10.0.0.1',
		transport: 'udp/514',
		parser: 'unifi-os',
		claimed: true,
		heartbeat_sec: 900,
		status: 'healthy',
		events_per_min: 5,
		...overrides
	};
}

function fakeHealth() {
	return { received_events_per_source: {}, loki_sent_events_total: 0, degraded: false };
}

describe('Sources load', () => {
	it('loads sources, health, and a preview sample for the first source by default', async () => {
		const searchMock = vi.fn().mockResolvedValue({ logql: '', count: 1, entries: [{ Line: '{}' }] });
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return {
				getSources: vi.fn().mockResolvedValue([fakeSource({ name: 'udm-ultra' })]),
				getIngestHealth: vi.fn().mockResolvedValue(fakeHealth()),
				search: searchMock
			};
		});

		const result = (await load({
			locals: { sessionToken: 'token-123' },
			url: new URL('https://siem.townsville.cc/sources')
		} as never)) as Exclude<Awaited<ReturnType<typeof load>>, void>;

		expect(result.previewName).toBe('udm-ultra');
		expect(result.previewSample).toEqual({ Line: '{}' });
		expect(searchMock).toHaveBeenCalledWith('token-123', { source: 'udm-ultra', limit: '1' });
	});

	it('uses the ?preview= query param over the first source when given', async () => {
		const searchMock = vi.fn().mockResolvedValue({ logql: '', count: 0, entries: [] });
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return {
				getSources: vi.fn().mockResolvedValue([
					fakeSource({ id: 1, name: 'udm-ultra' }),
					fakeSource({ id: 2, name: 'host-1' })
				]),
				getIngestHealth: vi.fn().mockResolvedValue(fakeHealth()),
				search: searchMock
			};
		});

		const result = (await load({
			locals: { sessionToken: 'token-123' },
			url: new URL('https://siem.townsville.cc/sources?preview=host-1')
		} as never)) as Exclude<Awaited<ReturnType<typeof load>>, void>;

		expect(result.previewName).toBe('host-1');
		expect(searchMock).toHaveBeenCalledWith('token-123', { source: 'host-1', limit: '1' });
	});

	it('splits sources into claimed and unclaimed', async () => {
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return {
				getSources: vi
					.fn()
					.mockResolvedValue([
						fakeSource({ id: 1, name: 'a', claimed: true }),
						fakeSource({ id: 2, name: 'b', claimed: false })
					]),
				getIngestHealth: vi.fn().mockResolvedValue(fakeHealth()),
				search: vi.fn().mockResolvedValue({ logql: '', count: 0, entries: [] })
			};
		});

		const result = (await load({
			locals: { sessionToken: 'token-123' },
			url: new URL('https://siem.townsville.cc/sources')
		} as never)) as Exclude<Awaited<ReturnType<typeof load>>, void>;

		expect(result.claimedSources).toHaveLength(1);
		expect(result.unclaimedSources).toHaveLength(1);
	});

	it('degrades the preview sample to null instead of failing the page on a search error', async () => {
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return {
				getSources: vi.fn().mockResolvedValue([fakeSource()]),
				getIngestHealth: vi.fn().mockResolvedValue(fakeHealth()),
				search: vi.fn().mockRejectedValue(new SiemApiError(502, 'loki down'))
			};
		});

		const result = (await load({
			locals: { sessionToken: 'token-123' },
			url: new URL('https://siem.townsville.cc/sources')
		} as never)) as Exclude<Awaited<ReturnType<typeof load>>, void>;

		expect(result.previewSample).toBeNull();
		expect(result.sources).toHaveLength(1);
	});

	it('redirects to /auth/logout on a 401/403 from the primary sources fetch', async () => {
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return {
				getSources: vi.fn().mockRejectedValue(new SiemApiError(401, 'invalid session')),
				getIngestHealth: vi.fn().mockResolvedValue(fakeHealth()),
				search: vi.fn()
			};
		});

		await expect(
			load({
				locals: { sessionToken: 'stale-token' },
				url: new URL('https://siem.townsville.cc/sources')
			} as never)
		).rejects.toMatchObject({ status: 302, location: '/auth/logout' });
	});

	it('surfaces a 502 when siem-api fails for a reason other than auth', async () => {
		vi.mocked(siemApiClientModule.SiemApiClient).mockImplementation(function () {
			return {
				getSources: vi.fn().mockRejectedValue(new SiemApiError(500, 'boom')),
				getIngestHealth: vi.fn().mockResolvedValue(fakeHealth()),
				search: vi.fn()
			};
		});

		await expect(
			load({
				locals: { sessionToken: 'token-123' },
				url: new URL('https://siem.townsville.cc/sources')
			} as never)
		).rejects.toMatchObject({ status: 502 });
	});
});
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd siem-web && npm run test:unit -- --run routes/sources`
Expected: FAIL — `Cannot find module './+page.server'`.

- [ ] **Step 3: Implement the load function**

Create `siem-web/src/routes/sources/+page.server.ts`:

```ts
import { redirect, error } from '@sveltejs/kit';
import { env } from '$env/dynamic/private';
import type { PageServerLoad } from './$types';
import { SiemApiClient, SiemApiError } from '$lib/server/siemApiClient';
import { splitClaimedUnclaimed } from '$lib/sources';

export const load: PageServerLoad = async ({ locals, url }) => {
	const client = new SiemApiClient({ baseUrl: env.API_URL as string });
	const token = locals.sessionToken as string;

	let sources, health;
	try {
		[sources, health] = await Promise.all([client.getSources(token), client.getIngestHealth(token)]);
	} catch (err) {
		if (err instanceof SiemApiError) {
			if (err.status === 401 || err.status === 403) {
				redirect(302, '/auth/logout');
			}
			error(502, 'siem-api unavailable');
		}
		throw err;
	}

	const previewName = url.searchParams.get('preview') ?? sources[0]?.name ?? null;

	let previewSample = null;
	if (previewName) {
		try {
			const result = await client.search(token, { source: previewName, limit: '1' });
			previewSample = result.entries[0] ?? null;
		} catch (err) {
			// Parser preview is supplementary (per design spec) — a Loki hiccup
			// here shouldn't take down the whole Sources screen.
			if (err instanceof SiemApiError && (err.status === 401 || err.status === 403)) {
				redirect(302, '/auth/logout');
			}
			previewSample = null;
		}
	}

	const { claimed, unclaimed } = splitClaimedUnclaimed(sources);

	return {
		sources,
		claimedSources: claimed,
		unclaimedSources: unclaimed,
		previewName,
		previewSample,
		health
	};
};
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd siem-web && npm run test:unit -- --run routes/sources`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add siem-web/src/routes/sources/+page.server.ts siem-web/src/routes/sources/page.server.test.ts
git commit -m "Add /sources load function"
```

---

### Task 8: siem-web — `SourcesTable.svelte` and `ParserPreview.svelte`

**Files:**
- Create: `siem-web/src/lib/components/SourcesTable.svelte`
- Create: `siem-web/src/lib/components/ParserPreview.svelte`

**Interfaces:**
- Consumes: `SourceResponse` (Task 4), `LogEntry` (existing, from `siemApiClient.ts`),
  `formatEventsPerMin`/`formatLastSeen` (Task 5).
- Produces: both consumed by `Task 10`'s `+page.svelte`. No unit tests — presentational
  components, per this project's established testing split (Wall/Alerts also skip
  component tests).

- [ ] **Step 1: Implement `SourcesTable.svelte`**

Create `siem-web/src/lib/components/SourcesTable.svelte`:

```svelte
<script lang="ts">
	import { resolve } from '$app/paths';
	import type { SourceResponse } from '$lib/server/siemApiClient';
	import { formatEventsPerMin, formatLastSeen } from '$lib/sources';

	let { sources, selectedName }: { sources: SourceResponse[]; selectedName: string | null } =
		$props();
</script>

<table class="sources">
	<thead>
		<tr>
			<th>Source</th>
			<th>Address</th>
			<th>Transport</th>
			<th>Parser</th>
			<th class="num">Events/min</th>
			<th>Last seen</th>
			<th>Health</th>
		</tr>
	</thead>
	<tbody>
		{#each sources as source (source.id)}
			<tr class:selected={source.name === selectedName}>
				<td
					><a class="name-link" href={resolve(`/sources?preview=${source.name}`)}
						>{source.name}</a
					></td
				>
				<td class="mono">{source.address}</td>
				<td class="mono">{source.transport}</td>
				<td><span class="tag">{source.parser}</span></td>
				<td class="num mono">{formatEventsPerMin(source.events_per_min)}</td>
				<td class="mono">{formatLastSeen(source.last_seen_at)}</td>
				<td>
					<span class="health health-{source.status}">
						<span class="dot"></span>{source.status}
					</span>
				</td>
			</tr>
		{/each}
	</tbody>
</table>

<style>
	.sources {
		width: 100%;
		border-collapse: collapse;
		font-size: var(--text-table);
		line-height: var(--line-height-dense-table);
	}
	thead th {
		text-align: left;
		font-size: var(--text-eyebrow);
		text-transform: uppercase;
		letter-spacing: 0.06em;
		color: var(--color-muted-2);
		padding: var(--space-2) var(--space-3);
		border-bottom: 1px solid var(--color-line);
	}
	tbody td {
		padding: var(--space-2) var(--space-3);
		border-bottom: 1px solid var(--color-line);
	}
	tbody tr.selected {
		background: var(--row-selected-bg);
	}
	tbody tr:hover {
		background: var(--row-hover-bg);
	}
	.num {
		text-align: right;
	}
	.mono {
		font-family: var(--font-mono);
		color: var(--color-text-3);
	}
	.name-link {
		color: var(--color-text);
		text-decoration: none;
		font-weight: 500;
	}
	.name-link:hover {
		color: var(--color-accent-light);
	}
	.tag {
		display: inline-block;
		font-family: var(--font-mono);
		font-size: var(--text-label);
		background: var(--color-surface-3);
		border-radius: var(--radius-sm);
		padding: 1px var(--space-2);
		color: var(--color-muted);
	}
	.health {
		display: inline-flex;
		align-items: center;
		gap: var(--space-1);
		text-transform: capitalize;
	}
	.dot {
		width: 7px;
		height: 7px;
		border-radius: 50%;
		background: currentColor;
	}
	.health-healthy {
		color: var(--color-severity-healthy);
	}
	.health-silent {
		color: var(--color-severity-warning);
	}
</style>
```

- [ ] **Step 2: Implement `ParserPreview.svelte`**

Create `siem-web/src/lib/components/ParserPreview.svelte`:

```svelte
<script lang="ts">
	import type { LogEntry } from '$lib/server/siemApiClient';

	let { sourceName, sample }: { sourceName: string | null; sample: LogEntry | null } = $props();

	function parsedFields(line: string): [string, string][] {
		try {
			const parsed = JSON.parse(line);
			if (typeof parsed !== 'object' || parsed === null) return [];
			return Object.entries(parsed).map(([k, v]) => [k, JSON.stringify(v)]);
		} catch {
			return [];
		}
	}
</script>

<section class="preview">
	<h2>Parser preview{sourceName ? ` — ${sourceName}` : ''}</h2>
	{#if !sample}
		<p class="empty">No recent events from this source yet.</p>
	{:else}
		<div class="cards">
			<div class="card">
				<div class="card-label">Raw line</div>
				<pre class="mono">{sample.Line}</pre>
			</div>
			<div class="card">
				<div class="card-label">Extracted fields</div>
				<dl class="mono">
					{#each parsedFields(sample.Line) as [key, value] (key)}
						<dt>{key}</dt>
						<dd>{value}</dd>
					{/each}
				</dl>
			</div>
		</div>
	{/if}
</section>

<style>
	.preview {
		margin-top: var(--space-5);
	}
	h2 {
		font-size: var(--text-section-head);
		color: var(--color-muted);
		margin: 0 0 var(--space-3);
	}
	.empty {
		color: var(--color-muted-2);
		font-size: var(--text-body);
	}
	.cards {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: var(--space-4);
	}
	.card {
		background: var(--color-surface-2);
		border-radius: var(--radius-default);
		padding: var(--space-4);
		overflow: auto;
	}
	.card-label {
		font-size: var(--text-eyebrow);
		text-transform: uppercase;
		color: var(--color-muted-2);
		margin-bottom: var(--space-2);
	}
	.mono {
		font-family: var(--font-mono);
		font-size: var(--text-log-row);
	}
	pre.mono {
		white-space: pre-wrap;
		word-break: break-word;
		margin: 0;
	}
	dl.mono {
		margin: 0;
		display: grid;
		grid-template-columns: auto 1fr;
		gap: var(--space-1) var(--space-3);
	}
	dt {
		color: var(--color-muted);
	}
	dd {
		margin: 0;
		color: var(--color-text-2);
		overflow-wrap: anywhere;
	}
</style>
```

- [ ] **Step 3: Typecheck**

Run: `cd siem-web && npm run check`
Expected: no new errors from these two files.

- [ ] **Step 4: Commit**

```bash
git add siem-web/src/lib/components/SourcesTable.svelte siem-web/src/lib/components/ParserPreview.svelte
git commit -m "Add SourcesTable and ParserPreview components"
```

---

### Task 9: siem-web — `IngestHealthPanel.svelte` and `UnclaimedSenders.svelte`

**Files:**
- Create: `siem-web/src/lib/components/IngestHealthPanel.svelte`
- Create: `siem-web/src/lib/components/UnclaimedSenders.svelte`

**Interfaces:**
- Consumes: `IngestHealthResponse` (Task 4), `SourceResponse` (Task 4).
- Produces: both consumed by `Task 10`'s `+page.svelte`. `UnclaimedSenders.svelte` POSTs
  to `Task 6`'s `/api/sources/{id}/claim` route directly via `fetch` (same pattern
  `TriageCard.svelte` already uses for its "Mute 1h" button — check that file for the
  exact `fetch`/error-handling shape before writing this step, since it's the existing
  precedent for a component-level optimistic POST).

- [ ] **Step 1: Implement `IngestHealthPanel.svelte`**

Create `siem-web/src/lib/components/IngestHealthPanel.svelte`:

```svelte
<script lang="ts">
	import type { IngestHealthResponse } from '$lib/server/siemApiClient';

	let { health }: { health: IngestHealthResponse } = $props();
</script>

<section class="panel">
	<h2>Ingest health</h2>
	{#if health.degraded}
		<p class="degraded">Ingest metrics unavailable — siem-ingest's API is unreachable.</p>
	{:else}
		<dl>
			{#each Object.entries(health.received_events_per_source) as [source, total] (source)}
				<div class="row">
					<dt>{source} received</dt>
					<dd class="mono">{total}</dd>
				</div>
			{/each}
			<div class="row">
				<dt>Loki sent</dt>
				<dd class="mono">{health.loki_sent_events_total}</dd>
			</div>
		</dl>
	{/if}
</section>

<style>
	.panel {
		background: var(--color-surface-2);
		border-radius: var(--radius-default);
		padding: var(--space-4);
	}
	h2 {
		font-size: var(--text-section-head);
		color: var(--color-muted);
		margin: 0 0 var(--space-3);
	}
	.degraded {
		font-size: var(--text-table);
		color: var(--color-severity-warning);
		margin: 0;
	}
	dl {
		margin: 0;
	}
	.row {
		display: flex;
		justify-content: space-between;
		font-size: var(--text-table);
		padding: var(--space-1) 0;
	}
	.row dt {
		color: var(--color-muted);
		text-transform: capitalize;
	}
	.mono {
		font-family: var(--font-mono);
		color: var(--color-text-2);
	}
</style>
```

- [ ] **Step 2: Implement `UnclaimedSenders.svelte`**

Create `siem-web/src/lib/components/UnclaimedSenders.svelte`. Base the claim button's
`fetch`/optimistic-removal logic on `TriageCard.svelte`'s existing "Mute 1h" handler —
read that file first so the error-handling shape (and whether it uses `$state`/local
array mutation or an event callback to the parent) matches exactly rather than
introducing a second pattern for the same kind of action:

```svelte
<script lang="ts">
	import { resolve } from '$app/paths';
	import type { SourceResponse } from '$lib/server/siemApiClient';

	let {
		sources,
		canClaim
	}: {
		sources: SourceResponse[];
		canClaim: boolean;
	} = $props();

	let claimed = $state(new Set<number>());
	let pending = $state(new Set<number>());

	async function claim(id: number) {
		pending.add(id);
		pending = new Set(pending);
		try {
			const res = await fetch(resolve(`/api/sources/${id}/claim`), { method: 'POST' });
			if (res.ok) {
				claimed.add(id);
				claimed = new Set(claimed);
			}
		} finally {
			pending.delete(id);
			pending = new Set(pending);
		}
	}
</script>

<section class="panel">
	<h2>Unclaimed senders</h2>
	{#if sources.filter((s) => !claimed.has(s.id)).length === 0}
		<p class="empty">Every known sender has been claimed.</p>
	{:else}
		<ul>
			{#each sources.filter((s) => !claimed.has(s.id)) as source (source.id)}
				<li>
					<div>
						<div class="name">{source.name}</div>
						<div class="mono meta">{source.address} · {source.transport}</div>
					</div>
					{#if canClaim}
						<button onclick={() => claim(source.id)} disabled={pending.has(source.id)}>
							{pending.has(source.id) ? 'Claiming…' : 'Claim'}
						</button>
					{/if}
				</li>
			{/each}
		</ul>
	{/if}
</section>

<style>
	.panel {
		background: var(--color-surface-2);
		border-radius: var(--radius-default);
		padding: var(--space-4);
		margin-top: var(--space-4);
	}
	h2 {
		font-size: var(--text-section-head);
		color: var(--color-muted);
		margin: 0 0 var(--space-3);
	}
	.empty {
		font-size: var(--text-table);
		color: var(--color-muted-2);
		margin: 0;
	}
	ul {
		list-style: none;
		margin: 0;
		padding: 0;
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
	}
	li {
		display: flex;
		justify-content: space-between;
		align-items: center;
		font-size: var(--text-table);
	}
	.name {
		font-weight: 500;
	}
	.meta {
		color: var(--color-muted-2);
		font-size: var(--text-label);
	}
	button {
		background: var(--color-accent-tint-2);
		color: var(--color-accent-lighter);
		border: none;
		border-radius: var(--radius-sm);
		padding: var(--space-1) var(--space-3);
		font-size: var(--text-label);
		cursor: pointer;
	}
	button:disabled {
		opacity: 0.6;
		cursor: default;
	}
</style>
```

- [ ] **Step 3: Typecheck**

Run: `cd siem-web && npm run check`
Expected: no new errors from these two files.

- [ ] **Step 4: Commit**

```bash
git add siem-web/src/lib/components/IngestHealthPanel.svelte siem-web/src/lib/components/UnclaimedSenders.svelte
git commit -m "Add IngestHealthPanel and UnclaimedSenders components"
```

---

### Task 10: siem-web — `/sources` page assembly

**Files:**
- Create: `siem-web/src/routes/sources/+page.svelte`
- Modify: `siem-web/src/lib/components/Nav.svelte`

**Interfaces:**
- Consumes: `Task 7`'s `PageData`, `Task 8`/`9`'s four components.

- [ ] **Step 1: Implement the page**

Create `siem-web/src/routes/sources/+page.svelte`:

```svelte
<script lang="ts">
	import SourcesTable from '$lib/components/SourcesTable.svelte';
	import ParserPreview from '$lib/components/ParserPreview.svelte';
	import IngestHealthPanel from '$lib/components/IngestHealthPanel.svelte';
	import UnclaimedSenders from '$lib/components/UnclaimedSenders.svelte';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();
</script>

<div class="sources-screen">
	<div class="main">
		<SourcesTable sources={data.sources} selectedName={data.previewName} />
		<ParserPreview sourceName={data.previewName} sample={data.previewSample} />
	</div>
	<aside class="rail">
		<section class="panel">
			<h2>Point a device here</h2>
			<p class="body">
				UniFi: <strong>Settings → System → Advanced → Remote Logging</strong>. Point it at:
			</p>
			<dl class="mono">
				<div class="row"><dt>Syslog (UDP)</dt><dd>514</dd></div>
				<div class="row"><dt>Syslog (TCP)</dt><dd>601</dd></div>
				<div class="row"><dt>Syslog (TLS)</dt><dd>6514</dd></div>
			</dl>
		</section>
		<IngestHealthPanel health={data.health} />
		<UnclaimedSenders sources={data.unclaimedSources} canClaim={true} />
	</aside>
</div>

<style>
	.sources-screen {
		display: flex;
		gap: var(--space-6);
		padding: var(--space-5) var(--space-6);
		align-items: flex-start;
	}
	.main {
		flex: 1 1 auto;
		min-width: 0;
	}
	.rail {
		flex: 0 0 260px;
		display: flex;
		flex-direction: column;
		gap: var(--space-4);
	}
	.panel {
		background: var(--color-surface-2);
		border-radius: var(--radius-default);
		padding: var(--space-4);
	}
	h2 {
		font-size: var(--text-section-head);
		color: var(--color-muted);
		margin: 0 0 var(--space-3);
	}
	.body {
		font-size: var(--text-table);
		color: var(--color-text-3);
		margin: 0 0 var(--space-3);
	}
	dl.mono {
		margin: 0;
	}
	.row {
		display: flex;
		justify-content: space-between;
		font-family: var(--font-mono);
		font-size: var(--text-table);
		padding: var(--space-1) 0;
		color: var(--color-text-2);
	}
	.row dt {
		color: var(--color-muted);
	}
</style>
```

`canClaim={true}` is a known simplification — see Step 3, which threads the real role
through before this task is done.

- [ ] **Step 2: Confirm role plumbing to the page**

`+layout.server.ts` already exposes `locals.user.role` to every route (it's how
`Nav.svelte` gets `userRole` today — check `routes/+layout.server.ts` and
`routes/+layout.svelte` to see exactly how). `/sources` needs the same value available in
its own `PageData` to gate the Claim button, since `+page.server.ts` (Task 7) doesn't
currently return it. Add one line to Task 7's `load()` return object:

```ts
		userRole: locals.user?.role,
```

(place it in the returned object alongside `sources`, `health`, etc. — re-open
`siem-web/src/routes/sources/+page.server.ts` from Task 7 and add this field; no new test
required beyond what Task 7 already covers, since this is a passthrough of a value
`locals` already provides, matching how `+layout.server.ts` does the identical passthrough
today).

- [ ] **Step 3: Wire the real role into `canClaim`**

Update `+page.svelte`'s `UnclaimedSenders` usage:

```svelte
<UnclaimedSenders sources={data.unclaimedSources} canClaim={data.userRole === 'admin'} />
```

- [ ] **Step 4: Drop the now-unnecessary `Pathname` assertion in `Nav.svelte`**

In `siem-web/src/lib/components/Nav.svelte`, the `/sources` route now exists, so its
entry no longer needs the forward-declaration cast — the file's own comment says "drop
each assertion as its route lands." Change:

```ts
		{ label: 'Sources', href: '/sources' as Pathname },
```

to:

```ts
		{ label: 'Sources', href: '/sources' },
```

- [ ] **Step 5: Typecheck and run the full test suite**

Run: `cd siem-web && npm run check && npm run test:unit -- --run`
Expected: no new type errors; all tests (existing + this plan's) pass.

- [ ] **Step 6: Manual verification**

Run `cd siem-web && npm run dev`, sign in, and visit `/sources`. Confirm:
- The table renders with Source/Address/Transport/Parser/Events-min/Last-seen/Health
  columns.
- Clicking a different row's name updates the URL to `?preview=<name>` and the parser
  preview panel below the table.
- The right rail shows the static UniFi instructions, the ingest-health panel (will read
  `degraded: true` unless a real siem-ingest is also running — that's expected in a bare
  siem-web dev server), and the unclaimed-senders list.
- The Sources nav link is no longer a dead link.

Since this environment likely has no real siem-api/siem-ingest running, this step may only
be able to confirm the page doesn't crash on the `error(502, ...)` path — note in the task
report whichever is actually observed.

- [ ] **Step 7: Commit**

```bash
git add siem-web/src/routes/sources/+page.svelte siem-web/src/routes/sources/+page.server.ts siem-web/src/lib/components/Nav.svelte
git commit -m "Assemble the Sources screen page and wire the nav link"
```
