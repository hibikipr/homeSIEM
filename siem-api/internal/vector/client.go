package vector

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// Client reads Vector's own component metrics from its prometheus_exporter
// sink (siem-ingest/vector.toml: sources.vector_metrics ->
// sinks.vector_metrics_exporter). Vector 0.60 replaced the GraphQL API this
// used to query with a gRPC one, so the Prometheus text endpoint is the
// stable, version-independent way to get the same counters.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

func New(baseURL string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{baseURL: baseURL, httpClient: httpClient}
}

// Sample is one line of Prometheus text exposition: a metric name, its
// labels and its value (the optional trailing timestamp is dropped).
type Sample struct {
	Name   string
	Labels map[string]string
	Value  float64
}

// Metrics GETs {baseURL}/metrics and returns every sample whose metric name
// is in names (all samples when names is empty).
func (c *Client) Metrics(ctx context.Context, names ...string) ([]Sample, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/metrics", nil)
	if err != nil {
		return nil, fmt.Errorf("vector: build request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("vector: request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("vector: request failed: status=%d", resp.StatusCode)
	}

	want := make(map[string]bool, len(names))
	for _, n := range names {
		want[n] = true
	}

	var samples []Sample
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		s, err := parseSample(line)
		if err != nil {
			return nil, fmt.Errorf("vector: parse metrics: %w", err)
		}
		if len(want) == 0 || want[s.Name] {
			samples = append(samples, s)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("vector: read metrics: %w", err)
	}
	return samples, nil
}

// parseSample parses `name{k="v",...} value [timestamp]` (labels optional).
func parseSample(line string) (Sample, error) {
	s := Sample{Labels: map[string]string{}}

	nameEnd := strings.IndexAny(line, "{ ")
	if nameEnd <= 0 {
		return s, fmt.Errorf("no value in %q", line)
	}
	s.Name = line[:nameEnd]
	rest := line[nameEnd:]

	if rest[0] == '{' {
		i := 1
		for {
			for i < len(rest) && (rest[i] == ',' || rest[i] == ' ') {
				i++
			}
			if i >= len(rest) {
				return s, fmt.Errorf("unterminated labels in %q", line)
			}
			if rest[i] == '}' {
				i++
				break
			}
			eq := strings.IndexByte(rest[i:], '=')
			if eq < 0 || i+eq+1 >= len(rest) || rest[i+eq+1] != '"' {
				return s, fmt.Errorf("malformed label in %q", line)
			}
			key := rest[i : i+eq]
			i += eq + 2
			var val strings.Builder
			for ; i < len(rest) && rest[i] != '"'; i++ {
				if rest[i] == '\\' && i+1 < len(rest) {
					i++
					switch rest[i] {
					case 'n':
						val.WriteByte('\n')
					default:
						val.WriteByte(rest[i])
					}
					continue
				}
				val.WriteByte(rest[i])
			}
			if i >= len(rest) {
				return s, fmt.Errorf("unterminated label value in %q", line)
			}
			i++ // closing quote
			s.Labels[key] = val.String()
		}
		rest = rest[i:]
	}

	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return s, fmt.Errorf("no value in %q", line)
	}
	v, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return s, fmt.Errorf("bad value in %q: %w", line, err)
	}
	s.Value = v
	return s, nil
}
