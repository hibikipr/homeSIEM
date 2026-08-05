package loki

import (
	"strings"
	"testing"
)

func TestBuildQuery_JobLabelOnly(t *testing.T) {
	got := BuildQuery("siem", Filters{})
	want := `{job="siem"}`
	if got != want {
		t.Errorf("BuildQuery() = %q, want %q", got, want)
	}
}

func TestBuildQuery_MandatedLabels(t *testing.T) {
	got := BuildQuery("siem", Filters{Source: "udm-ultra", Severity: "critical"})
	want := `{job="siem",source="udm-ultra",severity="critical"}`
	if got != want {
		t.Errorf("BuildQuery() = %q, want %q", got, want)
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
	want := `{job="siem"} |= "timeout"`
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
