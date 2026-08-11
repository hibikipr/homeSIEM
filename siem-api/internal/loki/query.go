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
}

func BuildQuery(jobLabel string, f Filters) string {
	labels := []string{fmt.Sprintf("job=%q", jobLabel)}
	for _, pair := range []struct{ name, value string }{
		{"source", f.Source},
		{"host", f.Host},
		{"program", f.Program},
		{"severity", f.Severity},
		{"facility", f.Facility},
	} {
		if pair.value != "" {
			labels = append(labels, fmt.Sprintf("%s=%q", pair.name, pair.value))
		}
	}
	query := "{" + strings.Join(labels, ",") + "}"

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
