package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/hibikipr/homeSIEM/siem-api/internal/ntfy"
)

type notificationSettingsResponse struct {
	NtfyConfigured bool   `json:"ntfy_configured"`
	MinSeverity    string `json:"min_severity"`
}

func (s *Server) handleGetNotificationSettings(w http.ResponseWriter, r *http.Request) {
	minSeverity, err := s.deps.Store.GetMinNotifySeverity(r.Context())
	if err != nil {
		s.deps.Logger.Error("get min notify severity failed", "error", err)
		http.Error(w, "get notification settings failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(notificationSettingsResponse{
		NtfyConfigured: s.deps.NtfyURL != "" && s.deps.NtfyTopic != "",
		MinSeverity:    minSeverity,
	})
}

var validMinSeverities = map[string]bool{"info": true, "warning": true, "critical": true}

type updateNotificationSettingsRequest struct {
	MinSeverity string `json:"min_severity"`
}

func (s *Server) handleUpdateNotificationSettings(w http.ResponseWriter, r *http.Request) {
	var req updateNotificationSettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}
	if !validMinSeverities[req.MinSeverity] {
		http.Error(w, "min_severity must be one of info, warning, critical", http.StatusBadRequest)
		return
	}

	if err := s.deps.Store.SetMinNotifySeverity(r.Context(), req.MinSeverity); err != nil {
		s.deps.Logger.Error("set min notify severity failed", "error", err)
		http.Error(w, "update notification settings failed", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleTestNotification(w http.ResponseWriter, r *http.Request) {
	if s.deps.NtfyURL == "" || s.deps.NtfyTopic == "" || s.deps.Ntfy == nil {
		http.Error(w, "ntfy is not configured", http.StatusBadRequest)
		return
	}

	msg := ntfy.Message{
		Title: "homeSIEM test notification",
		Body: "Sent from Settings at " + time.Now().UTC().Format(time.RFC3339) +
			" to confirm ntfy is reachable.",
		Priority: 3, // default
		Tags:     []string{"test_tube"},
		Markdown: true,
	}

	if err := s.deps.Ntfy.Publish(r.Context(), msg); err != nil {
		s.deps.Logger.Error("test notification publish failed", "error", err)
		http.Error(w, "test notification failed", http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}
