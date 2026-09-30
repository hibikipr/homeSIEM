package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"sync"
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
	now := time.Now().UTC()
	// The hourly grid is anchored on hour boundaries, ending at the top of
	// the next hour so its last bucket covers the hour in progress. Loki
	// aligns /query_range evaluation timestamps to multiples of `step`
	// anyway; aligning the window ourselves makes the timestamps it returns
	// exactly the buckets buildHeatGrid/buildHourlyTotals walk.
	end := now.Truncate(time.Hour).Add(time.Hour)
	start := end.Add(-24 * time.Hour)

	// The four queries below are independent of each other, so they run
	// concurrently instead of one after another.
	var (
		total                     int64
		critical, warning, volume bySourceHourly
		totalErr, criticalErr     error
		warningErr, volumeErr     error
	)
	var wg sync.WaitGroup
	wg.Add(4)
	go func() {
		defer wg.Done()
		total, totalErr = s.queryTotal24h(ctx, now)
	}()
	go func() {
		defer wg.Done()
		critical, criticalErr = s.queryHourlyBySource(ctx, `severity="critical"`, start, end)
	}()
	go func() {
		defer wg.Done()
		warning, warningErr = s.queryHourlyBySource(ctx, `severity="warning"`, start, end)
	}()
	go func() {
		defer wg.Done()
		volume, volumeErr = s.queryHourlyBySource(ctx, "", start, end)
	}()
	wg.Wait()

	if totalErr != nil {
		s.deps.Logger.Error("events stats: total query failed", "error", totalErr)
		http.Error(w, "query failed", http.StatusBadGateway)
		return
	}
	if criticalErr != nil {
		s.deps.Logger.Error("events stats: critical query failed", "error", criticalErr)
		http.Error(w, "query failed", http.StatusBadGateway)
		return
	}
	if warningErr != nil {
		s.deps.Logger.Error("events stats: warning query failed", "error", warningErr)
		http.Error(w, "query failed", http.StatusBadGateway)
		return
	}
	if volumeErr != nil {
		s.deps.Logger.Error("events stats: volume query failed", "error", volumeErr)
		http.Error(w, "query failed", http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(statsResponse{
		EventCount24h: total,
		HeatGrid:      buildHeatGrid(critical, warning, volume, start, end),
		HourlyTotals:  buildHourlyTotals(volume, start, end),
	})
}

// queryTotal24h is a single instant query over the rolling 24h ending at
// `at` (not the hour-aligned grid window, which can end up to an hour in
// the future).
func (s *Server) queryTotal24h(ctx context.Context, at time.Time) (int64, error) {
	logql := fmt.Sprintf(`sum(count_over_time({job=%q}[24h]))`, s.deps.JobLabel)
	result, err := s.deps.Loki.QueryInstant(ctx, logql, at)
	if err != nil {
		return 0, err
	}
	if len(result.Series) == 0 || len(result.Series[0].Samples) == 0 {
		return 0, nil
	}
	return int64(result.Series[0].Samples[0].Value), nil
}

// bySourceHourly maps source -> hour-bucket unix timestamp -> value.
type bySourceHourly map[string]map[int64]float64

// queryHourlyBySource returns per-source hourly counts for every hour
// bucket from start to end (inclusive; both hour-aligned) with a single
// /query_range call at a 1h step - one Loki request per series instead of
// the 25 per-bucket instant queries this used to fire. (That workaround
// assumed /query_range collapsed metric queries to one sample; verified
// against the homelab's Loki 3.7.8, it returns a real per-hour series.)
//
// Sample timestamps are snapped to the nearest hour before being used as
// map keys, so they line up exactly with buildHeatGrid/buildHourlyTotals'
// start-to-end hourly walk even if Loki returns a fractional timestamp.
func (s *Server) queryHourlyBySource(ctx context.Context, labelFilter string, start, end time.Time) (bySourceHourly, error) {
	selector := fmt.Sprintf(`{job=%q}`, s.deps.JobLabel)
	if labelFilter != "" {
		selector = fmt.Sprintf(`{job=%q, %s}`, s.deps.JobLabel, labelFilter)
	}
	logql := fmt.Sprintf(`sum by (source) (count_over_time(%s[1h]))`, selector)

	s.lokiSem <- struct{}{}
	result, err := s.deps.Loki.QueryMatrix(ctx, logql, start, end, time.Hour)
	<-s.lokiSem
	if err != nil {
		return nil, err
	}

	out := bySourceHourly{}
	for _, series := range result.Series {
		source := series.Labels["source"]
		hours := out[source]
		if hours == nil {
			hours = map[int64]float64{}
			out[source] = hours
		}
		for _, sample := range series.Samples {
			ts := sample.Timestamp.Round(time.Hour)
			if ts.Before(start) || ts.After(end) {
				continue
			}
			hours[ts.Unix()] = sample.Value
		}
	}
	return out, nil
}

func buildHeatGrid(critical, warning, volume bySourceHourly, start, end time.Time) []sourceHeatRow {
	sources := map[string]struct{}{}
	for source := range volume {
		sources[source] = struct{}{}
	}
	for source := range critical {
		sources[source] = struct{}{}
	}
	for source := range warning {
		sources[source] = struct{}{}
	}

	var rows []sourceHeatRow
	for source := range sources {
		row := sourceHeatRow{Source: source}
		for bucket := start; !bucket.After(end); bucket = bucket.Add(time.Hour) {
			ts := bucket.Unix()
			row.Hours = append(row.Hours, classifyHeatCell(
				critical[source][ts], warning[source][ts], volume[source][ts]))
		}
		rows = append(rows, row)
	}
	// Map iteration order is randomized in Go - sort so the UI's row order
	// is stable across page loads instead of shuffling on every request.
	sort.Slice(rows, func(i, j int) bool { return rows[i].Source < rows[j].Source })
	return rows
}

// buildHourlyTotals sums the same per-source hourly volume buildHeatGrid uses
// across all sources, producing a flat total-events-per-hour series - no new
// Loki query needed, this reuses data already fetched for the heat grid.
// Walks every hourly bucket from start to end explicitly (not just the
// buckets present in `volume`) because Loki's range-vector query omits a
// sample entirely for an hour with zero matching log lines - it does not
// return an explicit 0. Without this, a genuinely quiet hour would be
// silently absent from the series instead of present-with-zero, which
// would compress real time gaps in the chart (points spaced evenly by
// array index) and mislabel its hour-axis (every-4th-point stops meaning
// every 4 real hours once the series isn't dense).
func buildHourlyTotals(volume bySourceHourly, start, end time.Time) []hourlyTotal {
	sums := map[int64]float64{}
	for _, hours := range volume {
		for ts, count := range hours {
			sums[ts] += count
		}
	}

	var totals []hourlyTotal
	for bucket := start; !bucket.After(end); bucket = bucket.Add(time.Hour) {
		totals = append(totals, hourlyTotal{HourStart: bucket, Count: int64(sums[bucket.Unix()])})
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
