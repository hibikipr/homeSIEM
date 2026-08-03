package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/hibikipr/homeSIEM/siem-api/internal/store"
)

type sourceResponse struct {
	ID           int64      `json:"id"`
	Name         string     `json:"name"`
	Address      string     `json:"address"`
	Transport    string     `json:"transport"`
	Parser       string     `json:"parser"`
	Claimed      bool       `json:"claimed"`
	HeartbeatSec int        `json:"heartbeat_sec"`
	LastSeenAt   *time.Time `json:"last_seen_at,omitempty"`
}

func toSourceResponse(src store.Source) sourceResponse {
	return sourceResponse{
		ID: src.ID, Name: src.Name, Address: src.Address, Transport: src.Transport,
		Parser: src.Parser, Claimed: src.Claimed, HeartbeatSec: src.HeartbeatSec, LastSeenAt: src.LastSeenAt,
	}
}

func (s *Server) handleListSources(w http.ResponseWriter, r *http.Request) {
	sources, err := s.deps.Store.ListSources(r.Context())
	if err != nil {
		http.Error(w, "list sources failed", http.StatusInternalServerError)
		return
	}
	resp := make([]sourceResponse, len(sources))
	for i, src := range sources {
		resp[i] = toSourceResponse(src)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleClaimSource(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid source id", http.StatusBadRequest)
		return
	}
	if err := s.deps.Store.ClaimSource(r.Context(), id); err != nil {
		s.deps.Logger.Error("claim source failed", "source_id", id, "error", err)
		http.Error(w, "claim source failed", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type sourceHeartbeatRequest struct {
	Name      string `json:"name"`
	Address   string `json:"address"`
	Transport string `json:"transport"`
	Parser    string `json:"parser"`
}

func (s *Server) handleSourceHeartbeat(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("X-Fastpath-Token") != s.deps.FastpathToken || s.deps.FastpathToken == "" {
		http.Error(w, "invalid fastpath token", http.StatusUnauthorized)
		return
	}

	var req sourceHeartbeatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	// heartbeat_sec: no UI exists yet to let an admin customize this per
	// source, so every heartbeat call passes the schema's own default.
	// UpsertSource always overwrites it — harmless today since nothing sets
	// it to anything else, but whoever builds the Sources screen's "edit
	// heartbeat interval" feature will need to read-then-preserve here
	// instead of always passing this constant.
	const defaultHeartbeatSec = 900

	if _, err := s.deps.Store.UpsertSource(ctx, store.Source{
		Name: req.Name, Address: req.Address, Transport: req.Transport,
		Parser: req.Parser, HeartbeatSec: defaultHeartbeatSec,
	}); err != nil {
		s.deps.Logger.Error("source heartbeat: upsert failed", "name", req.Name, "error", err)
		http.Error(w, "heartbeat failed", http.StatusInternalServerError)
		return
	}

	if err := s.deps.Store.TouchSourceLastSeen(ctx, req.Name, time.Now().UTC()); err != nil {
		s.deps.Logger.Error("source heartbeat: touch last_seen failed", "name", req.Name, "error", err)
		http.Error(w, "heartbeat failed", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusAccepted)
}
