package auth

import (
	"context"
	"net/http"
	"strings"
)

type RoleResolver interface {
	ResolveRole(ctx context.Context, groups []string) (role string, ok bool)
}

type ctxKey int

const (
	ctxUserIDKey ctxKey = iota
	ctxRoleKey
)

var roleRank = map[string]int{"viewer": 1, "analyst": 2, "admin": 3}

func Middleware(verifier *TokenVerifier, resolver RoleResolver) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			const prefix = "Bearer "
			if !strings.HasPrefix(header, prefix) {
				http.Error(w, "missing bearer token", http.StatusUnauthorized)
				return
			}

			claims, err := verifier.Verify(strings.TrimPrefix(header, prefix))
			if err != nil {
				http.Error(w, "invalid token", http.StatusUnauthorized)
				return
			}

			role, ok := resolver.ResolveRole(r.Context(), claims.Groups)
			if !ok {
				http.Error(w, "no role mapping for this identity", http.StatusForbidden)
				return
			}

			ctx := context.WithValue(r.Context(), ctxUserIDKey, claims.UserID)
			ctx = context.WithValue(ctx, ctxRoleKey, role)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func RequireRole(minRole string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, role, ok := UserFromContext(r.Context())
		if !ok || roleRank[role] < roleRank[minRole] {
			http.Error(w, "insufficient role", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func UserFromContext(ctx context.Context) (userID int64, role string, ok bool) {
	uid, uidOK := ctx.Value(ctxUserIDKey).(int64)
	r, roleOK := ctx.Value(ctxRoleKey).(string)
	if !uidOK || !roleOK {
		return 0, "", false
	}
	return uid, r, true
}
