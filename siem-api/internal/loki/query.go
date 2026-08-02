package loki

import (
	"fmt"
	"sort"
	"strings"
)

type Filters struct {
	Source   string
	Host     string
	Program  string
	Severity string
	Facility string
	Extra    map[string]string // non-label fields, e.g. src_ip — always emitted as filters, never as labels
	FreeText string
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
			query += fmt.Sprintf(` | %s=%q`, k, f.Extra[k])
		}
	}

	if f.FreeText != "" {
		query += fmt.Sprintf(` |= %q`, f.FreeText)
	}

	return query
}
