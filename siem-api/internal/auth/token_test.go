package auth

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func mintTestToken(t *testing.T, secret []byte, claims jwtClaims) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(secret)
	if err != nil {
		t.Fatalf("SignedString() error = %v", err)
	}
	return signed
}

func TestVerify_ValidToken(t *testing.T) {
	secret := []byte("0123456789abcdef0123456789abcdef")
	v := NewTokenVerifier(secret)

	claims := jwtClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "oidc-sub-1",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
		UserID:      42,
		Email:       "alice@townsville.cc",
		DisplayName: "Alice",
		Groups:      []string{"siem-analysts"},
	}
	signed := mintTestToken(t, secret, claims)

	got, err := v.Verify(signed)
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if got.Subject != "oidc-sub-1" || got.Email != "alice@townsville.cc" {
		t.Errorf("Verify() = %+v", got)
	}
	if got.UserID != 42 {
		t.Errorf("UserID = %d, want 42", got.UserID)
	}
	if len(got.Groups) != 1 || got.Groups[0] != "siem-analysts" {
		t.Errorf("Groups = %v, want [siem-analysts]", got.Groups)
	}
}

func TestVerify_WrongSecret(t *testing.T) {
	signingSecret := []byte("0123456789abcdef0123456789abcdef")
	verifySecret := []byte("ffffffffffffffffffffffffffffffff")

	claims := jwtClaims{RegisteredClaims: jwt.RegisteredClaims{
		Subject: "oidc-sub-1", ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}}
	signed := mintTestToken(t, signingSecret, claims)

	v := NewTokenVerifier(verifySecret)
	if _, err := v.Verify(signed); err == nil {
		t.Fatal("Verify() error = nil, want error for wrong secret")
	}
}

func TestVerify_Expired(t *testing.T) {
	secret := []byte("0123456789abcdef0123456789abcdef")
	claims := jwtClaims{RegisteredClaims: jwt.RegisteredClaims{
		Subject: "oidc-sub-1", ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Hour)),
	}}
	signed := mintTestToken(t, secret, claims)

	v := NewTokenVerifier(secret)
	if _, err := v.Verify(signed); err == nil {
		t.Fatal("Verify() error = nil, want error for expired token")
	}
}

func TestVerify_Malformed(t *testing.T) {
	v := NewTokenVerifier([]byte("0123456789abcdef0123456789abcdef"))
	if _, err := v.Verify("not-a-jwt"); err == nil {
		t.Fatal("Verify() error = nil, want error for malformed token")
	}
}
