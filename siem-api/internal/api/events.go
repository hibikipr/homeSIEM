package api

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"sync"
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

	// includeVolume lets a caller that only wants entries (the Wall's use of
	// this endpoint to derive a country breakdown, never the volume
	// histogram) skip a whole extra Loki matrix query it would otherwise
	// discard. Defaults to true - the actual Search screen always wants both.
	includeVolume := q.Get("volume") != "false"

	// includeEntries lets a caller that only wants a total count (Search's
	// "N events from this IP in the last 24h" context callout, found
	// fetching up to 5000 full log entries - each the complete enriched
	// event JSON - over the network just to read len(entries)) skip the
	// entries query and get a real Loki-side aggregate count instead, which
	// is also not silently capped at `limit` the way len(entries) is.
	// Defaults to true - the actual Search screen always wants entries.
	includeEntries := q.Get("entries") != "false"

	// The entries query, the count-only query, and the volume-bucket query
	// are all independent Loki requests over the same LogQL/time range - run
	// whichever of them are needed concurrently rather than one after
	// another, since none depends on another's result.
	var (
		wg         sync.WaitGroup
		result     loki.QueryResult
		resultErr  error
		totalCount int
		countErr   error
		volume     []volumeBucket
		volumeErr  error
	)

	if includeEntries {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, resultErr = s.deps.Loki.QueryRange(r.Context(), logql, start, end, limit)
		}()
	} else {
		wg.Add(1)
		go func() {
			defer wg.Done()
			totalCount, countErr = s.queryTotalCount(r.Context(), logql, start, end)
		}()
	}

	if includeVolume {
		wg.Add(1)
		go func() {
			defer wg.Done()
			volume, volumeErr = s.queryVolumeBuckets(r.Context(), logql, start, end)
		}()
	}

	wg.Wait()

	if includeEntries {
		if resultErr != nil {
			s.deps.Logger.Error("events search: query failed", "error", resultErr)
			http.Error(w, "query failed", http.StatusBadGateway)
			return
		}
	} else if countErr != nil {
		s.deps.Logger.Error("events search: count query failed", "error", countErr)
		http.Error(w, "query failed", http.StatusBadGateway)
		return
	}
	if volumeErr != nil {
		s.deps.Logger.Error("events search: volume query failed", "error", volumeErr)
		volume = nil
	}
	if volume == nil {
		volume = []volumeBucket{}
	}

	count := len(result.Entries)
	if !includeEntries {
		count = totalCount
	}
	entries := result.Entries
	if entries == nil {
		entries = []loki.LogEntry{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(searchResponse{LogQL: logql, Count: count, Entries: entries, Volume: volume})
}

func (s *Server) handleEventsTail(w http.ResponseWriter, r *http.Request) {
	s.deps.Hub.ServeHTTP("tail", w, r)
}

// queryTotalCount returns a real Loki-side total match count over
// [start, end] via a single instant sum(count_over_time(...)) query -
// dramatically cheaper than fetching entries just to read len(entries),
// and not silently capped at any `limit` the way that would be.
func (s *Server) queryTotalCount(ctx context.Context, logql string, start, end time.Time) (int, error) {
	window := end.Sub(start)
	if window <= 0 {
		return 0, nil
	}
	countLogQL := fmt.Sprintf("sum(count_over_time(%s[%ds]))", logql, int64(math.Ceil(window.Seconds())))

	result, err := s.deps.Loki.QueryInstant(ctx, countLogQL, end)
	if err != nil {
		return 0, err
	}
	if len(result.Series) == 0 || len(result.Series[0].Samples) == 0 {
		return 0, nil
	}
	return int(result.Series[0].Samples[0].Value), nil
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
