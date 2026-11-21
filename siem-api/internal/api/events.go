package api

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"
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

// maxSearchLimit caps how many raw entries one search may ask Loki for.
// Loki rejects anything over its max_entries_limit_per_query (the homelab
// config sets 10000) with an error this handler would surface as a 502,
// and even below that, thousands of full enriched lines were measured
// timing out Loki on unfiltered 24h searches - so larger requests are
// clamped rather than passed through.
const maxSearchLimit = 5000

// searchParams is the fully parsed, normalized form of the search query
// string; it is both what the Loki queries run from and the cache key source.
type searchParams struct {
	filters        loki.Filters
	start, end     time.Time // zero when the caller omitted (or sent an unparsable) value: window is relative to "now"
	limit          int
	includeVolume  bool
	includeEntries bool
	includeCount   bool
	facets         []string
}

// cacheKey is the normalized-parameter key for the response cache. Every
// field that affects the Loki queries is included; an omitted start/end is
// the literal "rel", so all "default / last 24h" requests with the same
// filters share one entry for searchCacheTTL (up to 30s of staleness is the
// accepted trade-off for relative windows). Absolute start/end are keyed by
// their exact instants - those ranges don't shift, so equal keys are equal
// queries. Facets are keyed order-insensitively.
func (p searchParams) cacheKey() string {
	rng := func(t time.Time) string {
		if t.IsZero() {
			return "rel"
		}
		return t.UTC().Format(time.RFC3339Nano)
	}
	facets := slices.Clone(p.facets)
	sort.Strings(facets)
	return fmt.Sprintf("search\x00%#v\x00%s\x00%s\x00%d\x00%t%t%t\x00%s",
		p.filters, rng(p.start), rng(p.end), p.limit,
		p.includeVolume, p.includeEntries, p.includeCount, strings.Join(facets, ","))
}

// handleEventsSearch serves from a 30s cache keyed by the full normalized
// parameters (searchParams.cacheKey). The response depends only on those
// parameters - not on the caller (the route only enforces a minimum role) -
// so entries are shared across users. Errors and degraded responses (a
// failed volume or facets sub-query) are served but never cached.
func (s *Server) handleEventsSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	p := searchParams{
		filters: loki.Filters{
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
		},
		limit: 1000,
	}
	if v := q.Get("start"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			p.start = t
		}
	}
	if v := q.Get("end"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			p.end = t
		}
	}
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			p.limit = min(n, maxSearchLimit)
		}
	}
	// includeVolume lets a caller that only wants entries (the Wall's use of
	// this endpoint to derive a country breakdown, never the volume
	// histogram) skip a whole extra Loki matrix query it would otherwise
	// discard. Defaults to true - the actual Search screen always wants both.
	p.includeVolume = q.Get("volume") != "false"
	// includeEntries lets a caller that only wants a total count (Search's
	// "N events from this IP in the last 24h" context callout, found
	// fetching up to 5000 full log entries - each the complete enriched
	// event JSON - over the network just to read len(entries)) skip the
	// entries query. Defaults to true - the actual Search screen always
	// wants entries. Count itself is always the real Loki-side aggregate
	// (see totalCount below), never derived from len(entries) - that used
	// to silently cap Count at `limit` too, presenting e.g. "1,000 events"
	// as if it were a total rather than a page size.
	p.includeEntries = q.Get("entries") != "false"
	// facets selects which real per-label aggregate counts to compute
	// alongside the main query - see queryFacetCounts and the Facets field
	// doc. Each facet is its own full-window Loki scan, so callers ask only
	// for what they render: "false" skips them all (the context-summary
	// lookup), a comma-separated subset like "country" computes just those
	// (the Wall's country breakdown), and absent/"true" computes every one
	// (the Search screen's facet rail).
	p.facets = parseFacetsParam(q.Get("facets"))
	// includeCount lets a caller that never reads Count (the Wall's
	// country breakdown) skip the full-window total-count scan. Defaults
	// to true; Count is reported as 0 when skipped.
	p.includeCount = q.Get("count") != "false"

	body, err := s.respCache.Do(r.Context(), p.cacheKey(), searchCacheTTL, func(ctx context.Context) ([]byte, bool, error) {
		return s.runSearch(ctx, p)
	})
	if err != nil {
		http.Error(w, "query failed", http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(body)
}

// runSearch executes the Loki queries for p and returns the encoded
// response. The bool is false when the response is degraded (a tolerated
// sub-query failed) and so must not be cached.
func (s *Server) runSearch(ctx context.Context, p searchParams) ([]byte, bool, error) {
	filters, limit := p.filters, p.limit
	includeVolume, includeEntries, includeCount := p.includeVolume, p.includeEntries, p.includeCount
	requestedFacets := p.facets
	includeFacets := len(requestedFacets) > 0
	logql := loki.BuildQuery(s.deps.JobLabel, filters)

	end := time.Now().UTC()
	start := end.Add(-24 * time.Hour)
	if !p.start.IsZero() {
		start = p.start
	}
	if !p.end.IsZero() {
		end = p.end
	}

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
			result, resultErr = s.deps.Loki.QueryRange(ctx, logql, start, end, limit)
		}()
	}

	if includeCount {
		wg.Add(1)
		go func() {
			defer wg.Done()
			totalCount, countErr = s.queryTotalCount(ctx, logql, start, end)
		}()
	}

	if includeVolume {
		wg.Add(1)
		go func() {
			defer wg.Done()
			volume, volumeErr = s.queryVolumeBuckets(ctx, logql, start, end)
		}()
	}

	if includeFacets {
		wg.Add(1)
		go func() {
			defer wg.Done()
			facets, facetsErr = s.queryFacets(ctx, filters, requestedFacets, start, end)
		}()
	}

	wg.Wait()

	if includeEntries && resultErr != nil {
		s.deps.Logger.Error("events search: query failed", "error", resultErr)
		return nil, false, resultErr
	}
	if countErr != nil {
		s.deps.Logger.Error("events search: count query failed", "error", countErr)
		return nil, false, countErr
	}
	degraded := false
	if volumeErr != nil {
		s.deps.Logger.Error("events search: volume query failed", "error", volumeErr)
		volume = nil
		degraded = true
	}
	if volume == nil {
		volume = []volumeBucket{}
	}
	if facetsErr != nil {
		s.deps.Logger.Error("events search: facets query failed", "error", facetsErr)
		facets = nil
		degraded = true
	}
	if facets == nil {
		facets = map[string][]facetCount{}
	}

	entries := result.Entries
	if entries == nil {
		entries = []loki.LogEntry{}
	}

	out, err := json.Marshal(searchResponse{
		LogQL:   logql,
		Count:   totalCount,
		Entries: entries,
		Volume:  volume,
		Facets:  facets,
	})
	return out, err == nil && !degraded, err
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

// countryFacet is aggregated from each line's geoip JSON rather than a
// stream label (see queryCountryFacetCounts), so it isn't in facetLabels.
const countryFacet = "country"

// allFacets is every facet the search endpoint can compute.
var allFacets = append(append([]string{}, facetLabels...), countryFacet)

// parseFacetsParam turns the `facets` query param into the set of facets
// to compute: "" or "true" means all of them, "false" none, otherwise a
// comma-separated list (unknown names ignored).
func parseFacetsParam(v string) []string {
	switch v {
	case "", "true":
		return allFacets
	case "false":
		return nil
	}
	var out []string
	for _, name := range strings.Split(v, ",") {
		name = strings.TrimSpace(name)
		if slices.Contains(allFacets, name) && !slices.Contains(out, name) {
			out = append(out, name)
		}
	}
	return out
}

// queryFacets runs one real Loki-side aggregate query per requested facet,
// concurrently, each with that facet's own filter excluded
// from the query it's computed with - so e.g. the severity facet reports
// counts across every severity given the other active filters, not just
// whichever severity happens to already be selected. A single facet's
// failure doesn't fail the others; it's simply omitted from the result.
func (s *Server) queryFacets(ctx context.Context, filters loki.Filters, requested []string, start, end time.Time) (map[string][]facetCount, error) {
	type facetResult struct {
		label  string
		counts []facetCount
		err    error
	}

	results := make(chan facetResult, len(requested))
	var wg sync.WaitGroup
	for _, label := range requested {
		wg.Add(1)
		go func(label string) {
			defer wg.Done()
			var counts []facetCount
			var err error
			if label == countryFacet {
				counts, err = s.queryCountryFacetCounts(ctx, filters, start, end)
			} else {
				counts, err = s.queryFacetCounts(ctx, filters, label, start, end)
			}
			results <- facetResult{label: label, counts: counts, err: err}
		}(label)
	}
	wg.Wait()
	close(results)

	out := make(map[string][]facetCount, len(requested))
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
