package loki

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestQueryMatrix_ParsesMatrixResult(t *testing.T) {
	var gotQuery url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"status": "success",
			"data": {
				"resultType": "matrix",
				"result": [
					{
						"metric": {"source": "udm-ultra"},
						"values": [[1700000000, "5"], [1700003600, "3.5"]]
					},
					{
						"metric": {"source": "host-1"},
						"values": [[1700000000, "0"]]
					}
				]
			}
		}`))
	}))
	defer srv.Close()

	c := New(srv.URL, srv.Client())
	start := time.Unix(1700000000, 0)
	end := time.Unix(1700003600, 0)
	result, err := c.QueryMatrix(context.Background(), `sum by (source) (count_over_time({job="siem"}[1h]))`, start, end, time.Hour)
	if err != nil {
		t.Fatalf("QueryMatrix() error = %v", err)
	}

	if gotQuery.Get("query") != `sum by (source) (count_over_time({job="siem"}[1h]))` {
		t.Errorf("query param = %q", gotQuery.Get("query"))
	}
	if gotQuery.Get("step") != "3600" {
		t.Errorf("step param = %q, want 3600 (seconds)", gotQuery.Get("step"))
	}

	if len(result.Series) != 2 {
		t.Fatalf("len(Series) = %d, want 2", len(result.Series))
	}

	udm := result.Series[0]
	if udm.Labels["source"] != "udm-ultra" {
		t.Errorf("Series[0].Labels[source] = %q, want udm-ultra", udm.Labels["source"])
	}
	if len(udm.Samples) != 2 {
		t.Fatalf("len(Series[0].Samples) = %d, want 2", len(udm.Samples))
	}
	if udm.Samples[0].Value != 5 {
		t.Errorf("Samples[0].Value = %v, want 5", udm.Samples[0].Value)
	}
	if udm.Samples[1].Value != 3.5 {
		t.Errorf("Samples[1].Value = %v, want 3.5", udm.Samples[1].Value)
	}
	if !udm.Samples[0].Timestamp.Equal(time.Unix(1700000000, 0).UTC()) {
		t.Errorf("Samples[0].Timestamp = %v, want %v", udm.Samples[0].Timestamp, time.Unix(1700000000, 0).UTC())
	}
}

func TestQueryMatrix_NonSuccessStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"status":"error","error":"boom"}`))
	}))
	defer srv.Close()

	c := New(srv.URL, srv.Client())
	if _, err := c.QueryMatrix(context.Background(), `sum(count_over_time({job="siem"}[1h]))`, time.Now(), time.Now(), time.Hour); err == nil {
		t.Fatal("QueryMatrix() error = nil, want error for 500 response")
	}
}
