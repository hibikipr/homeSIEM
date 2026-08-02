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
