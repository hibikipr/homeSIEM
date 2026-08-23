package api

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

// fakeLokiSearchServer is the shared fake Loki backend for the search
// endpoint's tests: it discriminates real Loki's three request shapes by
// HTTP path and query text, the same way a real deployment would receive
// distinct requests for entries, the total count, volume buckets, and
// per-label facet counts, all of which the search handler now issues
// concurrently on every request.
//
//   - GET /loki/api/v1/query_range with a raw (non-aggregate) LogQL query
//     is the entries request.
//   - GET /loki/api/v1/query_range with a `count_over_time` aggregate is
//     the volume-bucket matrix request.
//   - GET /loki/api/v1/query with `sum(count_over_time(...))` (no `by`) is
//     the total-count instant request.
//   - GET /loki/api/v1/query with `sum by (...) (count_over_time(...))` is
//     a per-label facet instant request.
func fakeLokiSearchServer(t *testing.T, entriesJSON, countJSON, volumeJSON, facetJSON string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("query")
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/loki/api/v1/query" && strings.Contains(query, "sum by ("):
			w.Write([]byte(facetJSON))
		case r.URL.Path == "/loki/api/v1/query" && strings.HasPrefix(query, "sum(count_over_time"):
			w.Write([]byte(countJSON))
		case r.URL.Path == "/loki/api/v1/query_range" && strings.Contains(query, "count_over_time"):
			w.Write([]byte(volumeJSON))
		default:
			w.Write([]byte(entriesJSON))
		}
	}))
}

const emptyInstantResult = `{"status":"success","data":{"result":[]}}`
const emptyMatrixResult = `{"status":"success","data":{"resultType":"matrix","result":[]}}`

func TestEventsSearch_ReturnsCompiledQueryAndEntries(t *testing.T) {
	entriesJSON := `{"status":"success","data":{"result":[
            {"stream":{"job":"siem","source":"udm-ultra"},"values":[["1700000000000000000","hello"]]}
        ]}}`
	countJSON := `{"status":"success","data":{"result":[
			{"metric":{},"value":[1700000000,"1"]}
		]}}`
	fakeLoki := fakeLokiSearchServer(t, entriesJSON, countJSON, emptyMatrixResult, emptyInstantResult)
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
	if resp.LogQL != `{job="siem",source="udm-ultra",internal_noise!="true"}` {
		t.Errorf("LogQL = %q", resp.LogQL)
	}
	if resp.Count != 1 || len(resp.Entries) != 1 || resp.Entries[0].Line != "hello" {
		t.Errorf("resp = %+v", resp)
	}
}

// TestEventsSearch_IncludesVolumeBuckets covers an unfiltered/lightly-filtered
// search whose LogQL selector matches multiple distinct Loki streams (e.g. two
// different `source` label sets under the same job). Real Loki would return
// one matrix series per stream in that case, so the fake server here mirrors
// that: a bare (unsummed) count_over_time query gets back two separate
// series that must be combined by the server, while a sum(count_over_time(...))
// query — which is what real Loki would already have reduced to one series
// server-side — gets back the pre-summed single series. This lets the test
// catch a regression to reading only result.Series[0] of an unsummed query,
// which would silently under-report volume for the most common usage of this
// screen.
func TestEventsSearch_GeoipTrue_AddsTheGeoipFilterToTheCompiledQuery(t *testing.T) {
	fakeLoki := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"success","data":{"result":[]}}`))
	}))
	defer fakeLoki.Close()

	s, st := newTestServer(t)
	s.deps.Loki = loki.New(fakeLoki.URL, fakeLoki.Client())
	token := authToken(t, st, "viewer", 100)

	req := httptest.NewRequest(http.MethodGet, "/events/search?geoip=true", nil)
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
	want := `{job="siem",internal_noise!="true"} | json cc="geoip.country_code" | cc != ""`
	if resp.LogQL != want {
		t.Errorf("LogQL = %q, want %q", resp.LogQL, want)
	}
}

// TestEventsSearch_ExcludesInternalNoiseByDefault and its sibling below are
// the regression coverage for the actual production finding: Loki's own
// query-engine debug output alone accounted for the large majority of
// ingested volume in a live sample, burying real signal in Search's
// default view (see loki.BuildQuery's IncludeInternal doc).
func TestEventsSearch_ExcludesInternalNoiseByDefault(t *testing.T) {
	fakeLoki := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"success","data":{"result":[]}}`))
	}))
	defer fakeLoki.Close()

	s, st := newTestServer(t)
	s.deps.Loki = loki.New(fakeLoki.URL, fakeLoki.Client())
	token := authToken(t, st, "viewer", 100)

	req := httptest.NewRequest(http.MethodGet, "/events/search", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	var resp searchResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if !strings.Contains(resp.LogQL, `internal_noise!="true"`) {
		t.Errorf("LogQL = %q, want internal_noise excluded by default", resp.LogQL)
	}
}

func TestEventsSearch_InternalTrue_OptsIntoPlatformNoise(t *testing.T) {
	fakeLoki := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"success","data":{"result":[]}}`))
	}))
	defer fakeLoki.Close()

	s, st := newTestServer(t)
	s.deps.Loki = loki.New(fakeLoki.URL, fakeLoki.Client())
	token := authToken(t, st, "viewer", 100)

	req := httptest.NewRequest(http.MethodGet, "/events/search?internal=true", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	var resp searchResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if strings.Contains(resp.LogQL, "internal_noise") {
		t.Errorf("LogQL = %q, want no internal_noise term at all when explicitly opted in", resp.LogQL)
	}
}

func TestEventsSearch_IncludesVolumeBuckets(t *testing.T) {
	fakeLoki := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("query")
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/loki/api/v1/query":
			// The total-count and facet instant queries this test doesn't
			// care about - respond with an empty (but validly-shaped)
			// instant vector either way.
			w.Write([]byte(emptyInstantResult))
		case strings.Contains(query, "sum(count_over_time"):
			// Loki already aggregated server-side: exactly one series.
			w.Write([]byte(`{"status":"success","data":{"resultType":"matrix","result":[
				{"metric":{},"values":[[1700000000,"3"],[1700000300,"7"]]}
			]}}`))
		case strings.Contains(query, "count_over_time"):
			// Unsummed query: simulate two distinct streams (e.g. two
			// different `source` values) matching the broad selector, each
			// contributing part of the total. Combined per-bucket totals
			// are the same 3 and 7 as the summed case above.
			w.Write([]byte(`{"status":"success","data":{"resultType":"matrix","result":[
				{"metric":{"source":"udm-ultra"},"values":[[1700000000,"2"],[1700000300,"5"]]},
				{"metric":{"source":"synology-nas"},"values":[[1700000000,"1"],[1700000300,"2"]]}
			]}}`))
		default:
			w.Write([]byte(`{"status":"success","data":{"result":[]}}`))
		}
	}))
	defer fakeLoki.Close()

	s, st := newTestServer(t)
	s.deps.Loki = loki.New(fakeLoki.URL, fakeLoki.Client())

	token := authToken(t, st, "viewer", 100)
	req := httptest.NewRequest(http.MethodGet, "/events/search?start=2023-11-14T00:00:00Z&end=2023-11-14T01:00:00Z", nil)
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
	// These are the combined totals across both simulated streams (2+1=3,
	// 5+2=7), not just one stream's counts — proving the handler asked Loki
	// to sum server-side rather than dropping every series but the first.
	if len(resp.Volume) != 2 || resp.Volume[0].Count != 3 || resp.Volume[1].Count != 7 {
		t.Fatalf("Volume = %+v", resp.Volume)
	}
}

func TestEventsSearch_VolumeFalse_SkipsTheVolumeQueryEntirely(t *testing.T) {
	var sawVolumeQuery bool
	fakeLoki := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("query")
		w.Header().Set("Content-Type", "application/json")
		// Volume buckets are the only thing that queries count_over_time via
		// query_range (matrix) - the total-count and facet queries also
		// contain "count_over_time" but go through the instant /query
		// endpoint instead, so path is what actually distinguishes them.
		if r.URL.Path == "/loki/api/v1/query_range" && strings.Contains(query, "count_over_time") {
			sawVolumeQuery = true
			w.Write([]byte(emptyMatrixResult))
			return
		}
		if r.URL.Path == "/loki/api/v1/query" {
			w.Write([]byte(emptyInstantResult))
			return
		}
		w.Write([]byte(`{"status":"success","data":{"result":[
			{"stream":{"job":"siem"},"values":[["1700000000000000000","hello"]]}
		]}}`))
	}))
	defer fakeLoki.Close()

	s, st := newTestServer(t)
	s.deps.Loki = loki.New(fakeLoki.URL, fakeLoki.Client())

	token := authToken(t, st, "viewer", 100)
	req := httptest.NewRequest(http.MethodGet, "/events/search?volume=false", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	if sawVolumeQuery {
		t.Error("expected no count_over_time (volume) query to be made when volume=false")
	}
	var resp searchResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if len(resp.Entries) != 1 {
		t.Errorf("Entries = %+v, want the 1 entry from the fake server still returned", resp.Entries)
	}
	if resp.Volume == nil || len(resp.Volume) != 0 {
		t.Errorf("Volume = %+v, want an empty (non-nil) array", resp.Volume)
	}
}

func TestEventsSearch_EntriesFalse_SkipsEntriesQueryAndReturnsRealCount(t *testing.T) {
	var sawEntriesQuery bool
	fakeLoki := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("query")
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasPrefix(query, "sum(count_over_time"):
			// The count-only instant query - a real total, deliberately
			// larger than any `limit` a caller might have passed, proving
			// this isn't just len(entries) silently capped.
			w.Write([]byte(`{"status":"success","data":{"result":[
				{"metric":{},"value":[1700000000,"12345"]}
			]}}`))
		case strings.Contains(query, "count_over_time"):
			// Volume-bucket query, requested independently below.
			w.Write([]byte(`{"status":"success","data":{"result":[]}}`))
		default:
			sawEntriesQuery = true
			w.Write([]byte(`{"status":"success","data":{"result":[
				{"stream":{"job":"siem"},"values":[["1700000000000000000","hello"]]}
			]}}`))
		}
	}))
	defer fakeLoki.Close()

	s, st := newTestServer(t)
	s.deps.Loki = loki.New(fakeLoki.URL, fakeLoki.Client())

	token := authToken(t, st, "viewer", 100)
	req := httptest.NewRequest(http.MethodGet, "/events/search?entries=false&volume=false&limit=5", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	if sawEntriesQuery {
		t.Error("expected no entries (query_range log-stream) query to be made when entries=false")
	}
	var resp searchResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if resp.Count != 12345 {
		t.Errorf("Count = %d, want 12345 (the real aggregate, not capped at limit=5)", resp.Count)
	}
	if resp.Entries == nil || len(resp.Entries) != 0 {
		t.Errorf("Entries = %+v, want an empty (non-nil) array", resp.Entries)
	}
}

func TestEventsSearch_SucceedsWhenVolumeQueryFails(t *testing.T) {
	fakeLoki := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("query")
		w.Header().Set("Content-Type", "application/json")
		// Only the volume (matrix, query_range) request fails - the total
		// count and facets (both instant, /query) must keep working, same
		// as this test's sibling below for facets.
		if r.URL.Path == "/loki/api/v1/query_range" && strings.Contains(query, "count_over_time") {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if r.URL.Path == "/loki/api/v1/query" {
			if strings.HasPrefix(query, "sum(count_over_time") {
				w.Write([]byte(`{"status":"success","data":{"result":[
					{"metric":{},"value":[1700000000,"1"]}
				]}}`))
				return
			}
			w.Write([]byte(emptyInstantResult))
			return
		}
		w.Write([]byte(`{"status":"success","data":{"result":[
			{"stream":{"job":"siem"},"values":[["1700000000000000000","hello"]]}
		]}}`))
	}))
	defer fakeLoki.Close()

	s, st := newTestServer(t)
	s.deps.Loki = loki.New(fakeLoki.URL, fakeLoki.Client())

	token := authToken(t, st, "viewer", 100)
	req := httptest.NewRequest(http.MethodGet, "/events/search", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (volume failure must not fail the whole request), body=%s", rec.Code, rec.Body.String())
	}
	var resp searchResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if resp.Count != 1 {
		t.Fatalf("Count = %d, want 1", resp.Count)
	}
	if resp.Volume == nil || len(resp.Volume) != 0 {
		t.Fatalf("Volume = %+v, want a non-nil empty slice", resp.Volume)
	}
}

func TestEventsSearch_SucceedsWhenFacetsQueryFails(t *testing.T) {
	fakeLoki := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("query")
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/loki/api/v1/query" && strings.Contains(query, "sum by (") {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if r.URL.Path == "/loki/api/v1/query" {
			w.Write([]byte(`{"status":"success","data":{"result":[
				{"metric":{},"value":[1700000000,"1"]}
			]}}`))
			return
		}
		if r.URL.Path == "/loki/api/v1/query_range" && strings.Contains(query, "count_over_time") {
			w.Write([]byte(emptyMatrixResult))
			return
		}
		w.Write([]byte(`{"status":"success","data":{"result":[
			{"stream":{"job":"siem"},"values":[["1700000000000000000","hello"]]}
		]}}`))
	}))
	defer fakeLoki.Close()

	s, st := newTestServer(t)
	s.deps.Loki = loki.New(fakeLoki.URL, fakeLoki.Client())

	token := authToken(t, st, "viewer", 100)
	req := httptest.NewRequest(http.MethodGet, "/events/search", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (facets failure must not fail the whole request), body=%s", rec.Code, rec.Body.String())
	}
	var resp searchResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if resp.Count != 1 {
		t.Fatalf("Count = %d, want 1", resp.Count)
	}
	if resp.Facets == nil || len(resp.Facets) != 0 {
		t.Fatalf("Facets = %+v, want a non-nil empty map", resp.Facets)
	}
}

func TestEventsSearch_FacetsFalse_SkipsFacetQueriesEntirely(t *testing.T) {
	var sawFacetQuery bool
	fakeLoki := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("query")
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/loki/api/v1/query" && strings.Contains(query, "sum by (") {
			sawFacetQuery = true
			w.Write([]byte(emptyInstantResult))
			return
		}
		if r.URL.Path == "/loki/api/v1/query" {
			w.Write([]byte(`{"status":"success","data":{"result":[
				{"metric":{},"value":[1700000000,"1"]}
			]}}`))
			return
		}
		if r.URL.Path == "/loki/api/v1/query_range" && strings.Contains(query, "count_over_time") {
			w.Write([]byte(emptyMatrixResult))
			return
		}
		w.Write([]byte(`{"status":"success","data":{"result":[
			{"stream":{"job":"siem"},"values":[["1700000000000000000","hello"]]}
		]}}`))
	}))
	defer fakeLoki.Close()

	s, st := newTestServer(t)
	s.deps.Loki = loki.New(fakeLoki.URL, fakeLoki.Client())

	token := authToken(t, st, "viewer", 100)
	req := httptest.NewRequest(http.MethodGet, "/events/search?facets=false", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	if sawFacetQuery {
		t.Error("expected no sum by (...) (facet) query to be made when facets=false")
	}
	var resp searchResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if resp.Facets == nil || len(resp.Facets) != 0 {
		t.Errorf("Facets = %+v, want a non-nil empty map", resp.Facets)
	}
}

func TestEventsSearch_ReturnsRealFacetCountsNotCappedByEntriesLimit(t *testing.T) {
	fakeLoki := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("query")
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/loki/api/v1/query" && strings.HasPrefix(query, "sum by (severity)"):
			// The bug this exists to catch: entries capped at `limit` (here
			// 1) would only ever "see" one severity value. The real
			// aggregate must be able to report every severity's true count
			// regardless of what the (unrelated) entries limit is.
			w.Write([]byte(`{"status":"success","data":{"result":[
				{"metric":{"severity":"info"},"value":[1700000000,"932"]},
				{"metric":{"severity":"err"},"value":[1700000000,"57"]},
				{"metric":{"severity":"warning"},"value":[1700000000,"1000"]}
			]}}`))
		case r.URL.Path == "/loki/api/v1/query" && strings.Contains(query, "sum by ("):
			w.Write([]byte(emptyInstantResult))
		case r.URL.Path == "/loki/api/v1/query":
			w.Write([]byte(`{"status":"success","data":{"result":[
				{"metric":{},"value":[1700000000,"1989"]}
			]}}`))
		case r.URL.Path == "/loki/api/v1/query_range" && strings.Contains(query, "count_over_time"):
			w.Write([]byte(emptyMatrixResult))
		default:
			w.Write([]byte(`{"status":"success","data":{"result":[
				{"stream":{"job":"siem","severity":"info"},"values":[["1700000000000000000","hello"]]}
			]}}`))
		}
	}))
	defer fakeLoki.Close()

	s, st := newTestServer(t)
	s.deps.Loki = loki.New(fakeLoki.URL, fakeLoki.Client())

	token := authToken(t, st, "viewer", 100)
	req := httptest.NewRequest(http.MethodGet, "/events/search?limit=1", nil)
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
	severities := resp.Facets["severity"]
	if len(severities) != 3 {
		t.Fatalf("Facets[severity] = %+v, want 3 entries even though limit=1", severities)
	}
	// Sorted descending by count.
	if severities[0].Value != "warning" || severities[0].Count != 1000 {
		t.Errorf("severities[0] = %+v, want warning:1000", severities[0])
	}
	if severities[1].Value != "info" || severities[1].Count != 932 {
		t.Errorf("severities[1] = %+v, want info:932", severities[1])
	}
	if severities[2].Value != "err" || severities[2].Count != 57 {
		t.Errorf("severities[2] = %+v, want err:57", severities[2])
	}
}

// TestEventsSearch_ReturnsCountryFacetCounts is the regression test for a
// real production report: the Search page's "Source country" panel was
// always empty. Root cause was the frontend deriving it purely from a page
// of fetched entries (deriveCountryFacet), which almost never contains a
// geoip-bearing line even when plenty exist over the full window - the same
// class of undercount severity/program/source already had a real backend
// aggregate fix for. This covers that fix's LogQL and response shape for
// country specifically: grouped on the JSON-extracted `cc` field (never a
// stream label, see queryCountryFacetCounts), not any of facetLabels.
func TestEventsSearch_ReturnsCountryFacetCounts(t *testing.T) {
	fakeLoki := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("query")
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/loki/api/v1/query" && strings.HasPrefix(query, "sum by (cc)"):
			if !strings.Contains(query, `json cc="geoip.country_code"`) || !strings.Contains(query, `cc != ""`) {
				t.Errorf("country facet query = %q, want the geoip json-extraction filter", query)
			}
			w.Write([]byte(`{"status":"success","data":{"result":[
				{"metric":{"cc":"US"},"value":[1700000000,"14"]},
				{"metric":{"cc":"CN"},"value":[1700000000,"3"]}
			]}}`))
		case r.URL.Path == "/loki/api/v1/query" && strings.Contains(query, "sum by ("):
			w.Write([]byte(emptyInstantResult))
		case r.URL.Path == "/loki/api/v1/query":
			w.Write([]byte(`{"status":"success","data":{"result":[
				{"metric":{},"value":[1700000000,"17"]}
			]}}`))
		case r.URL.Path == "/loki/api/v1/query_range" && strings.Contains(query, "count_over_time"):
			w.Write([]byte(emptyMatrixResult))
		default:
			w.Write([]byte(`{"status":"success","data":{"result":[]}}`))
		}
	}))
	defer fakeLoki.Close()

	s, st := newTestServer(t)
	s.deps.Loki = loki.New(fakeLoki.URL, fakeLoki.Client())

	token := authToken(t, st, "viewer", 100)
	req := httptest.NewRequest(http.MethodGet, "/events/search", nil)
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
	countries := resp.Facets["country"]
	if len(countries) != 2 {
		t.Fatalf("Facets[country] = %+v, want 2 entries", countries)
	}
	if countries[0].Value != "US" || countries[0].Count != 14 {
		t.Errorf("countries[0] = %+v, want US:14", countries[0])
	}
	if countries[1].Value != "CN" || countries[1].Count != 3 {
		t.Errorf("countries[1] = %+v, want CN:3", countries[1])
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

	// Wait for the subscriber to connect with a reasonable timeout
	for i := 0; i < 100; i++ {
		if s.deps.Hub.SubscriberCount("tail") > 0 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if s.deps.Hub.SubscriberCount("tail") == 0 {
		t.Fatal("subscriber did not connect")
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
