package auth

import (
	"fmt"

	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	UserID      int64
	Subject     string
	Email       string
	DisplayName string
	Groups      []string
}

// jwtClaims is the wire format minted by siem-web's BFF; TokenVerifier only
// ever decodes it, never encodes it (minting lives in the BFF, out of scope
// for siem-api).
type jwtClaims struct {
	jwt.RegisteredClaims
	UserID      int64    `json:"user_id"`
	Email       string   `json:"email"`
	DisplayName string   `json:"display_name"`
	Groups      []string `json:"groups"`
}

type TokenVerifier struct {
	secret []byte
}

func NewTokenVerifier(secret []byte) *TokenVerifier {
	return &TokenVerifier{secret: secret}
}

func (v *TokenVerifier) Verify(tokenString string) (Claims, error) {
	var claims jwtClaims
	token, err := jwt.ParseWithClaims(tokenString, &claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("auth: unexpected signing method %v", t.Header["alg"])
		}
		return v.secret, nil
	})
	if err != nil {
		return Claims{}, fmt.Errorf("auth: verify token: %w", err)
	}
	if !token.Valid {
		return Claims{}, fmt.Errorf("auth: token invalid")
	}

	return Claims{
		UserID:      claims.UserID,
		Subject:     claims.Subject,
		Email:       claims.Email,
		DisplayName: claims.DisplayName,
		Groups:      claims.Groups,
	}, nil
}
