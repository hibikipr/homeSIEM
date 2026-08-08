package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hibikipr/homeSIEM/siem-api/internal/loki"
)

func TestEventsStats_ReturnsTotalAndHeatGrid(t *testing.T) {
	fakeLoki := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("query")
		w.Header().Set("Content-Type", "application/json")

		switch {
		case strings.Contains(query, `severity="critical"`):
			w.Write([]byte(`{"status":"success","data":{"resultType":"matrix","result":[
				{"metric":{"source":"udm-ultra"},"values":[[1700000000,"1"],[1700003600,"0"]]}
			]}}`))
		case strings.Contains(query, `severity="warning"`):
			w.Write([]byte(`{"status":"success","data":{"resultType":"matrix","result":[]}}`))
		case strings.Contains(query, "by (source)"):
			// total volume per source per hour (no severity filter)
			w.Write([]byte(`{"status":"success","data":{"resultType":"matrix","result":[
				{"metric":{"source":"udm-ultra"},"values":[[1700000000,"1"],[1700003600,"0"]]},
				{"metric":{"source":"host-1"},"values":[[1700000000,"60"],[1700003600,"3"]]}
			]}}`))
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

	if len(resp.HourlyTotals) != 2 {
		t.Fatalf("len(HourlyTotals) = %d, want 2, got %+v", len(resp.HourlyTotals), resp.HourlyTotals)
	}
	if resp.HourlyTotals[0].Count != 61 {
		t.Errorf("HourlyTotals[0].Count = %d, want 61 (1 udm-ultra + 60 host-1)", resp.HourlyTotals[0].Count)
	}
	if resp.HourlyTotals[1].Count != 3 {
		t.Errorf("HourlyTotals[1].Count = %d, want 3 (0 udm-ultra + 3 host-1)", resp.HourlyTotals[1].Count)
	}
}

func TestBuildHourlyTotals_SumsAcrossSources(t *testing.T) {
	volume := bySourceHourly{
		"udm-ultra": {1700000000: 1, 1700003600: 0},
		"host-1":    {1700000000: 60, 1700003600: 3},
	}

	totals := buildHourlyTotals(volume)

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
	totals := buildHourlyTotals(bySourceHourly{})
	if len(totals) != 0 {
		t.Errorf("len(totals) = %d, want 0", len(totals))
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
