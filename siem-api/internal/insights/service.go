package insights

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/hibikipr/homeSIEM/siem-api/internal/ollama"
	"github.com/hibikipr/homeSIEM/siem-api/internal/store"
)

type Chatter interface {
	Chat(ctx context.Context, systemPrompt, userPrompt string, opts ollama.ChatOptions) (string, error)
}

type InsightStore interface {
	InsertInsight(ctx context.Context, in store.Insight) (store.Insight, error)
}

// SettingsStore is the admin-editable half of a GenerateNow pass (system
// prompt override, generation options) - kept separate from InsightStore
// since it's read every pass rather than written, and comes from
// Settings → Ollama in siem-web rather than anything a pass itself produces.
type SettingsStore interface {
	GetOllamaSettings(ctx context.Context) (store.OllamaSettings, error)
}

type Service struct {
	Prompt   *PromptBuilder
	Chat     Chatter
	Store    InsightStore
	Settings SettingsStore
	Lookback time.Duration
	Logger   *slog.Logger
}

func NewService(prompt *PromptBuilder, chat Chatter, st InsightStore, settings SettingsStore, lookback time.Duration, logger *slog.Logger) *Service {
	return &Service{Prompt: prompt, Chat: chat, Store: st, Settings: settings, Lookback: lookback, Logger: logger}
}

type modelEvidence struct {
	Program       string `json:"program"`
	SampleMessage string `json:"sample_message"`
	Count         int    `json:"count"`
}

type modelInsight struct {
	Title    string          `json:"title"`
	Detail   string          `json:"detail"`
	Severity string          `json:"severity"`
	Category string          `json:"category"`
	Evidence []modelEvidence `json:"evidence"`
}

// validSeverities matches the real severity vocabulary used everywhere else
// in this codebase (see RuleFromEventForm.svelte's <select>) - anything
// else is coerced to "info" (the "unrecognized falls to the lowest tier"
// convention already established for alert severity).
var validSeverities = map[string]bool{"info": true, "warning": true, "critical": true}

// GenerateNow runs one full pass: build the prompt from current data, ask
// the model, parse its response, and store whatever insights come back. A
// malformed/unparseable model response is logged and produces zero
// insights for this pass - never a crash, never a partial insert. Errors
// from Chat or from prompt-building ARE propagated (the caller - the
// scheduler - decides how to log/retry a failed pass), since those
// indicate the pass genuinely couldn't run at all, unlike a response that
// ran but didn't parse. The returned int is how many insights this pass
// actually inserted, so a caller (e.g. a manually-triggered pass) can tell
// "ran fine, found nothing" apart from "ran fine, found three things".
func (s *Service) GenerateNow(ctx context.Context) (int, error) {
	settings, err := s.Settings.GetOllamaSettings(ctx)
	if err != nil {
		return 0, fmt.Errorf("insights: get ollama settings: %w", err)
	}
	systemPrompt := settings.SystemPrompt
	if systemPrompt == "" {
		systemPrompt = DefaultSystemPrompt
	}

	userPrompt, err := s.Prompt.Build(ctx, s.Lookback)
	if err != nil {
		return 0, fmt.Errorf("insights: build prompt: %w", err)
	}

	opts := ollama.ChatOptions{
		Temperature: settings.Temperature,
		TopP:        settings.TopP,
		NumPredict:  settings.NumPredict,
		NumCtx:      settings.NumCtx,
	}
	response, err := s.Chat.Chat(ctx, systemPrompt, userPrompt, opts)
	if err != nil {
		return 0, fmt.Errorf("insights: chat: %w", err)
	}

	parsed, err := parseModelResponse(response)
	if err != nil {
		s.Logger.Warn("insights: malformed model response, skipping this pass",
			"error", err, "response", response)
		return 0, nil
	}

	inserted := 0
	for _, mi := range parsed {
		severity := mi.Severity
		if !validSeverities[severity] {
			severity = "info"
		}
		evidenceJSON, err := json.Marshal(mi.Evidence)
		if err != nil {
			s.Logger.Warn("insights: failed to encode evidence, skipping this insight", "error", err, "title", mi.Title)
			continue
		}
		if _, err := s.Store.InsertInsight(ctx, store.Insight{
			Title:        mi.Title,
			Detail:       mi.Detail,
			Severity:     severity,
			Category:     mi.Category,
			EvidenceJSON: string(evidenceJSON),
		}); err != nil {
			s.Logger.Warn("insights: failed to insert insight", "error", err, "title", mi.Title)
			continue
		}
		inserted++
	}
	return inserted, nil
}

// parseModelResponse extracts the first "[...]" substring from the model's
// response before decoding - despite the system prompt's "ONLY a JSON
// array, no prose, no markdown fence" instruction, real models sometimes
// wrap output in exactly that anyway.
func parseModelResponse(response string) ([]modelInsight, error) {
	start := strings.Index(response, "[")
	end := strings.LastIndex(response, "]")
	if start == -1 || end == -1 || end < start {
		return nil, fmt.Errorf("insights: no JSON array found in model response")
	}
	var out []modelInsight
	if err := json.Unmarshal([]byte(response[start:end+1]), &out); err != nil {
		return nil, fmt.Errorf("insights: decode model response: %w", err)
	}
	return out, nil
}
