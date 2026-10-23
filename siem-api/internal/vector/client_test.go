package vector

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Verbatim shape of Vector 0.60's prometheus_exporter output (trailing
// millisecond timestamp, `output` label on sources/transforms).
const sampleExposition = `# HELP vector_component_received_events_total component_received_events_total
# TYPE vector_component_received_events_total counter
vector_component_received_events_total{component_id="unifi",component_kind="source",component_type="syslog",host="ab12"} 1234 1792786573500
vector_component_sent_events_total{component_id="loki",component_kind="sink",component_type="loki",host="ab12"} 77 1792786573500
vector_component_sent_events_total{component_id="fast_path",component_kind="transform",component_type="route",host="ab12",output="alerts"} 5 1792786573500
vector_uptime_seconds{host="ab12"} 42.5
`

func TestMetrics_ParsesAndFiltersByName(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/metrics" {
			t.Errorf("request = %s %s, want GET /metrics", r.Method, r.URL.Path)
		}
		w.Write([]byte(sampleExposition))
	}))
	defer srv.Close()

	c := New(srv.URL, srv.Client())
	got, err := c.Metrics(context.Background(), "vector_component_received_events_total", "vector_component_sent_events_total")
	if err != nil {
		t.Fatalf("Metrics() error = %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len(Metrics()) = %d, want 3 (vector_uptime_seconds filtered out): %+v", len(got), got)
	}
	if got[0].Name != "vector_component_received_events_total" || got[0].Labels["component_id"] != "unifi" || got[0].Value != 1234 {
		t.Fatalf("got[0] = %+v", got[0])
	}
	if got[2].Labels["output"] != "alerts" || got[2].Value != 5 {
		t.Fatalf("got[2] = %+v", got[2])
	}
}

func TestMetrics_NoNamesReturnsEverything(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(sampleExposition))
	}))
	defer srv.Close()

	got, err := New(srv.URL, srv.Client()).Metrics(context.Background())
	if err != nil {
		t.Fatalf("Metrics() error = %v", err)
	}
	if len(got) != 4 || got[3].Name != "vector_uptime_seconds" || got[3].Value != 42.5 {
		t.Fatalf("Metrics() = %+v, want 4 samples ending in vector_uptime_seconds=42.5", got)
	}
}

func TestMetrics_SurfacesHTTPErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	if _, err := New(srv.URL, srv.Client()).Metrics(context.Background()); err == nil {
		t.Fatal("Metrics() error = nil, want error for a non-200 response")
	}
}

func TestMetrics_SurfacesMalformedLines(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("vector_component_sent_events_total{component_id=\"loki\" 7\n"))
	}))
	defer srv.Close()

	if _, err := New(srv.URL, srv.Client()).Metrics(context.Background()); err == nil {
		t.Fatal("Metrics() error = nil, want error for an unterminated label set")
	}
}

func TestParseSample_EscapedLabelValue(t *testing.T) {
	s, err := parseSample(`m{a="x\"y\\z",b="n\nl"} 3`)
	if err != nil {
		t.Fatalf("parseSample() error = %v", err)
	}
	if s.Labels["a"] != `x"y\z` || s.Labels["b"] != "n\nl" || s.Value != 3 {
		t.Fatalf("parseSample() = %+v", s)
	}
}
