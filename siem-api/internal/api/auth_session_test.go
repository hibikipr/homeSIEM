package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hibikipr/homeSIEM/siem-api/internal/auth"
	"github.com/hibikipr/homeSIEM/siem-api/internal/store"
	"golang.org/x/crypto/bcrypt"
)

func withAuthDeps(s *Server) {
	s.deps.SessionEst = auth.NewSessionEstablisher(s.deps.Store, s.deps.Store)
	s.deps.LocalAuth = auth.NewLocalAuthenticator(s.deps.Store)
}

func TestAuthSession_MappedGroupSucceeds(t *testing.T) {
	s, st := newTestServer(t)
	withAuthDeps(s)
	if _, err := st.UpsertRoleMapping(context.Background(), store.RoleMapping{GroupClaim: "siem-analysts", Role: "analyst", Priority: 50}); err != nil {
		t.Fatalf("UpsertRoleMapping() error = %v", err)
	}

	body := `{"subject":"sub-1","email":"alice@townsville.cc","display_name":"Alice","groups":["siem-analysts"]}`
	req := httptest.NewRequest(http.MethodPost, "/auth/session", bytes.NewReader([]byte(body)))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	var resp sessionResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if resp.Role != "analyst" || resp.UserID == 0 {
		t.Errorf("resp = %+v", resp)
	}
}

func TestAuthSession_UnmappedGroupDenied(t *testing.T) {
	s, _ := newTestServer(t)
	withAuthDeps(s)

	body := `{"subject":"sub-1","email":"a@b.c","display_name":"A","groups":["no-mapping"]}`
	req := httptest.NewRequest(http.MethodPost, "/auth/session", bytes.NewReader([]byte(body)))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestAuthLocal_ValidCredentialsSucceed(t *testing.T) {
	s, st := newTestServer(t)
	withAuthDeps(s)
	hash, err := bcrypt.GenerateFromPassword([]byte("correct-horse"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("GenerateFromPassword() error = %v", err)
	}
	if _, err := st.EnsureLocalAdmin(context.Background(), "admin", string(hash)); err != nil {
		t.Fatalf("EnsureLocalAdmin() error = %v", err)
	}

	body := `{"username":"admin","password":"correct-horse"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/local", bytes.NewReader([]byte(body)))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	var resp sessionResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if resp.Role != "admin" {
		t.Errorf("Role = %q, want admin", resp.Role)
	}
}

func TestAuthLocal_InvalidCredentialsRejected(t *testing.T) {
	s, _ := newTestServer(t)
	withAuthDeps(s)

	body := `{"username":"ghost","password":"anything"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/local", bytes.NewReader([]byte(body)))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}
