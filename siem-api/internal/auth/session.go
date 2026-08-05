package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/hibikipr/homeSIEM/siem-api/internal/store"
	"golang.org/x/crypto/bcrypt"
)

type SessionStore interface {
	UpsertUserBySubject(ctx context.Context, subject, email, displayName, role string) (store.User, error)
	TouchUserLogin(ctx context.Context, id int64, at time.Time) error
}

type SessionEstablisher struct {
	store    SessionStore
	resolver RoleResolver
}

func NewSessionEstablisher(s SessionStore, r RoleResolver) *SessionEstablisher {
	return &SessionEstablisher{store: s, resolver: r}
}

func (e *SessionEstablisher) Establish(ctx context.Context, subject, email, displayName string, groups []string) (store.User, error) {
	role, ok := e.resolver.ResolveRole(ctx, groups)
	if !ok {
		return store.User{}, fmt.Errorf("auth: no role mapping for groups %v", groups)
	}

	u, err := e.store.UpsertUserBySubject(ctx, subject, email, displayName, role)
	if err != nil {
		return store.User{}, err
	}

	if err := e.store.TouchUserLogin(ctx, u.ID, time.Now()); err != nil {
		return store.User{}, err
	}
	return u, nil
}

type LocalUserStore interface {
	GetLocalAdminByUsername(ctx context.Context, username string) (*store.User, error)
	TouchUserLogin(ctx context.Context, id int64, at time.Time) error
}

type LocalAuthenticator struct {
	store LocalUserStore
}

func NewLocalAuthenticator(s LocalUserStore) *LocalAuthenticator {
	return &LocalAuthenticator{store: s}
}

func (a *LocalAuthenticator) Login(ctx context.Context, username, password string) (store.User, error) {
	u, err := a.store.GetLocalAdminByUsername(ctx, username)
	if err != nil {
		return store.User{}, err
	}
	if u == nil || u.LocalHash == nil {
		return store.User{}, fmt.Errorf("auth: invalid credentials")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(*u.LocalHash), []byte(password)); err != nil {
		return store.User{}, fmt.Errorf("auth: invalid credentials")
	}

	if err := a.store.TouchUserLogin(ctx, u.ID, time.Now()); err != nil {
		return store.User{}, err
	}
	return *u, nil
}
