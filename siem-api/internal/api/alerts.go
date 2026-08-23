package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/hibikipr/homeSIEM/siem-api/internal/auth"
	"github.com/hibikipr/homeSIEM/siem-api/internal/store"
)

type alertResponse struct {
	ID          int64      `json:"id"`
	RuleID      int64      `json:"rule_id"`
	GroupKey    string     `json:"group_key"`
	Severity    string     `json:"severity"`
	Title       string     `json:"title"`
	Body        string     `json:"body"`
	EventCount  int        `json:"event_count"`
	State       string     `json:"state"`
	FirstSeenAt time.Time  `json:"first_seen_at"`
	LastSeenAt  time.Time  `json:"last_seen_at"`
	AckedBy     *int64     `json:"acked_by,omitempty"`
	AckedAt     *time.Time `json:"acked_at,omitempty"`
	// Context is the rule shape's own structured payload (e.g. absence's
	// per-source last-seen/heartbeat data - see rules.sourceContext) -
	// passed through as raw JSON rather than unmarshaled into a Go type
	// here, since its shape is owned by whichever evaluator produced it,
	// not by this DTO. Omitted for rows stored before Context existed, or
	// whose evaluator never set one (an empty "{}" marshals to "{}", not
	// omitted, but null/"" - only possible for hand-inserted rows - is
	// omitted rather than sent as literal JSON null).
	Context json.RawMessage `json:"context,omitempty"`
}

func toAlertResponse(a store.Alert) alertResponse {
	resp := alertResponse{
		ID: a.ID, RuleID: a.RuleID, GroupKey: a.GroupKey, Severity: a.Severity,
		Title: a.Title, Body: a.Body, EventCount: a.EventCount, State: a.State,
		FirstSeenAt: a.FirstSeenAt, LastSeenAt: a.LastSeenAt, AckedBy: a.AckedBy, AckedAt: a.AckedAt,
	}
	if a.Context != "" {
		resp.Context = json.RawMessage(a.Context)
	}
	return resp
}

func (s *Server) handleListAlerts(w http.ResponseWriter, r *http.Request) {
	alertsList, err := s.deps.Store.ListAlerts(r.Context(), r.URL.Query().Get("state"))
	if err != nil {
		s.deps.Logger.Error("list alerts failed", "error", err)
		http.Error(w, "list alerts failed", http.StatusInternalServerError)
		return
	}

	resp := make([]alertResponse, len(alertsList))
	for i, a := range alertsList {
		resp[i] = toAlertResponse(a)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleAckAlert(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid alert id", http.StatusBadRequest)
		return
	}

	userID, _, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthenticated", http.StatusUnauthorized)
		return
	}

	if err := s.deps.Store.AckAlert(r.Context(), id, userID, time.Now().UTC()); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "alert not found", http.StatusNotFound)
			return
		}
		s.deps.Logger.Error("ack alert failed", "alert_id", id, "error", err)
		http.Error(w, "ack failed", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleMuteAlert(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid alert id", http.StatusBadRequest)
		return
	}
	userID, _, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthenticated", http.StatusUnauthorized)
		return
	}

	until := time.Now().UTC().Add(time.Hour)
	if err := s.deps.Store.MuteAlert(r.Context(), id, userID, until); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "alert not found", http.StatusNotFound)
			return
		}
		s.deps.Logger.Error("mute alert failed", "alert_id", id, "error", err)
		http.Error(w, "mute failed", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type alertSampleResponse struct {
	ID   int64     `json:"id"`
	TS   time.Time `json:"ts"`
	Line string    `json:"line"`
}

func (s *Server) handleListAlertSamples(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid alert id", http.StatusBadRequest)
		return
	}

	if _, err := s.deps.Store.GetAlert(r.Context(), id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "alert not found", http.StatusNotFound)
			return
		}
		s.deps.Logger.Error("get alert failed", "alert_id", id, "error", err)
		http.Error(w, "get alert failed", http.StatusInternalServerError)
		return
	}

	samples, err := s.deps.Store.ListAlertSamples(r.Context(), id)
	if err != nil {
		s.deps.Logger.Error("list alert samples failed", "alert_id", id, "error", err)
		http.Error(w, "list samples failed", http.StatusInternalServerError)
		return
	}

	resp := make([]alertSampleResponse, len(samples))
	for i, sample := range samples {
		resp[i] = alertSampleResponse{ID: sample.ID, TS: sample.TS, Line: sample.Line}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleAlertsStream(w http.ResponseWriter, r *http.Request) {
	s.deps.Hub.ServeHTTP("alerts", w, r)
}
