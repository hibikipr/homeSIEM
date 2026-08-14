package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/hibikipr/homeSIEM/siem-api/internal/ollama"
	"github.com/hibikipr/homeSIEM/siem-api/internal/store"
)

type evidenceItem struct {
	Program       string `json:"program"`
	SampleMessage string `json:"sample_message"`
	Count         int    `json:"count"`
}

type generateInsightsResponse struct {
	Generated int               `json:"generated"`
	Insights  []insightResponse `json:"insights"`
}

type insightResponse struct {
	ID              int64          `json:"id"`
	CreatedAt       time.Time      `json:"created_at"`
	Title           string         `json:"title"`
	Detail          string         `json:"detail"`
	Severity        string         `json:"severity"`
	Category        string         `json:"category"`
	Evidence        []evidenceItem `json:"evidence"`
	Dismissed       bool           `json:"dismissed"`
	Fingerprint     string         `json:"fingerprint"`
	OccurrenceCount int            `json:"occurrence_count"`
	LastSeenAt      time.Time      `json:"last_seen_at"`
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
		Fingerprint: in.Fingerprint, OccurrenceCount: in.OccurrenceCount, LastSeenAt: in.LastSeenAt,
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
	generated, err := s.deps.Insights.GenerateNow(r.Context())
	if err != nil {
		s.deps.Logger.Error("generate insights failed", "error", err)
		msg := "generate insights failed"
		if errors.Is(err, ollama.ErrUnreachable) {
			msg = "generate insights failed: Ollama host not reachable - check OLLAMA_URL and that the host is powered on and reachable on the network"
		}
		http.Error(w, msg, http.StatusBadGateway)
		return
	}
	list, err := s.deps.Store.ListInsights(r.Context(), false, 100)
	if err != nil {
		s.deps.Logger.Error("list insights after generate failed", "error", err)
		http.Error(w, "list insights failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(generateInsightsResponse{Generated: generated, Insights: toInsightResponses(list)})
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

type mutedFingerprintResponse struct {
	Fingerprint string    `json:"fingerprint"`
	Category    string    `json:"category"`
	Programs    string    `json:"programs"`
	MutedAt     time.Time `json:"muted_at"`
}

// handleMuteInsight mutes the fingerprint of the given insight - unlike
// dismiss, which only clears this one row and lets a future recurrence
// reappear, a mute suppresses every future occurrence of the same
// fingerprint (see Service.GenerateNow) until explicitly unmuted. Also
// dismisses the current row, so muting has the same immediate visible
// effect as dismissing plus the standing suppression.
func (s *Server) handleMuteInsight(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid insight id", http.StatusBadRequest)
		return
	}
	in, err := s.deps.Store.GetInsight(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "insight not found", http.StatusNotFound)
			return
		}
		s.deps.Logger.Error("mute insight: get insight failed", "insight_id", id, "error", err)
		http.Error(w, "mute insight failed", http.StatusInternalServerError)
		return
	}

	var evidence []evidenceItem
	_ = json.Unmarshal([]byte(in.EvidenceJSON), &evidence)
	programs := make([]string, 0, len(evidence))
	for _, e := range evidence {
		programs = append(programs, e.Program)
	}

	if err := s.deps.Store.MuteFingerprint(r.Context(), in.Fingerprint, in.Category, strings.Join(programs, ",")); err != nil {
		s.deps.Logger.Error("mute insight: mute fingerprint failed", "insight_id", id, "error", err)
		http.Error(w, "mute insight failed", http.StatusInternalServerError)
		return
	}
	if err := s.deps.Store.DismissInsight(r.Context(), id); err != nil && !errors.Is(err, sql.ErrNoRows) {
		s.deps.Logger.Error("mute insight: dismiss failed", "insight_id", id, "error", err)
		http.Error(w, "mute insight failed", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListMutedInsights(w http.ResponseWriter, r *http.Request) {
	list, err := s.deps.Store.ListMutedFingerprints(r.Context())
	if err != nil {
		s.deps.Logger.Error("list muted fingerprints failed", "error", err)
		http.Error(w, "list muted fingerprints failed", http.StatusInternalServerError)
		return
	}
	out := make([]mutedFingerprintResponse, len(list))
	for i, m := range list {
		out[i] = mutedFingerprintResponse{Fingerprint: m.Fingerprint, Category: m.Category, Programs: m.Programs, MutedAt: m.MutedAt}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}

func (s *Server) handleUnmuteInsight(w http.ResponseWriter, r *http.Request) {
	fingerprint := r.PathValue("fingerprint")
	if err := s.deps.Store.UnmuteFingerprint(r.Context(), fingerprint); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "muted fingerprint not found", http.StatusNotFound)
			return
		}
		s.deps.Logger.Error("unmute fingerprint failed", "fingerprint", fingerprint, "error", err)
		http.Error(w, "unmute fingerprint failed", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
