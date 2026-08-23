package loki

import (
	"strings"
	"testing"
)

func TestBuildQuery_JobLabelOnly(t *testing.T) {
	got := BuildQuery("siem", Filters{})
	want := `{job="siem",internal_noise!="true"}`
	if got != want {
		t.Errorf("BuildQuery() = %q, want %q", got, want)
	}
}

func TestBuildQuery_MandatedLabels(t *testing.T) {
	got := BuildQuery("siem", Filters{Source: "udm-ultra", Severity: "critical"})
	want := `{job="siem",source="udm-ultra",severity="critical",internal_noise!="true"}`
	if got != want {
		t.Errorf("BuildQuery() = %q, want %q", got, want)
	}
}

// Regression: facility is never promoted to a Loki stream label (see
// siem-ingest/vector.toml's sinks.loki.labels), so folding it into the
// label selector matched zero streams for every facility value entered.
// Found in production against live Loki data.
func TestBuildQuery_FacilityNeverBecomesALabel(t *testing.T) {
	got := BuildQuery("siem", Filters{Source: "udm-ultra", Facility: "user"})

	braceEnd := strings.Index(got, "}")
	if braceEnd < 0 {
		t.Fatalf("BuildQuery() = %q, no closing brace found", got)
	}
	labelPart := got[:braceEnd]
	if strings.Contains(labelPart, "facility") {
		t.Errorf("BuildQuery() label selector %q contains facility — facility must never become a label", labelPart)
	}

	rest := got[braceEnd:]
	if !strings.Contains(rest, `| json`) || !strings.Contains(rest, `facility="user"`) {
		t.Errorf("BuildQuery() = %q, want a json filter for facility after the label selector", got)
	}
}

func TestBuildQuery_ExtraFieldsNeverBecomeLabels(t *testing.T) {
	got := BuildQuery("siem", Filters{Source: "udm-ultra", Extra: map[string]string{"src_ip": "10.0.0.5"}})

	braceEnd := strings.Index(got, "}")
	if braceEnd < 0 {
		t.Fatalf("BuildQuery() = %q, no closing brace found", got)
	}
	labelPart := got[:braceEnd]
	if strings.Contains(labelPart, "src_ip") {
		t.Errorf("BuildQuery() label selector %q contains src_ip — Extra fields must never become labels", labelPart)
	}

	rest := got[braceEnd:]
	if !strings.Contains(rest, `| json`) || !strings.Contains(rest, `src_ip="10.0.0.5"`) {
		t.Errorf("BuildQuery() = %q, want a json filter for src_ip after the label selector", got)
	}
}

func TestBuildQuery_ExtraFieldsSortedDeterministically(t *testing.T) {
	got := BuildQuery("siem", Filters{Extra: map[string]string{"rule": "wan-portscan", "dst_port": "22"}})
	dstIdx := strings.Index(got, "dst_port")
	ruleIdx := strings.Index(got, `rule=`)
	if dstIdx < 0 || ruleIdx < 0 || dstIdx > ruleIdx {
		t.Errorf("BuildQuery() = %q, want dst_port before rule (alphabetical)", got)
	}
}

func TestBuildQuery_FreeText(t *testing.T) {
	got := BuildQuery("siem", Filters{FreeText: "timeout"})
	want := `{job="siem",internal_noise!="true"} |= "timeout"`
	if got != want {
		t.Errorf("BuildQuery() = %q, want %q", got, want)
	}
}

func TestBuildQuery_RequireGeoIP(t *testing.T) {
	got := BuildQuery("siem", Filters{RequireGeoIP: true})
	want := `{job="siem",internal_noise!="true"} | json cc="geoip.country_code" | cc != ""`
	if got != want {
		t.Errorf("BuildQuery() = %q, want %q", got, want)
	}
}

func TestBuildQuery_RequireGeoIP_ComposesAfterOtherFilters(t *testing.T) {
	got := BuildQuery("siem", Filters{Source: "udm-ultra", FreeText: "blocked", RequireGeoIP: true})
	want := `{job="siem",source="udm-ultra",internal_noise!="true"} |= "blocked" | json cc="geoip.country_code" | cc != ""`
	if got != want {
		t.Errorf("BuildQuery() = %q, want %q", got, want)
	}
}

func TestBuildQuery_ExtraFieldsRejectInvalidKeyNames(t *testing.T) {
	got := BuildQuery("siem", Filters{Extra: map[string]string{
		`src_ip" | label_format evil=`: "10.0.0.5",
		"dst_port":                     "22",
	}})
	if strings.Contains(got, "label_format") {
		t.Errorf("BuildQuery() = %q, want the malicious key rejected, not interpolated", got)
	}
	if !strings.Contains(got, `dst_port="22"`) {
		t.Errorf("BuildQuery() = %q, want the valid key still present", got)
	}
}

// TestBuildQuery_ExcludesInternalNoiseByDefault is the regression test for
// the actual production finding: Loki's own query-engine debug output
// alone accounted for the large majority of ingested volume in a live
// sample, burying real signal from monitored infrastructure in every
// caller that builds a query via Filters without explicitly opting in -
// Search, the Wall/Live-tail poller, and the Insights prompt builder all
// get this exclusion "for free" from Filters' zero value.
func TestBuildQuery_ExcludesInternalNoiseByDefault(t *testing.T) {
	got := BuildQuery("siem", Filters{})
	if !strings.Contains(got, `internal_noise!="true"`) {
		t.Errorf("BuildQuery() = %q, want internal_noise excluded by default", got)
	}
}

func TestBuildQuery_IncludeInternal_OmitsTheExclusion(t *testing.T) {
	got := BuildQuery("siem", Filters{IncludeInternal: true})
	want := `{job="siem"}`
	if got != want {
		t.Errorf("BuildQuery() = %q, want %q (no internal_noise term at all)", got, want)
	}
}
