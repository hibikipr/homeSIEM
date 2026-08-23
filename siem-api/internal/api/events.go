package api

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/hibikipr/homeSIEM/siem-api/internal/loki"
)

type volumeBucket struct {
	BucketStart time.Time `json:"bucket_start"`
	Count       int64     `json:"count"`
}

type facetCount struct {
	Value string `json:"value"`
	Count int64  `json:"count"`
}

type searchResponse struct {
	LogQL   string          `json:"logql"`
	Count   int             `json:"count"`
	Entries []loki.LogEntry `json:"entries"`
	Volume  []volumeBucket  `json:"volume"`
	// Facets holds real Loki-side aggregate counts per facet, keyed by facet
	// name: "severity", "program", "source" (genuine Loki stream labels, see
	// loki.BuildQuery and queryFacetCounts) plus "country" (not a stream
	// label - extracted from each line's JSON body at query time via the
	// same `| json cc="geoip.country_code"` LogQL RequireGeoIP already uses,
	// see queryCountryFacetCounts). Each label facet's own filter is
	// excluded from the query it's computed with (so the severity facet,
	// for example, shows counts across all severities given the other
	// active filters, not just whichever one is currently selected).
	//
	// Added because the frontend used to derive these entirely from
	// `entries`, which is capped at `limit` (1000 by default): a 24h
	// window with 932 info-level lines and 11 real warnings would exhaust
	// the whole cap on info alone, showing "warning: 11" when the true
	// count over that window was actually >1000 - a two-orders-of-
	// magnitude undercount that could hide a real incident. These are
	// separate real aggregate queries specifically so they aren't subject
	// to that cap.
	Facets map[string][]facetCount `json:"facets"`
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
		// Excluded by default (Filters' zero value) - see IncludeInternal's
		// doc. A user who actually wants to see this stack's own
		// operational chatter (debugging Loki/Docker directly, say) can
		// still opt back in explicitly.
		IncludeInternal: q.Get("internal") == "true",
		RequireGeoIP:    q.Get("geoip") == "true",
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
	// entries query. Defaults to true - the actual Search screen always
	// wants entries. Count itself is always the real Loki-side aggregate
	// (see totalCount below), never derived from len(entries) - that used
	// to silently cap Count at `limit` too, presenting e.g. "1,000 events"
	// as if it were a total rather than a page size.
	includeEntries := q.Get("entries") != "false"

	// includeFacets computes real per-label aggregate counts (severity,
	// program, source) alongside the main query - see queryFacetCounts and
	// the Facets field doc. Skippable for callers (the context-summary
	// lookup) that only want a total count, same reasoning as includeVolume.
	includeFacets := q.Get("facets") != "false"

	// The entries query, the count query, the volume-bucket query, and each
	// facet query are all independent Loki requests over the same
	// LogQL/time range - run whichever are needed concurrently rather than
	// one after another, since none depends on another's result.
	var (
		wg         sync.WaitGroup
		result     loki.QueryResult
		resultErr  error
		totalCount int
		countErr   error
		volume     []volumeBucket
		volumeErr  error
		facets     map[string][]facetCount
		facetsErr  error
	)

	if includeEntries {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, resultErr = s.deps.Loki.QueryRange(r.Context(), logql, start, end, limit)
		}()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		totalCount, countErr = s.queryTotalCount(r.Context(), logql, start, end)
	}()

	if includeVolume {
		wg.Add(1)
		go func() {
			defer wg.Done()
			volume, volumeErr = s.queryVolumeBuckets(r.Context(), logql, start, end)
		}()
	}

	if includeFacets {
		wg.Add(1)
		go func() {
			defer wg.Done()
			facets, facetsErr = s.queryFacets(r.Context(), filters, start, end)
		}()
	}

	wg.Wait()

	if includeEntries && resultErr != nil {
		s.deps.Logger.Error("events search: query failed", "error", resultErr)
		http.Error(w, "query failed", http.StatusBadGateway)
		return
	}
	if countErr != nil {
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
	if facetsErr != nil {
		s.deps.Logger.Error("events search: facets query failed", "error", facetsErr)
		facets = nil
	}
	if facets == nil {
		facets = map[string][]facetCount{}
	}

	entries := result.Entries
	if entries == nil {
		entries = []loki.LogEntry{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(searchResponse{
		LogQL:   logql,
		Count:   totalCount,
		Entries: entries,
		Volume:  volume,
		Facets:  facets,
	})
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

// facetLabels are the Filters fields that correspond to real Loki stream
// labels (see loki.BuildQuery) - only these can be aggregated with a LogQL
// `sum by (...)`. Facility and free text are deliberately excluded: facility
// only exists inside each line's JSON body (never promoted to a label, see
// the comment on Filters.Facility), and free text isn't a facet at all.
var facetLabels = []string{"severity", "program", "source"}

// queryFacets runs one real Loki-side aggregate query per entry in
// facetLabels, concurrently, each with that facet's own filter excluded
// from the query it's computed with - so e.g. the severity facet reports
// counts across every severity given the other active filters, not just
// whichever severity happens to already be selected. A single facet's
// failure doesn't fail the others; it's simply omitted from the result.
func (s *Server) queryFacets(ctx context.Context, filters loki.Filters, start, end time.Time) (map[string][]facetCount, error) {
	type facetResult struct {
		label  string
		counts []facetCount
		err    error
	}

	results := make(chan facetResult, len(facetLabels)+1)
	var wg sync.WaitGroup
	for _, label := range facetLabels {
		wg.Add(1)
		go func(label string) {
			defer wg.Done()
			counts, err := s.queryFacetCounts(ctx, filters, label, start, end)
			results <- facetResult{label: label, counts: counts, err: err}
		}(label)
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		counts, err := s.queryCountryFacetCounts(ctx, filters, start, end)
		results <- facetResult{label: "country", counts: counts, err: err}
	}()
	wg.Wait()
	close(results)

	out := make(map[string][]facetCount, len(facetLabels))
	var firstErr error
	for res := range results {
		if res.err != nil {
			s.deps.Logger.Error("events search: facet query failed", "label", res.label, "error", res.err)
			if firstErr == nil {
				firstErr = res.err
			}
			continue
		}
		out[res.label] = res.counts
	}
	if len(out) == 0 && firstErr != nil {
		return nil, firstErr
	}
	return out, nil
}

// queryFacetCounts returns a real Loki-side count per distinct value of
// `label`, via a single instant `sum by (label) (count_over_time(...))`
// query - the same technique queryTotalCount uses for the overall total,
// grouped instead of summed to one series.
func (s *Server) queryFacetCounts(ctx context.Context, filters loki.Filters, label string, start, end time.Time) ([]facetCount, error) {
	facetFilters := filters
	switch label {
	case "severity":
		facetFilters.Severity = ""
	case "program":
		facetFilters.Program = ""
	case "source":
		facetFilters.Source = ""
	}
	logql := loki.BuildQuery(s.deps.JobLabel, facetFilters)

	window := end.Sub(start)
	if window <= 0 {
		return []facetCount{}, nil
	}
	aggLogQL := fmt.Sprintf("sum by (%s) (count_over_time(%s[%ds]))", label, logql, int64(math.Ceil(window.Seconds())))

	result, err := s.deps.Loki.QueryInstant(ctx, aggLogQL, end)
	if err != nil {
		return nil, err
	}

	counts := make([]facetCount, 0, len(result.Series))
	for _, series := range result.Series {
		value := series.Labels[label]
		if value == "" || len(series.Samples) == 0 {
			continue
		}
		counts = append(counts, facetCount{Value: value, Count: int64(series.Samples[0].Value)})
	}
	sort.Slice(counts, func(i, j int) bool { return counts[i].Count > counts[j].Count })
	return counts, nil
}

// queryCountryFacetCounts returns a real Loki-side count per source country,
// the same technique queryFacetCounts uses for severity/program/source, but
// grouping on a JSON-extracted field instead of a stream label:
// geoip.country_code is deliberately never promoted to a label (see
// Filters.Facility's doc for why - the same cardinality reasoning applies),
// so `sum by (source)`-style label aggregation can't reach it. RequireGeoIP
// already builds the exact `| json cc="geoip.country_code" | cc != ""`
// extraction Search's geoip=true filter uses; forcing it on here (regardless
// of what the caller's own filters requested) narrows the aggregate to only
// the events that can contribute a country, the same reasoning as
// RequireGeoIP's own doc comment.
//
// Found in production: the frontend used to derive this facet purely from
// the fetched entries page, same as every other facet before the backend
// aggregates existed - but geoip-bearing events (a UniFi CEF threat/block
// with a public src or dst IP) are a small fraction of any default search's
// results, so that derivation came back empty in ordinary use even though
// enrich_geo was enriching correctly. This aggregate is a real Loki-side
// count over the full time range, not just whatever geoip-bearing lines
// happened to land in the current page.
func (s *Server) queryCountryFacetCounts(ctx context.Context, filters loki.Filters, start, end time.Time) ([]facetCount, error) {
	facetFilters := filters
	facetFilters.RequireGeoIP = true
	logql := loki.BuildQuery(s.deps.JobLabel, facetFilters)

	window := end.Sub(start)
	if window <= 0 {
		return []facetCount{}, nil
	}
	aggLogQL := fmt.Sprintf("sum by (cc) (count_over_time(%s[%ds]))", logql, int64(math.Ceil(window.Seconds())))

	result, err := s.deps.Loki.QueryInstant(ctx, aggLogQL, end)
	if err != nil {
		return nil, err
	}

	counts := make([]facetCount, 0, len(result.Series))
	for _, series := range result.Series {
		value := series.Labels["cc"]
		if value == "" || len(series.Samples) == 0 {
			continue
		}
		counts = append(counts, facetCount{Value: value, Count: int64(series.Samples[0].Value)})
	}
	sort.Slice(counts, func(i, j int) bool { return counts[i].Count > counts[j].Count })
	return counts, nil
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
