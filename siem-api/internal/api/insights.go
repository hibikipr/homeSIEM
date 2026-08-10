package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/hibikipr/homeSIEM/siem-api/internal/store"
)

type evidenceItem struct {
	Program       string `json:"program"`
	SampleMessage string `json:"sample_message"`
	Count         int    `json:"count"`
}

type insightResponse struct {
	ID        int64          `json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	Title     string         `json:"title"`
	Detail    string         `json:"detail"`
	Severity  string         `json:"severity"`
	Category  string         `json:"category"`
	Evidence  []evidenceItem `json:"evidence"`
	Dismissed bool           `json:"dismissed"`
}

func toInsightResponse(in store.Insight) insightResponse {
	// Best-effort: evidence_json is written by this same service (see
	// internal/insights.Service.GenerateNow), so a parse failure here would
	// mean a bug there, not bad input - degrade to an empty list rather
	// than fail the whole response for one insight's cosmetic detail.
	var evidence []evidenceItem
	_ = json.Unmarshal([]byte(in.EvidenceJSON), &evidence)
	return insightResponse{
		ID: in.ID, CreatedAt: in.CreatedAt, Title: in.Title, Detail: in.Detail,
		Severity: in.Severity, Category: in.Category, Evidence: evidence, Dismissed: in.Dismissed,
	}
}

func toInsightResponses(list []store.Insight) []insightResponse {
	out := make([]insightResponse, len(list))
	for i, in := range list {
		out[i] = toInsightResponse(in)
	}
	return out
}

func (s *Server) handleListInsights(w http.ResponseWriter, r *http.Request) {
	includeDismissed := r.URL.Query().Get("all") == "true"
	list, err := s.deps.Store.ListInsights(r.Context(), includeDismissed, 100)
	if err != nil {
		s.deps.Logger.Error("list insights failed", "error", err)
		http.Error(w, "list insights failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(toInsightResponses(list))
}

// handleGenerateInsights triggers a pass synchronously - a 24-27B model on
// the target hardware should respond well under a minute for this schema's
// response sizes, so blocking the request is the accepted v1 simplification
// (see the design doc's Known gaps).
func (s *Server) handleGenerateInsights(w http.ResponseWriter, r *http.Request) {
	if s.deps.Insights == nil {
		http.Error(w, "insights is not configured", http.StatusBadRequest)
		return
	}
	if err := s.deps.Insights.GenerateNow(r.Context()); err != nil {
		s.deps.Logger.Error("generate insights failed", "error", err)
		http.Error(w, "generate insights failed", http.StatusBadGateway)
		return
	}
	list, err := s.deps.Store.ListInsights(r.Context(), false, 100)
	if err != nil {
		s.deps.Logger.Error("list insights after generate failed", "error", err)
		http.Error(w, "list insights failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(toInsightResponses(list))
}

func (s *Server) handleDismissInsight(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid insight id", http.StatusBadRequest)
		return
	}
	if err := s.deps.Store.DismissInsight(r.Context(), id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "insight not found", http.StatusNotFound)
			return
		}
		s.deps.Logger.Error("dismiss insight failed", "error", err)
		http.Error(w, "dismiss insight failed", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
