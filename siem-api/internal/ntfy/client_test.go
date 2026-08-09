package ntfy

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPublish_SendsExpectedRequest(t *testing.T) {
	var gotPath, gotContentType, gotAuth string
	var gotPayload jsonPayload
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotContentType = r.Header.Get("Content-Type")
		gotAuth = r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &gotPayload); err != nil {
			t.Errorf("unmarshal request body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := New(srv.URL, "homesiem", "ntfy-token", srv.Client())
	err := c.Publish(context.Background(), Message{
		Title:    "Port scan detected",
		Body:     "40 dropped connections from 10.0.0.5",
		Priority: 4,
		Tags:     []string{"rotating_light"},
		Click:    "https://siem.example.com/alerts?id=42",
		Icon:     "https://siem.example.com/icons/homesiem-192.png",
		Actions:  []Action{{Label: "Open in homeSIEM", URL: "https://siem.example.com/alerts?id=42"}},
		Markdown: true,
	})
	if err != nil {
		t.Fatalf("Publish() error = %v", err)
	}

	// JSON publish goes to the server root with "topic" in the body, not
	// POSTed to /<topic> like the older header-based form.
	if gotPath != "/" {
		t.Errorf("path = %q, want /", gotPath)
	}
	if gotContentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", gotContentType)
	}
	if gotAuth != "Bearer ntfy-token" {
		t.Errorf("Authorization header = %q, want Bearer ntfy-token", gotAuth)
	}
	if gotPayload.Topic != "homesiem" {
		t.Errorf("payload.Topic = %q, want homesiem", gotPayload.Topic)
	}
	if gotPayload.Title != "Port scan detected" {
		t.Errorf("payload.Title = %q", gotPayload.Title)
	}
	if gotPayload.Message != "40 dropped connections from 10.0.0.5" {
		t.Errorf("payload.Message = %q", gotPayload.Message)
	}
	if gotPayload.Priority != 4 {
		t.Errorf("payload.Priority = %d, want 4", gotPayload.Priority)
	}
	if len(gotPayload.Tags) != 1 || gotPayload.Tags[0] != "rotating_light" {
		t.Errorf("payload.Tags = %v, want [rotating_light]", gotPayload.Tags)
	}
	if gotPayload.Click != "https://siem.example.com/alerts?id=42" {
		t.Errorf("payload.Click = %q", gotPayload.Click)
	}
	if gotPayload.Icon != "https://siem.example.com/icons/homesiem-192.png" {
		t.Errorf("payload.Icon = %q", gotPayload.Icon)
	}
	if !gotPayload.Markdown {
		t.Error("payload.Markdown = false, want true")
	}
	if len(gotPayload.Actions) != 1 || gotPayload.Actions[0].Action != "view" ||
		gotPayload.Actions[0].Label != "Open in homeSIEM" ||
		gotPayload.Actions[0].URL != "https://siem.example.com/alerts?id=42" {
		t.Errorf("payload.Actions = %+v", gotPayload.Actions)
	}
}

func TestPublish_OmitsEmptyOptionalFields(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := New(srv.URL, "homesiem", "", srv.Client())
	if err := c.Publish(context.Background(), Message{Title: "t", Body: "m"}); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}

	for _, field := range []string{`"tags"`, `"click"`, `"icon"`, `"actions"`, `"markdown"`, `"priority"`} {
		if strings.Contains(gotBody, field) {
			t.Errorf("body = %s, expected %s to be omitted when unset", gotBody, field)
		}
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
	if err := c.Publish(context.Background(), Message{Title: "t", Body: "m"}); err != nil {
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
	if err := c.Publish(context.Background(), Message{Title: "t", Body: "m"}); err == nil {
		t.Fatal("Publish() error = nil, want error for 500 response")
	}
}
