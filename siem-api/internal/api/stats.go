package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"time"
)

type statsResponse struct {
	EventCount24h int64           `json:"event_count_24h"`
	HeatGrid      []sourceHeatRow `json:"heat_grid"`
	HourlyTotals  []hourlyTotal   `json:"hourly_totals"`
}

type sourceHeatRow struct {
	Source string   `json:"source"`
	Hours  []string `json:"hours"`
}

type hourlyTotal struct {
	HourStart time.Time `json:"hour_start"`
	Count     int64     `json:"count"`
}

// Heat grid activity tiers for a (source, hour) cell that has neither a
// critical nor a warning event. Thresholds are a first pass and easy to
// tune once real homelab volume is observed.
const (
	heatBusyThreshold  = 50
	heatLightThreshold = 5
)

func (s *Server) handleEventsStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	end := time.Now().UTC()
	start := end.Add(-24 * time.Hour)

	total, err := s.queryTotal24h(ctx, start, end)
	if err != nil {
		s.deps.Logger.Error("events stats: total query failed", "error", err)
		http.Error(w, "query failed", http.StatusBadGateway)
		return
	}

	critical, err := s.queryHourlyBySource(ctx, `severity="critical"`, start, end)
	if err != nil {
		s.deps.Logger.Error("events stats: critical query failed", "error", err)
		http.Error(w, "query failed", http.StatusBadGateway)
		return
	}
	warning, err := s.queryHourlyBySource(ctx, `severity="warning"`, start, end)
	if err != nil {
		s.deps.Logger.Error("events stats: warning query failed", "error", err)
		http.Error(w, "query failed", http.StatusBadGateway)
		return
	}
	volume, err := s.queryHourlyBySource(ctx, "", start, end)
	if err != nil {
		s.deps.Logger.Error("events stats: volume query failed", "error", err)
		http.Error(w, "query failed", http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(statsResponse{
		EventCount24h: total,
		HeatGrid:      buildHeatGrid(critical, warning, volume),
		HourlyTotals:  buildHourlyTotals(volume),
	})
}

func (s *Server) queryTotal24h(ctx context.Context, start, end time.Time) (int64, error) {
	logql := fmt.Sprintf(`sum(count_over_time({job=%q}[24h]))`, s.deps.JobLabel)
	result, err := s.deps.Loki.QueryMatrix(ctx, logql, start, end, 24*time.Hour)
	if err != nil {
		return 0, err
	}
	if len(result.Series) == 0 || len(result.Series[0].Samples) == 0 {
		return 0, nil
	}
	samples := result.Series[0].Samples
	return int64(samples[len(samples)-1].Value), nil
}

// bySourceHourly maps source -> hour-bucket unix timestamp -> value.
type bySourceHourly map[string]map[int64]float64

func (s *Server) queryHourlyBySource(ctx context.Context, labelFilter string, start, end time.Time) (bySourceHourly, error) {
	selector := fmt.Sprintf(`{job=%q}`, s.deps.JobLabel)
	if labelFilter != "" {
		selector = fmt.Sprintf(`{job=%q, %s}`, s.deps.JobLabel, labelFilter)
	}
	logql := fmt.Sprintf(`sum by (source) (count_over_time(%s[1h]))`, selector)

	result, err := s.deps.Loki.QueryMatrix(ctx, logql, start, end, time.Hour)
	if err != nil {
		return nil, err
	}

	out := bySourceHourly{}
	for _, series := range result.Series {
		source := series.Labels["source"]
		bucket := out[source]
		if bucket == nil {
			bucket = map[int64]float64{}
			out[source] = bucket
		}
		for _, sample := range series.Samples {
			bucket[sample.Timestamp.Unix()] = sample.Value
		}
	}
	return out, nil
}

func buildHeatGrid(critical, warning, volume bySourceHourly) []sourceHeatRow {
	var rows []sourceHeatRow
	for source, hours := range volume {
		var hourTimestamps []int64
		for ts := range hours {
			hourTimestamps = append(hourTimestamps, ts)
		}
		sort.Slice(hourTimestamps, func(i, j int) bool { return hourTimestamps[i] < hourTimestamps[j] })

		row := sourceHeatRow{Source: source}
		for _, ts := range hourTimestamps {
			row.Hours = append(row.Hours, classifyHeatCell(
				critical[source][ts], warning[source][ts], hours[ts]))
		}
		rows = append(rows, row)
	}
	return rows
}

// buildHourlyTotals sums the same per-source hourly volume buildHeatGrid uses
// across all sources, producing a flat total-events-per-hour series - no new
// Loki query needed, this reuses data already fetched for the heat grid.
func buildHourlyTotals(volume bySourceHourly) []hourlyTotal {
	sums := map[int64]float64{}
	for _, hours := range volume {
		for ts, count := range hours {
			sums[ts] += count
		}
	}

	var timestamps []int64
	for ts := range sums {
		timestamps = append(timestamps, ts)
	}
	sort.Slice(timestamps, func(i, j int) bool { return timestamps[i] < timestamps[j] })

	totals := make([]hourlyTotal, len(timestamps))
	for i, ts := range timestamps {
		totals[i] = hourlyTotal{HourStart: time.Unix(ts, 0).UTC(), Count: int64(sums[ts])}
	}
	return totals
}

func classifyHeatCell(criticalCount, warningCount, totalCount float64) string {
	switch {
	case criticalCount > 0:
		return "critical"
	case warningCount > 0:
		return "warning"
	case totalCount == 0:
		return "none"
	case totalCount >= heatBusyThreshold:
		return "busy"
	case totalCount >= heatLightThreshold:
		return "light"
	default:
		return "quiet"
	}
}
