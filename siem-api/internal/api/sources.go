package api

import (
	"context"
	"encoding/json"
	"fmt"
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
	Status       string     `json:"status"`
	EventsPerMin float64    `json:"events_per_min"`
}

func toSourceResponse(src store.Source, now time.Time, eventsPerMin float64) sourceResponse {
	return sourceResponse{
		ID: src.ID, Name: src.Name, Address: src.Address, Transport: src.Transport,
		Parser: src.Parser, Claimed: src.Claimed, HeartbeatSec: src.HeartbeatSec, LastSeenAt: src.LastSeenAt,
		Status:       sourceStatus(src, now),
		EventsPerMin: eventsPerMin,
	}
}

// sourceStatus reimplements the same threshold store.StaleSources uses
// internally, as a plain comparison against fields handleListSources
// already fetched — not a second SQL round trip.
func sourceStatus(src store.Source, now time.Time) string {
	if src.LastSeenAt == nil {
		return "silent"
	}
	if now.Sub(*src.LastSeenAt) > time.Duration(src.HeartbeatSec)*time.Second {
		return "silent"
	}
	return "healthy"
}

func (s *Server) handleListSources(w http.ResponseWriter, r *http.Request) {
	sources, err := s.deps.Store.ListSources(r.Context())
	if err != nil {
		http.Error(w, "list sources failed", http.StatusInternalServerError)
		return
	}

	eventsPerMin := map[string]float64{}
	if s.deps.Loki != nil {
		rates, err := s.queryEventsPerMinBySource(r.Context())
		if err != nil {
			s.deps.Logger.Error("list sources: events-per-min query failed", "error", err)
		} else {
			eventsPerMin = rates
		}
	}

	now := time.Now().UTC()
	resp := make([]sourceResponse, len(sources))
	for i, src := range sources {
		resp[i] = toSourceResponse(src, now, eventsPerMin[src.Name])
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

// queryEventsPerMinBySource returns a 5-minute rolling events/min rate per
// source, the same query shape as stats.go's queryHourlyBySource but over a
// short window with no severity filter.
func (s *Server) queryEventsPerMinBySource(ctx context.Context) (map[string]float64, error) {
	end := time.Now().UTC()
	start := end.Add(-5 * time.Minute)
	logql := fmt.Sprintf(`sum by (source) (count_over_time({job=%q}[5m]))`, s.deps.JobLabel)

	result, err := s.deps.Loki.QueryMatrix(ctx, logql, start, end, 5*time.Minute)
	if err != nil {
		return nil, err
	}

	out := map[string]float64{}
	for _, series := range result.Series {
		if len(series.Samples) == 0 {
			continue
		}
		latest := series.Samples[len(series.Samples)-1].Value
		out[series.Labels["source"]] = latest / 5
	}
	return out, nil
}
