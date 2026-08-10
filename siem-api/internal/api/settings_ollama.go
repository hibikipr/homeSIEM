package api

import (
	"encoding/json"
	"net/http"

	"github.com/hibikipr/homeSIEM/siem-api/internal/insights"
	"github.com/hibikipr/homeSIEM/siem-api/internal/store"
)

// ollamaSettingsResponse mixes deployment-level info (set at deploy time via
// OLLAMA_URL/OLLAMA_MODEL/etc, read-only here - same as ntfy's URL/topic in
// notificationSettingsResponse) with the actual admin-editable fields below
// it. Model/timeout/schedule stay env-only deliberately: they're topology,
// not generation-shaping, and changing them without a redeploy would leave
// the running scheduler/HTTP client out of sync with what this pane shows.
type ollamaSettingsResponse struct {
	Configured  bool   `json:"configured"`
	Model       string `json:"model"`
	TimeoutSec  int    `json:"timeout_sec"`
	IntervalSec int    `json:"interval_sec"`
	LookbackMin int    `json:"lookback_min"`

	SystemPrompt        string  `json:"system_prompt"`
	DefaultSystemPrompt string  `json:"default_system_prompt"`
	Temperature         float64 `json:"temperature"`
	TopP                float64 `json:"top_p"`
	NumPredict          int     `json:"num_predict"`
	NumCtx              int     `json:"num_ctx"`
}

func (s *Server) handleGetOllamaSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := s.deps.Store.GetOllamaSettings(r.Context())
	if err != nil {
		s.deps.Logger.Error("get ollama settings failed", "error", err)
		http.Error(w, "get ollama settings failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ollamaSettingsResponse{
		Configured:  s.deps.OllamaURL != "",
		Model:       s.deps.OllamaModel,
		TimeoutSec:  s.deps.OllamaTimeoutSec,
		IntervalSec: s.deps.InsightsIntervalSec,
		LookbackMin: s.deps.InsightsLookbackMin,

		SystemPrompt:        settings.SystemPrompt,
		DefaultSystemPrompt: insights.DefaultSystemPrompt,
		Temperature:         settings.Temperature,
		TopP:                settings.TopP,
		NumPredict:          settings.NumPredict,
		NumCtx:              settings.NumCtx,
	})
}

type updateOllamaSettingsRequest struct {
	SystemPrompt string  `json:"system_prompt"`
	Temperature  float64 `json:"temperature"`
	TopP         float64 `json:"top_p"`
	NumPredict   int     `json:"num_predict"`
	NumCtx       int     `json:"num_ctx"`
}

// Bounds are generous rather than model-specific (this server has no way to
// know what any given Ollama instance's actual model supports) - they exist
// to catch fat-fingered input (e.g. a temperature of 20), not to encode
// opinions about what's "reasonable" for any one model.
func (s *Server) handleUpdateOllamaSettings(w http.ResponseWriter, r *http.Request) {
	var req updateOllamaSettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}
	if req.Temperature < 0 || req.Temperature > 2 {
		http.Error(w, "temperature must be between 0 and 2", http.StatusBadRequest)
		return
	}
	if req.TopP <= 0 || req.TopP > 1 {
		http.Error(w, "top_p must be greater than 0 and at most 1", http.StatusBadRequest)
		return
	}
	if req.NumPredict < 1 || req.NumPredict > 8192 {
		http.Error(w, "num_predict must be between 1 and 8192", http.StatusBadRequest)
		return
	}
	if req.NumCtx < 256 || req.NumCtx > 262144 {
		http.Error(w, "num_ctx must be between 256 and 262144", http.StatusBadRequest)
		return
	}

	if err := s.deps.Store.SetOllamaSettings(r.Context(), store.OllamaSettings{
		SystemPrompt: req.SystemPrompt,
		Temperature:  req.Temperature,
		TopP:         req.TopP,
		NumPredict:   req.NumPredict,
		NumCtx:       req.NumCtx,
	}); err != nil {
		s.deps.Logger.Error("set ollama settings failed", "error", err)
		http.Error(w, "update ollama settings failed", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
