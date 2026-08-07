package api

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/hibikipr/homeSIEM/siem-api/internal/loki"
)

type volumeBucket struct {
	BucketStart time.Time `json:"bucket_start"`
	Count       int64     `json:"count"`
}

type searchResponse struct {
	LogQL   string          `json:"logql"`
	Count   int             `json:"count"`
	Entries []loki.LogEntry `json:"entries"`
	Volume  []volumeBucket  `json:"volume"`
}

func (s *Server) handleEventsSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	filters := loki.Filters{
		Source:   q.Get("source"),
		Host:     q.Get("host"),
		Program:  q.Get("program"),
		Severity: q.Get("severity"),
		Facility: q.Get("facility"),
		FreeText: q.Get("q"),
	}
	logql := loki.BuildQuery(s.deps.JobLabel, filters)

	end := time.Now().UTC()
	start := end.Add(-24 * time.Hour)
	if v := q.Get("start"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			start = t
		}
	}
	if v := q.Get("end"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			end = t
		}
	}
	limit := 1000
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}

	result, err := s.deps.Loki.QueryRange(r.Context(), logql, start, end, limit)
	if err != nil {
		s.deps.Logger.Error("events search: query failed", "error", err)
		http.Error(w, "query failed", http.StatusBadGateway)
		return
	}

	volume, err := s.queryVolumeBuckets(r.Context(), logql, start, end)
	if err != nil {
		s.deps.Logger.Error("events search: volume query failed", "error", err)
		volume = []volumeBucket{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(searchResponse{LogQL: logql, Count: len(result.Entries), Entries: result.Entries, Volume: volume})
}

func (s *Server) handleEventsTail(w http.ResponseWriter, r *http.Request) {
	s.deps.Hub.ServeHTTP("tail", w, r)
}

const volumeBucketCount = 72

// queryVolumeBuckets returns real Loki-side counts (not derived from the
// capped entries sample above, since results are limited but true event
// volume in a busy window may exceed that limit) across `volumeBucketCount`
// evenly-sized buckets spanning [start, end].
func (s *Server) queryVolumeBuckets(ctx context.Context, logql string, start, end time.Time) ([]volumeBucket, error) {
	total := end.Sub(start)
	if total <= 0 {
		return []volumeBucket{}, nil
	}
	bucketWidth := total / volumeBucketCount
	if bucketWidth < time.Second {
		bucketWidth = time.Second
	}
	bucketSeconds := int64(math.Ceil(bucketWidth.Seconds()))
	countLogQL := fmt.Sprintf("sum(count_over_time(%s[%ds]))", logql, bucketSeconds)

	result, err := s.deps.Loki.QueryMatrix(ctx, countLogQL, start, end, time.Duration(bucketSeconds)*time.Second)
	if err != nil {
		return nil, err
	}
	if len(result.Series) == 0 {
		return []volumeBucket{}, nil
	}

	buckets := make([]volumeBucket, len(result.Series[0].Samples))
	for i, sample := range result.Series[0].Samples {
		buckets[i] = volumeBucket{BucketStart: sample.Timestamp, Count: int64(sample.Value)}
	}
	return buckets, nil
}
