package api

import (
	"encoding/json"
	"net/http"
)

type sessionResponse struct {
	UserID      int64  `json:"user_id"`
	Role        string `json:"role"`
	DisplayName string `json:"display_name"`
}

type sessionRequest struct {
	Subject     string   `json:"subject"`
	Email       string   `json:"email"`
	DisplayName string   `json:"display_name"`
	Groups      []string `json:"groups"`
}

func (s *Server) handleAuthSession(w http.ResponseWriter, r *http.Request) {
	var req sessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}

	user, err := s.deps.SessionEst.Establish(r.Context(), req.Subject, req.Email, req.DisplayName, req.Groups)
	if err != nil {
		s.deps.Logger.Error("auth session establish failed", "subject", req.Subject, "error", err)
		http.Error(w, "denied", http.StatusForbidden)
		return
	}

	displayName := ""
	if user.DisplayName != nil {
		displayName = *user.DisplayName
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sessionResponse{UserID: user.ID, Role: user.Role, DisplayName: displayName})
}

type localLoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (s *Server) handleAuthLocal(w http.ResponseWriter, r *http.Request) {
	var req localLoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}

	user, err := s.deps.LocalAuth.Login(r.Context(), req.Username, req.Password)
	if err != nil {
		s.deps.Logger.Error("local login failed", "username", req.Username, "error", err)
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}

	displayName := ""
	if user.DisplayName != nil {
		displayName = *user.DisplayName
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sessionResponse{UserID: user.ID, Role: user.Role, DisplayName: displayName})
}
