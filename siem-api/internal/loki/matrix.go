package loki

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

type MatrixSample struct {
	Timestamp time.Time
	Value     float64
}

type MatrixSeries struct {
	Labels  map[string]string
	Samples []MatrixSample
}

type MatrixResult struct {
	Series []MatrixSeries
}

type queryMatrixResponse struct {
	Status string `json:"status"`
	Data   struct {
		Result []struct {
			Metric map[string]string `json:"metric"`
			Values [][2]any          `json:"values"`
		} `json:"result"`
	} `json:"data"`
	Error string `json:"error"`
}

// QueryMatrix runs a LogQL metric query (e.g. count_over_time) against
// Loki's query_range endpoint and parses the resulting Prometheus-style
// matrix, as opposed to QueryRange's log-stream parsing. Loki reports
// matrix sample timestamps in (possibly fractional) Unix seconds, unlike
// the nanosecond-string timestamps QueryRange handles for log streams.
func (c *Client) QueryMatrix(ctx context.Context, logql string, start, end time.Time, step time.Duration) (MatrixResult, error) {
	q := url.Values{}
	q.Set("query", logql)
	q.Set("start", strconv.FormatInt(start.UnixNano(), 10))
	q.Set("end", strconv.FormatInt(end.UnixNano(), 10))
	q.Set("step", strconv.FormatFloat(step.Seconds(), 'f', -1, 64))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.baseURL+"/loki/api/v1/query_range?"+q.Encode(), nil)
	if err != nil {
		return MatrixResult{}, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return MatrixResult{}, fmt.Errorf("loki: query_range (matrix) request: %w", err)
	}
	defer resp.Body.Close()

	var parsed queryMatrixResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return MatrixResult{}, fmt.Errorf("loki: decode matrix response: %w", err)
	}
	if resp.StatusCode != http.StatusOK || parsed.Status != "success" {
		return MatrixResult{}, fmt.Errorf("loki: query_range (matrix) failed: status=%d error=%q", resp.StatusCode, parsed.Error)
	}

	var series []MatrixSeries
	for _, s := range parsed.Data.Result {
		var samples []MatrixSample
		for _, v := range s.Values {
			tsSeconds, ok := v[0].(float64)
			if !ok {
				return MatrixResult{}, fmt.Errorf("loki: unexpected matrix timestamp type %T", v[0])
			}
			valStr, ok := v[1].(string)
			if !ok {
				return MatrixResult{}, fmt.Errorf("loki: unexpected matrix value type %T", v[1])
			}
			value, err := strconv.ParseFloat(valStr, 64)
			if err != nil {
				return MatrixResult{}, fmt.Errorf("loki: parse matrix value %q: %w", valStr, err)
			}
			samples = append(samples, MatrixSample{
				Timestamp: time.Unix(int64(tsSeconds), 0).UTC(),
				Value:     value,
			})
		}
		series = append(series, MatrixSeries{Labels: s.Metric, Samples: samples})
	}

	return MatrixResult{Series: series}, nil
}
