package ntfy

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPublish_SendsExpectedRequest(t *testing.T) {
	var gotPath, gotTitle, gotPriority, gotAuth, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotTitle = r.Header.Get("Title")
		gotPriority = r.Header.Get("Priority")
		gotAuth = r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := New(srv.URL, "homesiem", "ntfy-token", srv.Client())
	err := c.Publish(context.Background(), "Port scan detected", "40 dropped connections from 10.0.0.5", "high")
	if err != nil {
		t.Fatalf("Publish() error = %v", err)
	}

	if gotPath != "/homesiem" {
		t.Errorf("path = %q, want /homesiem", gotPath)
	}
	if gotTitle != "Port scan detected" {
		t.Errorf("Title header = %q", gotTitle)
	}
	if gotPriority != "high" {
		t.Errorf("Priority header = %q, want high", gotPriority)
	}
	if gotAuth != "Bearer ntfy-token" {
		t.Errorf("Authorization header = %q, want Bearer ntfy-token", gotAuth)
	}
	if gotBody != "40 dropped connections from 10.0.0.5" {
		t.Errorf("body = %q", gotBody)
	}
}

func TestPublish_NoTokenOmitsAuthHeader(t *testing.T) {
	var gotAuth string
	hadAuth := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth, hadAuth = r.Header.Get("Authorization"), r.Header.Get("Authorization") != ""
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := New(srv.URL, "homesiem", "", srv.Client())
	if err := c.Publish(context.Background(), "t", "m", "default"); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	if hadAuth {
		t.Errorf("Authorization header = %q, want absent when no token configured", gotAuth)
	}
}

func TestPublish_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := New(srv.URL, "homesiem", "", srv.Client())
	if err := c.Publish(context.Background(), "t", "m", "default"); err == nil {
		t.Fatal("Publish() error = nil, want error for 500 response")
	}
}
