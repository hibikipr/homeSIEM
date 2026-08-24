package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hibikipr/homeSIEM/siem-api/internal/loki"
)

func TestEventsStats_ReturnsTotalAndHeatGrid(t *testing.T) {
	// queryTotal24h and queryHourlyBySource both evaluate via QueryInstant
	// (Loki's /query endpoint, one call per hour bucket for the latter,
	// fired concurrently - see queryHourlyBySource's own doc comment)
	// rather than a single QueryMatrix/query_range call - see stats.go's
	// doc comments for why. Every hour bucket gets the SAME response here
	// (uniform per severity/volume, not tied to any specific "first"
	// timestamp) precisely because requests now arrive out of order under
	// concurrency - a fixture keyed on arrival order would be flaky. Gap-
	// filling for a genuinely quiet hour is already covered directly at
	// the buildHourlyTotals level (TestBuildHourlyTotals_FillsGapsForQuietHours),
	// so it doesn't need re-proving through this HTTP round trip too.
	fakeLoki := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("query")
		w.Header().Set("Content-Type", "application/json")

		if r.URL.Path != "/loki/api/v1/query" {
			t.Errorf("unexpected request path %q, want /loki/api/v1/query (instant) only", r.URL.Path)
		}

		timeNanos, _ := strconv.ParseInt(r.URL.Query().Get("time"), 10, 64)
		at := timeNanos / int64(time.Second)

		switch {
		case !strings.Contains(query, "by (source)"):
			// 24h grand total, no grouping - a single instant call at "end".
			fmt.Fprint(w, `{"status":"success","data":{"resultType":"vector","result":[
				{"metric":{},"value":[1700003600,"1240000"]}
			]}}`)
		case strings.Contains(query, `severity="critical"`):
			fmt.Fprintf(w, `{"status":"success","data":{"resultType":"vector","result":[
				{"metric":{"source":"udm-ultra"},"value":[%d,"1"]}
			]}}`, at)
		case strings.Contains(query, `severity="warning"`):
			fmt.Fprint(w, `{"status":"success","data":{"resultType":"vector","result":[]}}`)
		default:
			// total volume per source, no severity filter
			fmt.Fprintf(w, `{"status":"success","data":{"resultType":"vector","result":[
				{"metric":{"source":"udm-ultra"},"value":[%d,"1"]},
				{"metric":{"source":"host-1"},"value":[%d,"60"]}
			]}}`, at, at)
		}
	}))
	defer fakeLoki.Close()

	s, st := newTestServer(t)
	s.deps.Loki = loki.New(fakeLoki.URL, fakeLoki.Client())
	token := authToken(t, st, "viewer", 100)

	req := httptest.NewRequest(http.MethodGet, "/events/stats", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}

	var resp statsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if resp.EventCount24h != 1240000 {
		t.Errorf("EventCount24h = %d, want 1240000", resp.EventCount24h)
	}

	if len(resp.HeatGrid) != 2 {
		t.Fatalf("len(HeatGrid) = %d, want 2, got %+v", len(resp.HeatGrid), resp.HeatGrid)
	}

	var udm, host1 *sourceHeatRow
	for i := range resp.HeatGrid {
		switch resp.HeatGrid[i].Source {
		case "udm-ultra":
			udm = &resp.HeatGrid[i]
		case "host-1":
			host1 = &resp.HeatGrid[i]
		}
	}
	if udm == nil || host1 == nil {
		t.Fatalf("missing expected sources in HeatGrid: %+v", resp.HeatGrid)
	}

	// Dense 24h+1 series (buildHeatGrid walks every hourly bucket across
	// the handler's own start/end window, not just buckets with data).
	if len(udm.Hours) != 25 {
		t.Fatalf("len(udm.Hours) = %d, want 25 (dense 24h+1 series)", len(udm.Hours))
	}
	// Every hour is identical in this fixture, so every cell should be too.
	for i, h := range udm.Hours {
		if h != "critical" {
			t.Errorf("udm.Hours[%d] = %q, want critical (1 critical event every hour)", i, h)
		}
	}
	for i, h := range host1.Hours {
		if h != "busy" {
			t.Errorf("host1.Hours[%d] = %q, want busy (60 events every hour, no critical/warning)", i, h)
		}
	}

	if len(resp.HourlyTotals) != 25 {
		t.Fatalf("len(HourlyTotals) = %d, want 25 (dense 24h+1 series), got %+v", len(resp.HourlyTotals), resp.HourlyTotals)
	}
	for _, ht := range resp.HourlyTotals {
		if ht.Count != 61 {
			t.Errorf("HourlyTotals count at %v = %d, want 61 (1 udm-ultra + 60 host-1, every hour)", ht.HourStart, ht.Count)
		}
	}
}

func TestEventsStats_QueriesLokiConcurrently(t *testing.T) {
	// Each of the ~76 Loki instant queries this handler makes (1 total +
	// 3*25 hourly-by-source) artificially takes lokiLatency here. Run
	// sequentially, that's ~76*lokiLatency; run concurrently (bounded by
	// MaxConcurrentLokiQueries), it should take a small, roughly constant
	// number of "rounds" regardless of how many buckets there are. This is
	// the actual regression test for the fix - it would fail again if
	// someone reverts to a sequential loop.
	const lokiLatency = 20 * time.Millisecond
	fakeLoki := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(lokiLatency)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"status":"success","data":{"resultType":"vector","result":[]}}`)
	}))
	defer fakeLoki.Close()

	s, st := newTestServer(t)
	s.deps.Loki = loki.New(fakeLoki.URL, fakeLoki.Client())
	token := authToken(t, st, "viewer", 100)

	req := httptest.NewRequest(http.MethodGet, "/events/stats", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	start := time.Now()
	s.Handler().ServeHTTP(rec, req)
	elapsed := time.Since(start)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}

	// Sequential would be ~76*20ms = 1.52s. Fully unbounded-concurrent would
	// be ~2 "rounds" (one per queryHourlyBySource's own 25 buckets, all
	// three of which overlap). Generous upper bound that still clearly
	// distinguishes "parallelized" from "sequential" without being flaky
	// under CI load.
	const maxExpected = 500 * time.Millisecond
	if elapsed > maxExpected {
		t.Errorf("handleEventsStats took %v, want under %v - Loki queries don't appear to be running concurrently", elapsed, maxExpected)
	}
}

func TestBuildHourlyTotals_SumsAcrossSources(t *testing.T) {
	volume := bySourceHourly{
		"udm-ultra": {1700000000: 1, 1700003600: 0},
		"host-1":    {1700000000: 60, 1700003600: 3},
	}

	totals := buildHourlyTotals(volume, time.Unix(1700000000, 0).UTC(), time.Unix(1700003600, 0).UTC())

	if len(totals) != 2 {
		t.Fatalf("len(totals) = %d, want 2", len(totals))
	}
	if totals[0].HourStart.Unix() != 1700000000 || totals[0].Count != 61 {
		t.Errorf("totals[0] = %+v, want {1700000000, 61}", totals[0])
	}
	if totals[1].HourStart.Unix() != 1700003600 || totals[1].Count != 3 {
		t.Errorf("totals[1] = %+v, want {1700003600, 3}", totals[1])
	}
}

func TestBuildHourlyTotals_EmptyVolume(t *testing.T) {
	start := time.Unix(1700000000, 0).UTC()
	end := start.Add(2 * time.Hour)

	// Even with no data anywhere, buildHourlyTotals must still walk and
	// emit real timestamped zero-buckets across the requested range - not
	// an empty slice - since that's the entire point of the gap-filling
	// fix: a quiet range looks like zero-valued buckets, not no buckets.
	totals := buildHourlyTotals(bySourceHourly{}, start, end)

	if len(totals) != 3 {
		t.Fatalf("len(totals) = %d, want 3 (start, start+1h, start+2h)", len(totals))
	}
	for i, want := range []time.Time{start, start.Add(time.Hour), start.Add(2 * time.Hour)} {
		if !totals[i].HourStart.Equal(want) {
			t.Errorf("totals[%d].HourStart = %v, want %v", i, totals[i].HourStart, want)
		}
		if totals[i].Count != 0 {
			t.Errorf("totals[%d].Count = %d, want 0", i, totals[i].Count)
		}
	}
}

func TestBuildHourlyTotals_FillsGapsForQuietHours(t *testing.T) {
	hour0 := time.Unix(1700000000, 0).UTC()
	hour1 := hour0.Add(time.Hour)
	hour2 := hour0.Add(2 * time.Hour)

	// Loki-realistic: no sample at all for the quiet middle hour, not a 0-valued one.
	volume := bySourceHourly{
		"udm-ultra": {hour0.Unix(): 5, hour2.Unix(): 3},
	}

	totals := buildHourlyTotals(volume, hour0, hour2)

	if len(totals) != 3 {
		t.Fatalf("len(totals) = %d, want 3 (hour0, hour1, hour2 - including the gap)", len(totals))
	}
	if totals[0].Count != 5 {
		t.Errorf("totals[0].Count = %d, want 5", totals[0].Count)
	}
	if totals[1].Count != 0 {
		t.Errorf("totals[1].Count = %d, want 0 (quiet hour, no Loki sample, must still appear as a zero bucket)", totals[1].Count)
	}
	if !totals[1].HourStart.Equal(hour1) {
		t.Errorf("totals[1].HourStart = %v, want %v (the gap hour's own timestamp, not skipped)", totals[1].HourStart, hour1)
	}
	if totals[2].Count != 3 {
		t.Errorf("totals[2].Count = %d, want 3", totals[2].Count)
	}
}

func TestEventsStats_RequiresAuth(t *testing.T) {
	s, _ := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/events/stats", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}
