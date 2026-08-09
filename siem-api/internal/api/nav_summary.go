package api

import (
	"fmt"
	"net/http"
	"time"
)

type navSummaryResponse struct {
	EventsPerMin   int64 `json:"events_per_min"`
	OpenAlertCount int   `json:"open_alert_count"`
}

// handleNavSummary backs the global nav chrome's "ingest live X/min" text
// and alert-count badge - both previously hardcoded to 0 in siem-web with
// no data source at all. Deliberately cheap (one Loki instant query, one
// indexed SQL count) since this runs on every page navigation, unlike
// /events/stats' full heat-grid/hourly-chart computation (25 Loki queries
// per severity tier). Uses QueryInstant rather than QueryMatrix for the
// same reason stats.go does - see QueryInstant's doc comment.
func (s *Server) handleNavSummary(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	logql := fmt.Sprintf(`sum(count_over_time({job=%q}[1m]))`, s.deps.JobLabel)
	result, err := s.deps.Loki.QueryInstant(ctx, logql, time.Now().UTC())
	if err != nil {
		s.deps.Logger.Error("nav summary: loki query failed", "error", err)
		http.Error(w, "query failed", http.StatusBadGateway)
		return
	}
	var eventsPerMin int64
	if len(result.Series) > 0 && len(result.Series[0].Samples) > 0 {
		eventsPerMin = int64(result.Series[0].Samples[0].Value)
	}

	openAlerts, err := s.deps.Store.ListAlerts(ctx, "open")
	if err != nil {
		s.deps.Logger.Error("nav summary: list alerts failed", "error", err)
		http.Error(w, "query failed", http.StatusInternalServerError)
		return
	}

	writeJSON(w, navSummaryResponse{
		EventsPerMin:   eventsPerMin,
		OpenAlertCount: len(openAlerts),
	})
}
