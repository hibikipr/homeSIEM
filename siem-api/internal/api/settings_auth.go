package api

import (
	"encoding/json"
	"net/http"

	"github.com/hibikipr/homeSIEM/siem-api/internal/store"
)

type roleMappingResp struct {
	ID         int64  `json:"id"`
	GroupClaim string `json:"group_claim"`
	Role       string `json:"role"`
	Priority   int    `json:"priority"`
}

type authSettingsResponse struct {
	OIDCIssuer      string            `json:"oidc_issuer"`
	OIDCClientID    string            `json:"oidc_client_id"`
	OIDCGroupsScope string            `json:"oidc_groups_scope"`
	RoleMappings    []roleMappingResp `json:"role_mappings"`
}

func (s *Server) handleGetAuthSettings(w http.ResponseWriter, r *http.Request) {
	mappings, err := s.deps.Store.ListRoleMappings(r.Context())
	if err != nil {
		http.Error(w, "list role mappings failed", http.StatusInternalServerError)
		return
	}

	resp := authSettingsResponse{
		OIDCIssuer: s.deps.OIDCIssuer, OIDCClientID: s.deps.OIDCClientID, OIDCGroupsScope: s.deps.OIDCGroupsScope,
	}
	for _, m := range mappings {
		resp.RoleMappings = append(resp.RoleMappings, roleMappingResp{ID: m.ID, GroupClaim: m.GroupClaim, Role: m.Role, Priority: m.Priority})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

type updateAuthSettingsRequest struct {
	RoleMappings []roleMappingResp `json:"role_mappings"`
}

func (s *Server) handleUpdateAuthSettings(w http.ResponseWriter, r *http.Request) {
	var req updateAuthSettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}

	for _, m := range req.RoleMappings {
		if _, err := s.deps.Store.UpsertRoleMapping(r.Context(), store.RoleMapping{
			GroupClaim: m.GroupClaim, Role: m.Role, Priority: m.Priority,
		}); err != nil {
			s.deps.Logger.Error("upsert role mapping failed", "group", m.GroupClaim, "error", err)
			http.Error(w, "update role mappings failed", http.StatusInternalServerError)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}
