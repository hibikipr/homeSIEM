package store

import (
	"context"
	"testing"
	"time"
)

func TestUpsertUserBySubject(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	created, err := s.UpsertUserBySubject(ctx, "oidc-sub-1", "alice@townsville.cc", "Alice", "analyst")
	if err != nil {
		t.Fatalf("UpsertUserBySubject() error = %v", err)
	}
	if created.ID == 0 {
		t.Error("UpsertUserBySubject() ID = 0, want nonzero")
	}

	// Same subject again should update, not duplicate.
	updated, err := s.UpsertUserBySubject(ctx, "oidc-sub-1", "alice@townsville.cc", "Alice", "admin")
	if err != nil {
		t.Fatalf("second UpsertUserBySubject() error = %v", err)
	}
	if updated.ID != created.ID {
		t.Errorf("second UpsertUserBySubject() ID = %d, want %d (same user)", updated.ID, created.ID)
	}
	if updated.Role != "admin" {
		t.Errorf("Role = %q, want admin", updated.Role)
	}
}

func TestTouchUserLogin(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	created, err := s.UpsertUserBySubject(ctx, "oidc-sub-1", "alice@townsville.cc", "Alice", "analyst")
	if err != nil {
		t.Fatalf("UpsertUserBySubject() error = %v", err)
	}

	if err := s.TouchUserLogin(ctx, created.ID, time.Now().UTC()); err != nil {
		t.Fatalf("TouchUserLogin() error = %v", err)
	}

	entries, err := s.ListAudit(ctx, 10)
	if err != nil {
		t.Fatalf("ListAudit() error = %v", err)
	}
	found := false
	for _, e := range entries {
		if e.Action == "auth.login" {
			found = true
		}
	}
	if !found {
		t.Error("no auth.login audit entry found")
	}
}

func TestEnsureLocalAdmin_IdempotentAndFindable(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	first, err := s.EnsureLocalAdmin(ctx, "admin", "bcrypt-hash-1")
	if err != nil {
		t.Fatalf("EnsureLocalAdmin() error = %v", err)
	}

	// Calling again with a different hash must NOT overwrite the existing row.
	second, err := s.EnsureLocalAdmin(ctx, "admin", "bcrypt-hash-2")
	if err != nil {
		t.Fatalf("second EnsureLocalAdmin() error = %v", err)
	}
	if second.ID != first.ID {
		t.Errorf("second EnsureLocalAdmin() ID = %d, want %d", second.ID, first.ID)
	}

	found, err := s.GetLocalAdminByUsername(ctx, "admin")
	if err != nil {
		t.Fatalf("GetLocalAdminByUsername() error = %v", err)
	}
	if found == nil {
		t.Fatal("GetLocalAdminByUsername() = nil, want a user")
	}
	if found.LocalHash == nil || *found.LocalHash != "bcrypt-hash-1" {
		t.Errorf("LocalHash = %v, want bcrypt-hash-1 (unchanged by second EnsureLocalAdmin)", found.LocalHash)
	}
}

func TestEnsureBootstrapRoleMapping_SeedsOnlyWhenTableEmpty(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	m, created, err := s.EnsureBootstrapRoleMapping(ctx, "admins")
	if err != nil {
		t.Fatalf("EnsureBootstrapRoleMapping() error = %v", err)
	}
	if !created {
		t.Fatal("EnsureBootstrapRoleMapping() created = false on an empty table, want true")
	}
	if m.GroupClaim != "admins" || m.Role != "admin" {
		t.Errorf("EnsureBootstrapRoleMapping() = %+v, want group_claim=admins role=admin", m)
	}

	// A second call, even with a different group, must not touch anything -
	// the table is no longer empty, whether or not the admin has since
	// edited or added to what's there.
	if _, err := s.UpsertRoleMapping(ctx, RoleMapping{GroupClaim: "sre", Role: "analyst", Priority: 5}); err != nil {
		t.Fatalf("UpsertRoleMapping() error = %v", err)
	}
	_, created, err = s.EnsureBootstrapRoleMapping(ctx, "someone-else")
	if err != nil {
		t.Fatalf("second EnsureBootstrapRoleMapping() error = %v", err)
	}
	if created {
		t.Error("EnsureBootstrapRoleMapping() created = true on a non-empty table, want false")
	}

	mappings, err := s.ListRoleMappings(ctx)
	if err != nil {
		t.Fatalf("ListRoleMappings() error = %v", err)
	}
	if len(mappings) != 2 {
		t.Errorf("ListRoleMappings() = %d mappings, want 2 (no bootstrap re-seed)", len(mappings))
	}
}

func TestGetLocalAdminByUsername_NotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	found, err := s.GetLocalAdminByUsername(ctx, "nobody")
	if err != nil {
		t.Fatalf("GetLocalAdminByUsername() error = %v", err)
	}
	if found != nil {
		t.Fatalf("GetLocalAdminByUsername() = %+v, want nil", found)
	}
}

func TestResolveRole_FirstMatchWinsAndDeny(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if _, err := s.UpsertRoleMapping(ctx, RoleMapping{GroupClaim: "admins", Role: "admin", Priority: 10}); err != nil {
		t.Fatalf("UpsertRoleMapping() error = %v", err)
	}
	if _, err := s.UpsertRoleMapping(ctx, RoleMapping{GroupClaim: "homelab", Role: "viewer", Priority: 100}); err != nil {
		t.Fatalf("UpsertRoleMapping() error = %v", err)
	}

	role, ok := s.ResolveRole(ctx, []string{"homelab", "admins"})
	if !ok || role != "admin" {
		t.Errorf("ResolveRole(homelab,admins) = (%q, %v), want (admin, true) — lowest priority wins", role, ok)
	}

	role, ok = s.ResolveRole(ctx, []string{"unmapped-group"})
	if ok {
		t.Errorf("ResolveRole(unmapped-group) ok = true, want false (deny)")
	}
	_ = role
}
