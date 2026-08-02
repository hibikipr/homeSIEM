package auth

import (
	"context"
	"testing"
	"time"

	"github.com/hibikipr/homeSIEM/siem-api/internal/store"
	"golang.org/x/crypto/bcrypt"
)

type fakeSessionStore struct {
	users        map[string]store.User // keyed by subject
	nextID       int64
	loginTouches []int64
}

func (f *fakeSessionStore) UpsertUserBySubject(ctx context.Context, subject, email, displayName, role string) (store.User, error) {
	if u, ok := f.users[subject]; ok {
		u.Email, u.DisplayName, u.Role = &email, &displayName, role
		f.users[subject] = u
		return u, nil
	}
	f.nextID++
	u := store.User{ID: f.nextID, Subject: &subject, Email: &email, DisplayName: &displayName, Role: role}
	f.users[subject] = u
	return u, nil
}

func (f *fakeSessionStore) TouchUserLogin(ctx context.Context, id int64, at time.Time) error {
	f.loginTouches = append(f.loginTouches, id)
	return nil
}

func TestSessionEstablisher_Success(t *testing.T) {
	store := &fakeSessionStore{users: map[string]store.User{}}
	resolver := &fakeResolver{roles: map[string]string{"siem-analysts": "analyst"}}
	e := NewSessionEstablisher(store, resolver)

	u, err := e.Establish(context.Background(), "sub-1", "alice@townsville.cc", "Alice", []string{"siem-analysts"})
	if err != nil {
		t.Fatalf("Establish() error = %v", err)
	}
	if u.Role != "analyst" {
		t.Errorf("Role = %q, want analyst", u.Role)
	}
	if len(store.loginTouches) != 1 || store.loginTouches[0] != u.ID {
		t.Errorf("loginTouches = %v, want [%d]", store.loginTouches, u.ID)
	}
}

func TestSessionEstablisher_UnmappedGroupDenied(t *testing.T) {
	store := &fakeSessionStore{users: map[string]store.User{}}
	resolver := &fakeResolver{roles: map[string]string{}}
	e := NewSessionEstablisher(store, resolver)

	if _, err := e.Establish(context.Background(), "sub-1", "a@b.c", "A", []string{"no-mapping"}); err == nil {
		t.Fatal("Establish() error = nil, want error for unmapped groups")
	}
	if len(store.loginTouches) != 0 {
		t.Error("TouchUserLogin called for a denied session, want not called")
	}
}

type fakeLocalStore struct {
	user         *store.User
	loginTouches []int64
}

func (f *fakeLocalStore) GetLocalAdminByUsername(ctx context.Context, username string) (*store.User, error) {
	if f.user == nil || f.user.DisplayName == nil || *f.user.DisplayName != username {
		return nil, nil
	}
	return f.user, nil
}

func (f *fakeLocalStore) TouchUserLogin(ctx context.Context, id int64, at time.Time) error {
	f.loginTouches = append(f.loginTouches, id)
	return nil
}

func TestLocalAuthenticator_Success(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("correct-horse"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("GenerateFromPassword() error = %v", err)
	}
	hashStr := string(hash)
	username := "admin"
	fs := &fakeLocalStore{user: &store.User{ID: 1, DisplayName: &username, LocalHash: &hashStr, Role: "admin"}}
	a := NewLocalAuthenticator(fs)

	u, err := a.Login(context.Background(), "admin", "correct-horse")
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if u.ID != 1 {
		t.Errorf("ID = %d, want 1", u.ID)
	}
	if len(fs.loginTouches) != 1 {
		t.Errorf("loginTouches = %v, want one entry", fs.loginTouches)
	}
}

func TestLocalAuthenticator_WrongPassword(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("correct-horse"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("GenerateFromPassword() error = %v", err)
	}
	hashStr := string(hash)
	username := "admin"
	fs := &fakeLocalStore{user: &store.User{ID: 1, DisplayName: &username, LocalHash: &hashStr, Role: "admin"}}
	a := NewLocalAuthenticator(fs)

	if _, err := a.Login(context.Background(), "admin", "wrong-password"); err == nil {
		t.Fatal("Login() error = nil, want error for wrong password")
	}
	if len(fs.loginTouches) != 0 {
		t.Error("TouchUserLogin called on failed login, want not called")
	}
}

func TestLocalAuthenticator_UnknownUsername(t *testing.T) {
	fs := &fakeLocalStore{user: nil}
	a := NewLocalAuthenticator(fs)

	if _, err := a.Login(context.Background(), "ghost", "anything"); err == nil {
		t.Fatal("Login() error = nil, want error for unknown username")
	}
}
