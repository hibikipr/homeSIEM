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
	// The heat-grid/hourly-totals queries are keyed off the handler's own
	// start/end (time.Now()-based), so the fixture timestamps below are
	// derived from the request's actual "start" param rather than
	// hardcoded, letting them land inside the dense bucket range that
	// buildHourlyTotals now walks.
	var hour0Sec, hour1Sec int64
	fakeLoki := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("query")
		w.Header().Set("Content-Type", "application/json")

		startNanos, _ := strconv.ParseInt(r.URL.Query().Get("start"), 10, 64)
		hour0 := startNanos / int64(time.Second)
		hour1 := hour0 + 3600
		hour0Sec, hour1Sec = hour0, hour1

		switch {
		case strings.Contains(query, `severity="critical"`):
			fmt.Fprintf(w, `{"status":"success","data":{"resultType":"matrix","result":[
				{"metric":{"source":"udm-ultra"},"values":[[%d,"1"],[%d,"0"]]}
			]}}`, hour0, hour1)
		case strings.Contains(query, `severity="warning"`):
			w.Write([]byte(`{"status":"success","data":{"resultType":"matrix","result":[]}}`))
		case strings.Contains(query, "by (source)"):
			// total volume per source per hour (no severity filter)
			fmt.Fprintf(w, `{"status":"success","data":{"resultType":"matrix","result":[
				{"metric":{"source":"udm-ultra"},"values":[[%d,"1"],[%d,"0"]]},
				{"metric":{"source":"host-1"},"values":[[%d,"60"],[%d,"3"]]}
			]}}`, hour0, hour1, hour0, hour1)
		default:
			// 24h grand total, no grouping
			w.Write([]byte(`{"status":"success","data":{"resultType":"matrix","result":[
				{"metric":{},"values":[[1700003600,"1240000"]]}
			]}}`))
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

	if len(udm.Hours) != 2 {
		t.Fatalf("len(udm.Hours) = %d, want 2", len(udm.Hours))
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
