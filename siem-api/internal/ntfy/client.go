package ntfy

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

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

func (c *Client) Publish(ctx context.Context, title, message, priority string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(c.baseURL, "/")+"/"+c.topic, strings.NewReader(message))
	if err != nil {
		return err
	}
	req.Header.Set("Title", title)
	req.Header.Set("Priority", priority)
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
