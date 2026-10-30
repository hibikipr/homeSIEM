package api

import (
	"crypto/subtle"
	"net/http"
	"strings"
	"time"
)

type haSummaryAlert struct {
	Title    string `json:"title"`
	Severity string `json:"severity"`
	At       string `json:"at"`
}

type haSummaryResponse struct {
	Alerts24h int             `json:"alerts_24h"`
	High24h   int             `json:"high_24h"`
	Last      *haSummaryAlert `json:"last"`
}

// handleHASummary serves a small read-only rollup of the last 24h of alerts
// for Home Assistant (a REST sensor polling it for a dashboard). HA can't
// hold a session JWT, so this one route takes its own static bearer token
// (SIEM_READONLY_TOKEN) instead of going through protect - and that token is
// checked nowhere else, so it can't be used against any other route.
// "high" is the critical severity tier.
func (s *Server) handleHASummary(w http.ResponseWriter, r *http.Request) {
	if !s.validReadonlyToken(r) {
		http.Error(w, "invalid read-only token", http.StatusUnauthorized)
		return
	}

	sum, err := s.deps.Store.SummarizeAlerts(r.Context(), time.Now().UTC().Add(-24*time.Hour))
	if err != nil {
		s.deps.Logger.Error("summarize alerts failed", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	resp := haSummaryResponse{Alerts24h: sum.Count, High24h: sum.Critical}
	if sum.Latest != nil {
		resp.Last = &haSummaryAlert{
			Title:    sum.Latest.Title,
			Severity: sum.Latest.Severity,
			At:       sum.Latest.LastSeenAt.UTC().Format(time.RFC3339),
		}
	}
	writeJSON(w, resp)
}

// validReadonlyToken checks "Authorization: Bearer <token>" against
// SIEM_READONLY_TOKEN in constant time. An unconfigured (empty) token
// rejects everything, so the route is off until it's set.
func (s *Server) validReadonlyToken(r *http.Request) bool {
	want := s.deps.ReadonlyToken
	got, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	return ok && want != "" && subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}
