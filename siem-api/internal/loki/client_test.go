package loki

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestQueryRange_ParsesAndSortsEntries(t *testing.T) {
	var gotQuery url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"status": "success",
			"data": {
				"resultType": "streams",
				"result": [
					{
						"stream": {"job": "siem", "source": "udm-ultra"},
						"values": [["1700000002000000000", "second line"], ["1700000000000000000", "first line"]]
					},
					{
						"stream": {"job": "siem", "source": "host-1"},
						"values": [["1700000001000000000", "middle line"]]
					}
				]
			}
		}`))
	}))
	defer srv.Close()

	c := New(srv.URL, srv.Client())
	start := time.Unix(1700000000, 0)
	end := time.Unix(1700000010, 0)
	result, err := c.QueryRange(context.Background(), `{job="siem"}`, start, end, 100)
	if err != nil {
		t.Fatalf("QueryRange() error = %v", err)
	}

	if gotQuery.Get("query") != `{job="siem"}` {
		t.Errorf("query param = %q", gotQuery.Get("query"))
	}
	if gotQuery.Get("limit") != "100" {
		t.Errorf("limit param = %q, want 100", gotQuery.Get("limit"))
	}

	if len(result.Entries) != 3 {
		t.Fatalf("len(Entries) = %d, want 3", len(result.Entries))
	}
	if result.Entries[0].Line != "first line" {
		t.Errorf("Entries[0].Line = %q, want first line (entries must be time-sorted ascending)", result.Entries[0].Line)
	}
	if result.Entries[1].Line != "middle line" {
		t.Errorf("Entries[1].Line = %q, want middle line", result.Entries[1].Line)
	}
	if result.Entries[2].Line != "second line" {
		t.Errorf("Entries[2].Line = %q, want second line", result.Entries[2].Line)
	}
	if result.Entries[0].Labels["source"] != "udm-ultra" {
		t.Errorf("Labels[source] = %q, want udm-ultra", result.Entries[0].Labels["source"])
	}
}

func TestQueryRange_NoMatchesReturnsEmptyNotNilEntries(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status": "success", "data": {"resultType": "streams", "result": []}}`))
	}))
	defer srv.Close()

	c := New(srv.URL, srv.Client())
	result, err := c.QueryRange(context.Background(), `{job="siem"}`, time.Now(), time.Now(), 100)
	if err != nil {
		t.Fatalf("QueryRange() error = %v", err)
	}

	// siem-api JSON-encodes this directly as the search response's "entries"
	// field; a nil slice marshals to `null`, which siem-web's `for (const
	// entry of entries)` throws TypeError on. Must stay a non-nil empty slice
	// so the field marshals to `[]`.
	if result.Entries == nil {
		t.Fatal("Entries = nil, want non-nil empty slice (marshals to JSON null, crashes siem-web callers)")
	}
	if len(result.Entries) != 0 {
		t.Fatalf("len(Entries) = %d, want 0", len(result.Entries))
	}
}

func TestQueryRange_NonSuccessStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"status":"error","error":"boom"}`))
	}))
	defer srv.Close()

	c := New(srv.URL, srv.Client())
	if _, err := c.QueryRange(context.Background(), `{job="siem"}`, time.Now(), time.Now(), 10); err == nil {
		t.Fatal("QueryRange() error = nil, want error for 500 response")
	}
}
