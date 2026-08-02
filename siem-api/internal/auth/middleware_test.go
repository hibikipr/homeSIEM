package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type fakeResolver struct {
	roles map[string]string // group -> role
}

func (f *fakeResolver) ResolveRole(ctx context.Context, groups []string) (string, bool) {
	for _, g := range groups {
		if role, ok := f.roles[g]; ok {
			return role, true
		}
	}
	return "", false
}

func echoHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, role, ok := UserFromContext(r.Context())
		if !ok {
			http.Error(w, "no user in context", http.StatusInternalServerError)
			return
		}
		w.Header().Set("X-User-ID", itoa(userID))
		w.Header().Set("X-Role", role)
		w.WriteHeader(http.StatusOK)
	})
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func signedTestToken(t *testing.T, secret []byte, userID int64, groups []string) string {
	t.Helper()
	claims := jwtClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "sub-1",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
		UserID: userID,
		Groups: groups,
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(secret)
	if err != nil {
		t.Fatalf("SignedString() error = %v", err)
	}
	return token
}

func TestMiddleware_ValidTokenAttachesUserAndRole(t *testing.T) {
	secret := []byte("0123456789abcdef0123456789abcdef")
	resolver := &fakeResolver{roles: map[string]string{"siem-analysts": "analyst"}}
	mw := Middleware(NewTokenVerifier(secret), resolver)
	handler := mw(echoHandler())

	req := httptest.NewRequest(http.MethodGet, "/alerts", nil)
	req.Header.Set("Authorization", "Bearer "+signedTestToken(t, secret, 7, []string{"siem-analysts"}))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if rec.Header().Get("X-User-ID") != "7" {
		t.Errorf("X-User-ID = %q, want 7", rec.Header().Get("X-User-ID"))
	}
	if rec.Header().Get("X-Role") != "analyst" {
		t.Errorf("X-Role = %q, want analyst", rec.Header().Get("X-Role"))
	}
}

func TestMiddleware_MissingToken(t *testing.T) {
	secret := []byte("0123456789abcdef0123456789abcdef")
	mw := Middleware(NewTokenVerifier(secret), &fakeResolver{})
	handler := mw(echoHandler())

	req := httptest.NewRequest(http.MethodGet, "/alerts", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestMiddleware_UnmappedGroupDenied(t *testing.T) {
	secret := []byte("0123456789abcdef0123456789abcdef")
	resolver := &fakeResolver{roles: map[string]string{"admins": "admin"}}
	mw := Middleware(NewTokenVerifier(secret), resolver)
	handler := mw(echoHandler())

	req := httptest.NewRequest(http.MethodGet, "/alerts", nil)
	req.Header.Set("Authorization", "Bearer "+signedTestToken(t, secret, 7, []string{"some-other-group"}))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestRequireRole_InsufficientDenied(t *testing.T) {
	secret := []byte("0123456789abcdef0123456789abcdef")
	resolver := &fakeResolver{roles: map[string]string{"homelab": "viewer"}}
	mw := Middleware(NewTokenVerifier(secret), resolver)
	handler := mw(RequireRole("admin", echoHandler()))

	req := httptest.NewRequest(http.MethodPost, "/rules", nil)
	req.Header.Set("Authorization", "Bearer "+signedTestToken(t, secret, 7, []string{"homelab"}))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestRequireRole_SufficientAllowed(t *testing.T) {
	secret := []byte("0123456789abcdef0123456789abcdef")
	resolver := &fakeResolver{roles: map[string]string{"admins": "admin"}}
	mw := Middleware(NewTokenVerifier(secret), resolver)
	handler := mw(RequireRole("analyst", echoHandler()))

	req := httptest.NewRequest(http.MethodPost, "/rules", nil)
	req.Header.Set("Authorization", "Bearer "+signedTestToken(t, secret, 7, []string{"admins"}))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}
