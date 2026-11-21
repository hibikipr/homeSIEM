package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hibikipr/homeSIEM/siem-api/internal/loki"
	"github.com/hibikipr/homeSIEM/siem-api/internal/store"
)

// countingLoki is a fake Loki that answers every query with an empty-ish
// success and counts requests, so tests can assert cache hits.
func countingLoki(t *testing.T, fail *atomic.Bool) (*loki.Client, *atomic.Int32) {
	t.Helper()
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		if fail != nil && fail.Load() {
			http.Error(w, "down", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/loki/api/v1/query" {
			fmt.Fprint(w, `{"status":"success","data":{"resultType":"vector","result":[{"metric":{},"value":[1700003600,"5"]}]}}`)
			return
		}
		fmt.Fprint(w, `{"status":"success","data":{"resultType":"matrix","result":[]}}`)
	}))
	t.Cleanup(srv.Close)
	return loki.New(srv.URL, srv.Client()), &n
}

func get(t *testing.T, s *Server, token, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec
}

func TestEventsStats_SecondCallWithinTTLSkipsLoki(t *testing.T) {
	var fail atomic.Bool
	client, n := countingLoki(t, &fail)
	s, st := newTestServer(t)
	s.deps.Loki = client
	token := authToken(t, st, "viewer", 100)

	first := get(t, s, token, "/events/stats")
	if first.Code != http.StatusOK {
		t.Fatalf("first status = %d", first.Code)
	}
	afterFirst := n.Load()
	if afterFirst != 4 {
		t.Fatalf("Loki requests after first call = %d, want 4", afterFirst)
	}

	second := get(t, s, token, "/events/stats")
	if second.Code != http.StatusOK || second.Body.String() != first.Body.String() {
		t.Fatalf("second response differs or failed: %d", second.Code)
	}
	if got := n.Load(); got != afterFirst {
		t.Fatalf("Loki requests after cached call = %d, want %d (no new queries)", got, afterFirst)
	}
}

func TestEventsStats_ErrorNotCached(t *testing.T) {
	var fail atomic.Bool
	fail.Store(true)
	client, n := countingLoki(t, &fail)
	s, st := newTestServer(t)
	s.deps.Loki = client
	token := authToken(t, st, "viewer", 100)

	if rec := get(t, s, token, "/events/stats"); rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", rec.Code)
	}
	fail.Store(false)
	before := n.Load()
	if rec := get(t, s, token, "/events/stats"); rec.Code != http.StatusOK {
		t.Fatalf("status after recovery = %d, want 200 (error must not be cached)", rec.Code)
	}
	if n.Load() == before {
		t.Fatal("recovery call did not hit Loki")
	}
}

func TestEventsSearch_CacheKeying(t *testing.T) {
	client, n := countingLoki(t, nil)
	s, st := newTestServer(t)
	s.deps.Loki = client
	token := authToken(t, st, "viewer", 100)

	wall := "/events/search?entries=false&volume=false&count=false&facets=country"
	get(t, s, token, wall)
	base := n.Load()
	if base == 0 {
		t.Fatal("expected Loki queries on first call")
	}
	get(t, s, token, wall)
	if n.Load() != base {
		t.Fatal("identical search within TTL hit Loki again")
	}
	// A different filter must not share the entry.
	get(t, s, token, wall+"&severity=critical")
	if n.Load() == base {
		t.Fatal("different filter was served from the wrong cache entry")
	}
	// Absolute ranges key on exact instants.
	abs := "/events/search?entries=false&volume=false&facets=false&start=2026-01-01T00:00:00Z&end=2026-01-01T01:00:00Z"
	get(t, s, token, abs)
	c := n.Load()
	get(t, s, token, abs)
	if n.Load() != c {
		t.Fatal("identical absolute-range search hit Loki again")
	}
	get(t, s, token, strings.Replace(abs, "01:00:00Z", "02:00:00Z", 1))
	if n.Load() == c {
		t.Fatal("different absolute end shared a cache entry")
	}
}

func TestSearchParams_CacheKey(t *testing.T) {
	a := searchParams{limit: 10, facets: []string{"source", "severity"}}
	b := searchParams{limit: 10, facets: []string{"severity", "source"}}
	if a.cacheKey() != b.cacheKey() {
		t.Error("facet order changed the key")
	}
	c := a
	c.start = time.Unix(100, 0)
	if a.cacheKey() == c.cacheKey() {
		t.Error("explicit start did not change the key")
	}
}

func TestListSources_CachesRatesButNotSQLiteFields(t *testing.T) {
	client, n := countingLoki(t, nil)
	s, st := newTestServer(t)
	s.deps.Loki = client
	token := authToken(t, st, "viewer", 100)
	admin := authToken(t, st, "admin", 200)

	src, err := st.UpsertSource(t.Context(), store.Source{Name: "fw", HeartbeatSec: 900})
	if err != nil {
		t.Fatal(err)
	}
	get(t, s, token, "/sources")
	base := n.Load()

	req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/sources/%d/rename", src.ID), strings.NewReader(`{"display_name":"Firewall"}`))
	req.Header.Set("Authorization", "Bearer "+admin)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("rename status = %d", rec.Code)
	}

	got := get(t, s, token, "/sources")
	if !strings.Contains(got.Body.String(), `"display_name":"Firewall"`) {
		t.Fatalf("rename not visible immediately: %s", got.Body.String())
	}
	if n.Load() != base {
		t.Fatal("second /sources call re-queried Loki within TTL")
	}
}
