package loki

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var validFieldName = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_.]*$`)

type Filters struct {
	Source   string
	Host     string
	Program  string
	Severity string
	// Facility is never promoted to a Loki stream label (see
	// siem-ingest/vector.toml's sinks.loki.labels, which only sets job,
	// source, host, program, severity) - it only exists inside each line's
	// own JSON body. Found in production: BuildQuery used to fold this into
	// the label selector like the fields above, which silently matched zero
	// streams for every facility value ever entered. Must stay a
	// post-`| json` filter, not a label.
	Facility string
	Extra    map[string]string // non-label fields, e.g. src_ip — always emitted as filters, never as labels
	FreeText string
	// RequireGeoIP filters to only entries with a non-empty
	// geoip.country_code. Found in production: the Wall's country-
	// breakdown widget samples a small, recency-capped window of
	// {job="siem"} - geoip-bearing security events (a UniFi CEF threat/
	// block with a public src or dst IP) are a tiny fraction of this
	// host's overall log volume, so a plain recent sample almost never
	// contains one at all, even though enrich_geo is enriching them
	// correctly. Filtering at the LogQL level instead of after the fact
	// means the sample only ever spends its cap on entries that can
	// actually contribute a country.
	RequireGeoIP bool
	// IncludeInternal opts into this stack's own operational chatter and
	// other routine internal polling (Loki's own query-engine debug
	// output, docker-socket-proxy's container polling, homarr's Redis
	// background-save noise - see enrich_geo's .internal_noise doc in
	// vector.toml) - excluded by default (the Go zero value is false)
	// everywhere a Filters gets built, not just here. Found in production:
	// Loki's own internals alone accounted for the large majority of
	// ingested volume, burying real signal from actual monitored
	// infrastructure in Search's default view, the Wall ticker, and the
	// Insights LLM's own input.
	IncludeInternal bool
}

func BuildQuery(jobLabel string, f Filters) string {
	labels := []string{fmt.Sprintf("job=%q", jobLabel)}
	for _, pair := range []struct{ name, value string }{
		{"source", f.Source},
		{"host", f.Host},
		{"program", f.Program},
		{"severity", f.Severity},
	} {
		if pair.value != "" {
			labels = append(labels, fmt.Sprintf("%s=%q", pair.name, pair.value))
		}
	}
	if !f.IncludeInternal {
		labels = append(labels, `internal_noise!="true"`)
	}
	query := "{" + strings.Join(labels, ",") + "}"

	if f.Facility != "" {
		query += fmt.Sprintf(` | json | facility=%q`, f.Facility)
	}

	if len(f.Extra) > 0 {
		keys := make([]string, 0, len(f.Extra))
		for k := range f.Extra {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		query += " | json"
		for _, k := range keys {
			if !validFieldName.MatchString(k) {
				continue // skip invalid field names; never interpolate unvalidated keys
			}
			query += fmt.Sprintf(` | %s=%q`, k, f.Extra[k])
		}
	}

	if f.FreeText != "" {
		query += fmt.Sprintf(` |= %q`, f.FreeText)
	}

	if f.RequireGeoIP {
		query += ` | json cc="geoip.country_code" | cc != ""`
	}

	return query
}
