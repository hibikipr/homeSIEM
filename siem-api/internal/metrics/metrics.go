// Package metrics renders per-source Prometheus text-exposition metrics for
// the Wall. It has no dependency on api or store so it can be unit tested
// against plain structs, and no third-party client library - the exposition
// format is a handful of "name{labels} value" lines, not worth pulling in
// prometheus/client_golang's dependency tree for.
package metrics

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Source is the subset of a source's state the Wall's per-source metrics are
// derived from - deliberately not store.Source itself, so this package
// doesn't need to import store.
type Source struct {
	Name         string
	Claimed      bool
	Up           bool // sourceStatus(src, now) == "healthy"
	HeartbeatSec int
	LastSeenAt   *time.Time
	EventsPerMin float64 // 0 when Loki isn't configured or the query failed
}

// RenderSources writes Prometheus text-exposition format for one source's
// worth of gauges per entry in sources, sorted by name so scrapes diff
// cleanly across polls.
func RenderSources(sources []Source) string {
	sorted := make([]Source, len(sources))
	copy(sorted, sources)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })

	var b strings.Builder
	writeHelp(&b, "siem_source_up", "gauge",
		"Whether a source has checked in within its configured heartbeat window (1) or is silent (0).")
	for _, s := range sorted {
		fmt.Fprintf(&b, "siem_source_up{source=\"%s\"} %s\n", escapeLabel(s.Name), boolValue(s.Up))
	}

	writeHelp(&b, "siem_source_claimed", "gauge", "Whether a source has been claimed by an operator (1) or is still pending (0).")
	for _, s := range sorted {
		fmt.Fprintf(&b, "siem_source_claimed{source=\"%s\"} %s\n", escapeLabel(s.Name), boolValue(s.Claimed))
	}

	writeHelp(&b, "siem_source_heartbeat_seconds", "gauge", "Configured heartbeat interval - how long a source may go quiet before it's flagged silent.")
	for _, s := range sorted {
		fmt.Fprintf(&b, "siem_source_heartbeat_seconds{source=\"%s\"} %d\n", escapeLabel(s.Name), s.HeartbeatSec)
	}

	writeHelp(&b, "siem_source_last_seen_timestamp_seconds", "gauge", "Unix timestamp of the last event or heartbeat received from a source.")
	for _, s := range sorted {
		if s.LastSeenAt == nil {
			continue
		}
		fmt.Fprintf(&b, "siem_source_last_seen_timestamp_seconds{source=\"%s\"} %d\n", escapeLabel(s.Name), s.LastSeenAt.UTC().Unix())
	}

	writeHelp(&b, "siem_source_events_per_minute", "gauge", "Rolling 5-minute events/min rate for a source, as shown on the Wall (0 when Loki isn't configured).")
	for _, s := range sorted {
		fmt.Fprintf(&b, "siem_source_events_per_minute{source=\"%s\"} %s\n", escapeLabel(s.Name), formatFloat(s.EventsPerMin))
	}

	return b.String()
}

func writeHelp(b *strings.Builder, name, metricType, help string) {
	fmt.Fprintf(b, "# HELP %s %s\n", name, help)
	fmt.Fprintf(b, "# TYPE %s %s\n", name, metricType)
}

func boolValue(v bool) string {
	if v {
		return "1"
	}
	return "0"
}

func formatFloat(v float64) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.6f", v), "0"), ".")
}

// escapeLabel escapes a label value per the Prometheus text format: a
// backslash, then a double quote, then a newline, each in that order so a
// backslash introduced by an earlier substitution isn't re-escaped.
func escapeLabel(v string) string {
	v = strings.ReplaceAll(v, `\`, `\\`)
	v = strings.ReplaceAll(v, `"`, `\"`)
	v = strings.ReplaceAll(v, "\n", `\n`)
	return v
}
