package ntfy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// Action is a single ntfy action button, shown in addition to the
// notification's own tap-to-open (Message.Click). Only the "view" action
// type (open a URL) is supported — ntfy also has "http" and "broadcast"
// actions, but those would mean embedding a bearer token in a message body
// anyone with the topic can read, which this project deliberately avoids.
type Action struct {
	Label string
	URL   string
}

// Message is everything homeSIEM can express about a single ntfy
// notification. See https://docs.ntfy.sh/publish/ for what each field does.
type Message struct {
	Title    string
	Body     string
	Priority int      // 1 (min) .. 5 (urgent); 0 leaves ntfy's own default (3)
	Tags     []string // emoji short-codes, e.g. "rotating_light" renders as 🚨
	Click    string   // URL opened when the notification body itself is tapped
	Icon     string   // URL to a small icon shown next to the notification
	Actions  []Action // extra labeled buttons, alongside Click
	Markdown bool     // render Body as Markdown
}

type Client struct {
	baseURL    string
	topic      string
	token      string
	httpClient *http.Client
}

func New(baseURL, topic, token string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{baseURL: baseURL, topic: topic, token: token, httpClient: httpClient}
}

type jsonAction struct {
	Action string `json:"action"`
	Label  string `json:"label"`
	URL    string `json:"url"`
}

type jsonPayload struct {
	Topic    string       `json:"topic"`
	Title    string       `json:"title,omitempty"`
	Message  string       `json:"message,omitempty"`
	Priority int          `json:"priority,omitempty"`
	Tags     []string     `json:"tags,omitempty"`
	Click    string       `json:"click,omitempty"`
	Icon     string       `json:"icon,omitempty"`
	Actions  []jsonAction `json:"actions,omitempty"`
	Markdown bool         `json:"markdown,omitempty"`
}

// Publish sends msg to the configured topic using ntfy's JSON publish API
// (https://docs.ntfy.sh/publish/#publish-as-json), which is POSTed to the
// server root with "topic" in the body rather than the header-based form
// POSTed to /<topic> — the header form can't express "actions" at all.
func (c *Client) Publish(ctx context.Context, msg Message) error {
	payload := jsonPayload{
		Topic:    c.topic,
		Title:    msg.Title,
		Message:  msg.Body,
		Priority: msg.Priority,
		Tags:     msg.Tags,
		Click:    msg.Click,
		Icon:     msg.Icon,
		Markdown: msg.Markdown,
	}
	for _, a := range msg.Actions {
		payload.Actions = append(payload.Actions, jsonAction{Action: "view", Label: a.Label, URL: a.URL})
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("ntfy: encode payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(c.baseURL, "/")+"/", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("ntfy: publish request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("ntfy: publish failed: status=%d", resp.StatusCode)
	}
	return nil
}
