package loki

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"time"
)

type LogEntry struct {
	Timestamp time.Time
	Labels    map[string]string
	Line      string
}

type QueryResult struct {
	Entries []LogEntry
}

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

type queryRangeResponse struct {
	Status string `json:"status"`
	Data   struct {
		Result []struct {
			Stream map[string]string `json:"stream"`
			Values [][2]string        `json:"values"`
		} `json:"result"`
	} `json:"data"`
	Error string `json:"error"`
}

func (c *Client) QueryRange(ctx context.Context, logql string, start, end time.Time, limit int) (QueryResult, error) {
	q := url.Values{}
	q.Set("query", logql)
	q.Set("start", strconv.FormatInt(start.UnixNano(), 10))
	q.Set("end", strconv.FormatInt(end.UnixNano(), 10))
	q.Set("limit", strconv.Itoa(limit))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.baseURL+"/loki/api/v1/query_range?"+q.Encode(), nil)
	if err != nil {
		return QueryResult{}, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return QueryResult{}, fmt.Errorf("loki: query_range request: %w", err)
	}
	defer resp.Body.Close()

	var parsed queryRangeResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return QueryResult{}, fmt.Errorf("loki: decode response: %w", err)
	}
	if resp.StatusCode != http.StatusOK || parsed.Status != "success" {
		return QueryResult{}, fmt.Errorf("loki: query_range failed: status=%d error=%q", resp.StatusCode, parsed.Error)
	}

	var entries []LogEntry
	for _, stream := range parsed.Data.Result {
		for _, v := range stream.Values {
			nanos, err := strconv.ParseInt(v[0], 10, 64)
			if err != nil {
				return QueryResult{}, fmt.Errorf("loki: parse timestamp %q: %w", v[0], err)
			}
			entries = append(entries, LogEntry{
				Timestamp: time.Unix(0, nanos).UTC(),
				Labels:    stream.Stream,
				Line:      v[1],
			})
		}
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Timestamp.Before(entries[j].Timestamp)
	})

	return QueryResult{Entries: entries}, nil
}
