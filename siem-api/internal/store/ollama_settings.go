package store

import (
	"context"
	"fmt"
)

// OllamaSettings holds the admin-editable pieces of siem-insights' Ollama
// call - everything topology-related (URL, model, timeout, schedule) stays
// env-only, same as ntfy's URL/topic; this is only the generation-shaping
// knobs a human might reasonably want to tune without a redeploy.
//
// SystemPrompt == "" means "use insights.DefaultSystemPrompt" - same
// empty-means-default convention as AppURL/OllamaURL elsewhere in this
// codebase, and what lets a PUT with an empty string act as "reset to
// default" with no separate endpoint.
type OllamaSettings struct {
	SystemPrompt string
	Temperature  float64
	TopP         float64
	NumPredict   int
	NumCtx       int
}

func (s *Store) GetOllamaSettings(ctx context.Context) (OllamaSettings, error) {
	var out OllamaSettings
	err := s.db.QueryRowContext(ctx, `
		SELECT system_prompt, temperature, top_p, num_predict, num_ctx
		FROM ollama_settings WHERE id = 1
	`).Scan(&out.SystemPrompt, &out.Temperature, &out.TopP, &out.NumPredict, &out.NumCtx)
	if err != nil {
		return OllamaSettings{}, fmt.Errorf("store: get ollama settings: %w", err)
	}
	return out, nil
}

func (s *Store) SetOllamaSettings(ctx context.Context, in OllamaSettings) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO ollama_settings (id, system_prompt, temperature, top_p, num_predict, num_ctx)
		VALUES (1, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			system_prompt = excluded.system_prompt,
			temperature = excluded.temperature,
			top_p = excluded.top_p,
			num_predict = excluded.num_predict,
			num_ctx = excluded.num_ctx
	`, in.SystemPrompt, in.Temperature, in.TopP, in.NumPredict, in.NumCtx)
	if err != nil {
		return fmt.Errorf("store: set ollama settings: %w", err)
	}
	return nil
}
