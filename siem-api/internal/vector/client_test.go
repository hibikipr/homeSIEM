package vector

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestQuery_ReturnsData(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/graphql" {
			t.Errorf("request = %s %s, want POST /graphql", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":{"sources":{"nodes":[{"componentId":"unifi"}]}}}`))
	}))
	defer srv.Close()

	c := New(srv.URL, srv.Client())
	data, err := c.Query(context.Background(), `{ sources { nodes { componentId } } }`)
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if !strings.Contains(string(data), `"componentId":"unifi"`) {
		t.Fatalf("data = %s, want it to contain componentId unifi", data)
	}
}

func TestQuery_SurfacesGraphQLErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":null,"errors":[{"message":"field not found"}]}`))
	}))
	defer srv.Close()

	c := New(srv.URL, srv.Client())
	if _, err := c.Query(context.Background(), `{ bogus }`); err == nil {
		t.Fatal("Query() error = nil, want error for a GraphQL errors[] response")
	}
}

func TestQuery_SurfacesHTTPErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	c := New(srv.URL, srv.Client())
	if _, err := c.Query(context.Background(), `{ sources { nodes { componentId } } }`); err == nil {
		t.Fatal("Query() error = nil, want error for a non-200 response")
	}
}
