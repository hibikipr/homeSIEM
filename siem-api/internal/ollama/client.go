// Package ollama is a minimal client for a locally-hosted Ollama instance's
// chat API (https://github.com/ollama/ollama/blob/main/docs/api.md#generate-a-chat-completion).
// Non-streaming only - siem-insights wants one complete response to parse
// as JSON, not a token stream.
package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type Client struct {
	baseURL    string
	model      string
	httpClient *http.Client
}

func New(baseURL, model string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{baseURL: baseURL, model: model, httpClient: httpClient}
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatOptions maps to a subset of Ollama's generation options
// (https://github.com/ollama/ollama/blob/main/docs/api.md#generate-a-chat-completion),
// the ones siem-insights exposes as user-tunable in Settings. All four are
// always sent (no omitempty) since the caller always has real values from
// store.OllamaSettings, including a legitimate Temperature of 0.
type ChatOptions struct {
	Temperature float64 `json:"temperature"`
	TopP        float64 `json:"top_p"`
	// NumPredict caps the response length (Ollama's max-tokens-to-generate
	// knob) - bounds worst-case generation time independent of the HTTP
	// client timeout.
	NumPredict int `json:"num_predict"`
	// NumCtx overrides the model's context window. Worth setting explicitly:
	// Ollama's runtime default (often 2048-4096 depending on the model) can
	// silently truncate a prompt carrying open alerts + a severity rollup +
	// up to sampleCap deduplicated log samples.
	NumCtx int `json:"num_ctx"`
}

type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
	Options  ChatOptions   `json:"options"`
}

type chatResponse struct {
	Message chatMessage `json:"message"`
	Done    bool        `json:"done"`
}

// Chat sends a single system+user message pair and returns the assistant's
// full response content. The caller's http.Client is responsible for
// setting a timeout long enough for a full response from a ~24-30B model,
// which can genuinely take tens of seconds - this package doesn't guess one.
func (c *Client) Chat(ctx context.Context, systemPrompt, userPrompt string, opts ChatOptions) (string, error) {
	payload := chatRequest{
		Model: c.model,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Stream:  false,
		Options: opts,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("ollama: encode request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(c.baseURL, "/")+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("ollama: chat request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("ollama: chat failed: status=%d", resp.StatusCode)
	}

	var out chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("ollama: decode response: %w", err)
	}
	return out.Message.Content, nil
}
