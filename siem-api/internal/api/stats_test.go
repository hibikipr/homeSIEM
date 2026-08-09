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
	// (Loki's /query endpoint, one call per hour bucket for the latter)
	// rather than a single QueryMatrix/query_range call - see stats.go's
	// doc comments for why. This fake server responds per-instant-call,
	// keyed on each request's own "time" param rather than a shared
	// "start" - the first "by (source)"-shaped request's time becomes
	// hour0 (the handler's actual start, i.e. real time.Now()-24h, so it
	// can't be hardcoded), hour1 = hour0+3600, and every other hour bucket
	// gets an empty result, matching a real quiet-hour Loki response.
	var hour0Sec, hour1Sec int64
	fakeLoki := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("query")
		w.Header().Set("Content-Type", "application/json")

		if r.URL.Path != "/loki/api/v1/query" {
			t.Errorf("unexpected request path %q, want /loki/api/v1/query (instant) only", r.URL.Path)
		}

		timeNanos, _ := strconv.ParseInt(r.URL.Query().Get("time"), 10, 64)
		at := timeNanos / int64(time.Second)

		if !strings.Contains(query, "by (source)") {
			// 24h grand total, no grouping - a single instant call at "end".
			fmt.Fprint(w, `{"status":"success","data":{"resultType":"vector","result":[
				{"metric":{},"value":[1700003600,"1240000"]}
			]}}`)
			return
		}

		if hour0Sec == 0 {
			hour0Sec = at
			hour1Sec = hour0Sec + 3600
		}

		switch {
		case at != hour0Sec && at != hour1Sec:
			fmt.Fprint(w, `{"status":"success","data":{"resultType":"vector","result":[]}}`)
		case strings.Contains(query, `severity="critical"`):
			if at == hour0Sec {
				fmt.Fprintf(w, `{"status":"success","data":{"resultType":"vector","result":[
					{"metric":{"source":"udm-ultra"},"value":[%d,"1"]}
				]}}`, at)
			} else {
				fmt.Fprint(w, `{"status":"success","data":{"resultType":"vector","result":[]}}`)
			}
		case strings.Contains(query, `severity="warning"`):
			fmt.Fprint(w, `{"status":"success","data":{"resultType":"vector","result":[]}}`)
		default:
			// total volume per source, no severity filter
			if at == hour0Sec {
				fmt.Fprintf(w, `{"status":"success","data":{"resultType":"vector","result":[
					{"metric":{"source":"udm-ultra"},"value":[%d,"1"]},
					{"metric":{"source":"host-1"},"value":[%d,"60"]}
				]}}`, at, at)
			} else {
				fmt.Fprintf(w, `{"status":"success","data":{"resultType":"vector","result":[
					{"metric":{"source":"udm-ultra"},"value":[%d,"0"]},
					{"metric":{"source":"host-1"},"value":[%d,"3"]}
				]}}`, at, at)
			}
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

	// buildHeatGrid now walks every hourly bucket across the handler's own
	// 24h start/end window (dense, gap-filled - the same fix
	// buildHourlyTotals already had), not just the buckets present in the
	// volume map - so each row has ~25 cells (24h inclusive of both ends),
	// and hour0Sec/hour1Sec (the fixture's only two non-empty hours) land
	// at exactly indices 0/1 since hour0Sec is the loop's first bucket
	// (queryHourlyBySource's very first request, for severity="critical",
	// always starts at `start` itself).
	if len(udm.Hours) != 25 {
		t.Fatalf("len(udm.Hours) = %d, want 25 (dense 24h+1 series)", len(udm.Hours))
	}
	if udm.Hours[0] != "critical" {
		t.Errorf("udm.Hours[0] = %q, want critical (1 critical event that hour)", udm.Hours[0])
	}
	if udm.Hours[1] != "none" {
		t.Errorf("udm.Hours[1] = %q, want none (zero volume that hour)", udm.Hours[1])
	}

	if host1.Hours[0] != "busy" {
		t.Errorf("host1.Hours[0] = %q, want busy (60 events, no critical/warning)", host1.Hours[0])
	}
	if host1.Hours[1] != "quiet" {
		t.Errorf("host1.Hours[1] = %q, want quiet (3 events)", host1.Hours[1])
	}

	// A genuinely quiet hour (no data from any source, not just this one)
	// must still produce a real "none" cell at its own index, not be
	// omitted from the row entirely - this is the actual gap-filling fix:
	// without it, both rows would only ever have 2 cells (hour0, hour1),
	// never 25.
	if udm.Hours[2] != "none" {
		t.Errorf("udm.Hours[2] = %q, want none (a real gap-filled quiet hour, not omitted)", udm.Hours[2])
	}
	if host1.Hours[2] != "none" {
		t.Errorf("host1.Hours[2] = %q, want none (a real gap-filled quiet hour, not omitted)", host1.Hours[2])
	}

	// buildHourlyTotals now walks every hourly bucket across the handler's
	// own 24h start/end window (dense, gap-filled), not just the two
	// timestamps present in the fake Loki fixture - so the response has
	// ~25 buckets (24h inclusive of both ends), and the fixture's two
	// known timestamps must be located by HourStart rather than assumed
	// to be resp.HourlyTotals[0]/[1].
	if len(resp.HourlyTotals) != 25 {
		t.Fatalf("len(HourlyTotals) = %d, want 25 (dense 24h+1 series), got %+v", len(resp.HourlyTotals), resp.HourlyTotals)
	}

	byHour := map[int64]int64{}
	for _, ht := range resp.HourlyTotals {
		byHour[ht.HourStart.Unix()] = ht.Count
	}

	if got, want := byHour[hour0Sec], int64(61); got != want {
		t.Errorf("HourlyTotals count at hour0 = %d, want %d (1 udm-ultra + 60 host-1)", got, want)
	}
	if got, want := byHour[hour1Sec], int64(3); got != want {
		t.Errorf("HourlyTotals count at hour1 = %d, want %d (0 udm-ultra + 3 host-1)", got, want)
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
