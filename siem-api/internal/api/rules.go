package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/hibikipr/homeSIEM/siem-api/internal/auth"
	"github.com/hibikipr/homeSIEM/siem-api/internal/store"
)

type ruleResponse struct {
	ID           int64      `json:"id"`
	Name         string     `json:"name"`
	Shape        string     `json:"shape"`
	LogQL        string     `json:"logql"`
	WindowSec    int        `json:"window_sec"`
	Threshold    *int       `json:"threshold,omitempty"`
	GroupBy      []string   `json:"group_by"`
	Severity     string     `json:"severity"`
	Destinations []string   `json:"destinations"`
	CooldownSec  int        `json:"cooldown_sec"`
	IntervalSec  int        `json:"interval_sec"`
	Enabled      bool       `json:"enabled"`
	LastRunAt    *time.Time `json:"last_run_at,omitempty"`
}

// emptyIfNil keeps `group_by` and `destinations` JSON arrays rather than `null`
// for rules stored without them. Clients (siem-web's RuleResponse) type these as
// plain arrays and call array methods on them directly, so a `null` would be a
// runtime error there rather than an empty list.
func emptyIfNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func toRuleResponse(r store.Rule) ruleResponse {
	return ruleResponse{
		ID: r.ID, Name: r.Name, Shape: r.Shape, LogQL: r.LogQL, WindowSec: r.WindowSec,
		Threshold: r.Threshold, GroupBy: emptyIfNil(r.GroupBy), Severity: r.Severity,
		Destinations: emptyIfNil(r.Destinations), CooldownSec: r.CooldownSec, IntervalSec: r.IntervalSec,
		Enabled: r.Enabled, LastRunAt: r.LastRunAt,
	}
}

type ruleRequest struct {
	Name         string   `json:"name"`
	Shape        string   `json:"shape"`
	LogQL        string   `json:"logql"`
	WindowSec    int      `json:"window_sec"`
	Threshold    *int     `json:"threshold"`
	GroupBy      []string `json:"group_by"`
	Severity     string   `json:"severity"`
	Destinations []string `json:"destinations"`
	CooldownSec  int      `json:"cooldown_sec"`
	IntervalSec  int      `json:"interval_sec"`
	Enabled      bool     `json:"enabled"`
}

var validSeverities = map[string]bool{"info": true, "warning": true, "critical": true}

func (rq ruleRequest) toStoreRule() store.Rule {
	return store.Rule{
		Name: rq.Name, Shape: rq.Shape, LogQL: rq.LogQL, WindowSec: rq.WindowSec,
		Threshold: rq.Threshold, GroupBy: rq.GroupBy, Severity: rq.Severity,
		Destinations: rq.Destinations, CooldownSec: rq.CooldownSec, IntervalSec: rq.IntervalSec,
		Enabled: rq.Enabled,
	}
}

func (s *Server) handleListRules(w http.ResponseWriter, r *http.Request) {
	ruleList, err := s.deps.Store.ListRules(r.Context())
	if err != nil {
		http.Error(w, "list rules failed", http.StatusInternalServerError)
		return
	}
	resp := make([]ruleResponse, len(ruleList))
	for i, rl := range ruleList {
		resp[i] = toRuleResponse(rl)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleCreateRule(w http.ResponseWriter, r *http.Request) {
	var req ruleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}

	if !validSeverities[req.Severity] {
		http.Error(w, "severity must be one of: info, warning, critical", http.StatusBadRequest)
		return
	}

	userID, _, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthenticated", http.StatusUnauthorized)
		return
	}

	created, err := s.deps.Store.CreateRule(r.Context(), req.toStoreRule(), &userID)
	if err != nil {
		s.deps.Logger.Error("create rule failed", "error", err)
		http.Error(w, "create rule failed", http.StatusInternalServerError)
		return
	}

	if created.Enabled && s.deps.Scheduler != nil {
		s.deps.Scheduler.StartRule(s.deps.SchedulerCtx, created)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(toRuleResponse(created))
}

func (s *Server) handleUpdateRule(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid rule id", http.StatusBadRequest)
		return
	}

	var req ruleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}

	if !validSeverities[req.Severity] {
		http.Error(w, "severity must be one of: info, warning, critical", http.StatusBadRequest)
		return
	}

	ruleToUpdate := req.toStoreRule()
	ruleToUpdate.ID = id

	userID, _, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthenticated", http.StatusUnauthorized)
		return
	}

	updated, err := s.deps.Store.UpdateRule(r.Context(), ruleToUpdate, &userID)
	if err != nil {
		s.deps.Logger.Error("update rule failed", "rule_id", id, "error", err)
		http.Error(w, "update rule failed", http.StatusInternalServerError)
		return
	}

	if s.deps.Scheduler != nil {
		if updated.Enabled {
			s.deps.Scheduler.StartRule(s.deps.SchedulerCtx, updated)
		} else {
			s.deps.Scheduler.StopRule(updated.ID)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(toRuleResponse(updated))
}

func (s *Server) handleDeleteRule(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid rule id", http.StatusBadRequest)
		return
	}

	userID, _, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthenticated", http.StatusUnauthorized)
		return
	}

	if err := s.deps.Store.DeleteRule(r.Context(), id, &userID); err != nil {
		s.deps.Logger.Error("delete rule failed", "rule_id", id, "error", err)
		http.Error(w, "delete rule failed", http.StatusInternalServerError)
		return
	}

	if s.deps.Scheduler != nil {
		s.deps.Scheduler.StopRule(id)
	}
	w.WriteHeader(http.StatusNoContent)
}
