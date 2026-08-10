package insights

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/hibikipr/homeSIEM/siem-api/internal/store"
)

type Chatter interface {
	Chat(ctx context.Context, systemPrompt, userPrompt string) (string, error)
}

type InsightStore interface {
	InsertInsight(ctx context.Context, in store.Insight) (store.Insight, error)
}

type Service struct {
	Prompt   *PromptBuilder
	Chat     Chatter
	Store    InsightStore
	Lookback time.Duration
	Logger   *slog.Logger
}

func NewService(prompt *PromptBuilder, chat Chatter, st InsightStore, lookback time.Duration, logger *slog.Logger) *Service {
	return &Service{Prompt: prompt, Chat: chat, Store: st, Lookback: lookback, Logger: logger}
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
// ran but didn't parse.
func (s *Service) GenerateNow(ctx context.Context) error {
	systemPrompt, userPrompt, err := s.Prompt.Build(ctx, s.Lookback)
	if err != nil {
		return fmt.Errorf("insights: build prompt: %w", err)
	}

	response, err := s.Chat.Chat(ctx, systemPrompt, userPrompt)
	if err != nil {
		return fmt.Errorf("insights: chat: %w", err)
	}

	parsed, err := parseModelResponse(response)
	if err != nil {
		s.Logger.Warn("insights: malformed model response, skipping this pass",
			"error", err, "response", response)
		return nil
	}

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
		}
	}
	return nil
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
