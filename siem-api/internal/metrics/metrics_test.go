package metrics

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestRenderSources(t *testing.T) {
	seen := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	out := RenderSources([]Source{
		{Name: "udm-ultra", Claimed: true, Up: true, HeartbeatSec: 900, LastSeenAt: &seen, EventsPerMin: 12.5},
		{Name: "pending-box", Claimed: false, Up: false, HeartbeatSec: 300, LastSeenAt: nil, EventsPerMin: 0},
	})

	wantLines := []string{
		`siem_source_up{source="udm-ultra"} 1`,
		`siem_source_up{source="pending-box"} 0`,
		`siem_source_claimed{source="udm-ultra"} 1`,
		`siem_source_claimed{source="pending-box"} 0`,
		`siem_source_heartbeat_seconds{source="udm-ultra"} 900`,
		`siem_source_heartbeat_seconds{source="pending-box"} 300`,
		fmt.Sprintf(`siem_source_last_seen_timestamp_seconds{source="udm-ultra"} %d`, seen.Unix()),
		`siem_source_events_per_minute{source="udm-ultra"} 12.5`,
		`siem_source_events_per_minute{source="pending-box"} 0`,
	}
	for _, want := range wantLines {
		if !strings.Contains(out, want) {
			t.Errorf("RenderSources() missing line %q, got:\n%s", want, out)
		}
	}

	if strings.Contains(out, "siem_source_last_seen_timestamp_seconds{source=\"pending-box\"}") {
		t.Errorf("RenderSources() should omit last_seen for a source with no LastSeenAt, got:\n%s", out)
	}

	// sources are sorted by name, so pending-box (before udm-ultra
	// alphabetically) must come first within each metric block.
	if strings.Index(out, `source="pending-box"`) > strings.Index(out, `source="udm-ultra"`) {
		t.Errorf("RenderSources() expected pending-box before udm-ultra, got:\n%s", out)
	}
}

func TestRenderSourcesEscapesLabelValues(t *testing.T) {
	out := RenderSources([]Source{
		{Name: `weird"name\with` + "\n" + `stuff`, Up: true, HeartbeatSec: 60},
	})
	want := `siem_source_up{source="weird\"name\\with\nstuff"} 1`
	if !strings.Contains(out, want) {
		t.Errorf("RenderSources() missing escaped line %q, got:\n%s", want, out)
	}
}

func TestRenderSourcesEmpty(t *testing.T) {
	out := RenderSources(nil)
	if !strings.Contains(out, "# HELP siem_source_up") {
		t.Errorf("RenderSources(nil) should still emit HELP/TYPE headers, got:\n%s", out)
	}
}
