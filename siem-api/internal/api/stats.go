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
		HeatGrid:      buildHeatGrid(critical, warning, volume, start, end),
		HourlyTotals:  buildHourlyTotals(volume, start, end),
	})
}

// queryTotal24h and queryHourlyBySource both evaluate via QueryInstant
// (Loki's /query endpoint) rather than QueryMatrix (/query_range): this
// Loki deployment's /query_range collapses metric queries to a single,
// incorrectly-timestamped sample regardless of the requested
// start/end/step - confirmed directly against Loki itself, unrelated to
// how this client builds its request. See QueryInstant's own doc comment
// in internal/loki/matrix.go for the full story.
func (s *Server) queryTotal24h(ctx context.Context, start, end time.Time) (int64, error) {
	logql := fmt.Sprintf(`sum(count_over_time({job=%q}[24h]))`, s.deps.JobLabel)
	result, err := s.deps.Loki.QueryInstant(ctx, logql, end)
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

// queryHourlyBySource issues one instant query per hour bucket from start
// to end (inclusive) instead of a single QueryMatrix call - more requests
// (25 for a 24h window instead of 1), but each is a cheap single-point
// aggregation and this only runs once per Wall page load, not a hot path.
// Bucket map keys are the loop's own bucket timestamps, not whatever Loki
// echoes back in the response, so they line up exactly with
// buildHourlyTotals' identical start-to-end hourly walk.
func (s *Server) queryHourlyBySource(ctx context.Context, labelFilter string, start, end time.Time) (bySourceHourly, error) {
	selector := fmt.Sprintf(`{job=%q}`, s.deps.JobLabel)
	if labelFilter != "" {
		selector = fmt.Sprintf(`{job=%q, %s}`, s.deps.JobLabel, labelFilter)
	}
	logql := fmt.Sprintf(`sum by (source) (count_over_time(%s[1h]))`, selector)

	out := bySourceHourly{}
	for bucket := start; !bucket.After(end); bucket = bucket.Add(time.Hour) {
		result, err := s.deps.Loki.QueryInstant(ctx, logql, bucket)
		if err != nil {
			return nil, err
		}
		for _, series := range result.Series {
			source := series.Labels["source"]
			hours := out[source]
			if hours == nil {
				hours = map[int64]float64{}
				out[source] = hours
			}
			for _, sample := range series.Samples {
				hours[bucket.Unix()] = sample.Value
			}
		}
	}
	return out, nil
}

// buildHeatGrid walks every hourly bucket from start to end explicitly for
// each source, the same gap-filling buildHourlyTotals already does (see its
// own comment) - not just the buckets present in `volume`. A source with
// any genuinely quiet hour (zero of ITS OWN traffic, even while other
// sources are busy) would otherwise silently lose that hour's cell instead
// of getting a real "none"-tier one: found via a source that had only
// existed for a couple of hours within the 24h window, whose row was
// missing 20+ hours of cells entirely rather than showing them as quiet.
// Indexing a nil map (a source with no critical/warning events at all)
// with [ts] is safe in Go and returns the zero value, so no existence
// checks are needed here.
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
