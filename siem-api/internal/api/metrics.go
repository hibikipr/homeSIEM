package api

import (
	"net/http"
	"time"

	"github.com/hibikipr/homeSIEM/siem-api/internal/metrics"
)

// handleMetrics exposes the same per-source data the Wall's sources list
// shows (see handleListSources) as Prometheus text-exposition format, so a
// home Prometheus can scrape and alert on it independently of the Wall
// itself. Unauthenticated like /healthz - scrapers don't carry a session,
// and homeSIEM's own users are already the only people who can reach this
// server.
func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sources, err := s.deps.Store.ListSources(ctx)
	if err != nil {
		http.Error(w, "list sources failed", http.StatusInternalServerError)
		return
	}

	eventsPerMin := map[string]float64{}
	if s.deps.Loki != nil {
		rates, err := s.queryEventsPerMinBySource(ctx)
		if err != nil {
			s.deps.Logger.Error("metrics: events-per-min query failed", "error", err)
		} else {
			eventsPerMin = rates
		}
	}

	now := time.Now().UTC()
	out := make([]metrics.Source, len(sources))
	for i, src := range sources {
		out[i] = metrics.Source{
			Name:         src.Name,
			Claimed:      src.Claimed,
			Up:           sourceStatus(src, now) == "healthy",
			HeartbeatSec: src.HeartbeatSec,
			LastSeenAt:   src.LastSeenAt,
			EventsPerMin: eventsPerMin[src.Name],
		}
	}

	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.Write([]byte(metrics.RenderSources(out)))
}
