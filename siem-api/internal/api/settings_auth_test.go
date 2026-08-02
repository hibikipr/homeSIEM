package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hibikipr/homeSIEM/siem-api/internal/store"
)

func TestGetAuthSettings_RequiresAdmin(t *testing.T) {
	s, st := newTestServer(t)
	token := authToken(t, st, "analyst", 50)

	req := httptest.NewRequest(http.MethodGet, "/settings/auth", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestGetAuthSettings_ReturnsConfigAndMappings(t *testing.T) {
	s, st := newTestServer(t)
	s.deps.OIDCIssuer = "https://pocketid.townsville.cc"
	s.deps.OIDCClientID = "homeSIEM"
	s.deps.OIDCGroupsScope = "groups"
	ctx := context.Background()
	if _, err := st.UpsertRoleMapping(ctx, store.RoleMapping{GroupClaim: "admins", Role: "admin", Priority: 10}); err != nil {
		t.Fatalf("UpsertRoleMapping() error = %v", err)
	}

	token := authToken(t, st, "admin", 5)
	req := httptest.NewRequest(http.MethodGet, "/settings/auth", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	var resp authSettingsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if resp.OIDCIssuer != "https://pocketid.townsville.cc" {
		t.Errorf("OIDCIssuer = %q", resp.OIDCIssuer)
	}
	found := false
	for _, m := range resp.RoleMappings {
		if m.GroupClaim == "admins" && m.Role == "admin" {
			found = true
		}
	}
	if !found {
		t.Errorf("RoleMappings = %+v, want to contain admins->admin", resp.RoleMappings)
	}
}

func TestUpdateAuthSettings_UpsertsMappings(t *testing.T) {
	s, st := newTestServer(t)
	token := authToken(t, st, "admin", 5)

	body := `{"role_mappings":[{"group_claim":"homelab","role":"viewer","priority":100}]}`
	req := httptest.NewRequest(http.MethodPut, "/settings/auth", bytes.NewReader([]byte(body)))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204, body=%s", rec.Code, rec.Body.String())
	}

	mappings, err := st.ListRoleMappings(context.Background())
	if err != nil {
		t.Fatalf("ListRoleMappings() error = %v", err)
	}
	found := false
	for _, m := range mappings {
		if m.GroupClaim == "homelab" && m.Role == "viewer" {
			found = true
		}
	}
	if !found {
		t.Errorf("mappings = %+v, want to contain homelab->viewer", mappings)
	}
}
