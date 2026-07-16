package jwtservice

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestAccessToken_RoundTrip(t *testing.T) {
	s := NewJWTService("test-secret")

	tokenStr, err := s.GenerateAccessToken("user-1", 3, time.Hour)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	claims, err := s.ParseAccessToken(tokenStr)
	if err != nil {
		t.Fatalf("unexpected error parsing: %v", err)
	}
	if claims.Subject != "user-1" {
		t.Errorf("expected subject user-1, got %q", claims.Subject)
	}
	if claims.TokenVersion != 3 {
		t.Errorf("expected token version 3, got %d", claims.TokenVersion)
	}
	if claims.Purpose != purposeAccess {
		t.Errorf("expected purpose %q, got %q", purposeAccess, claims.Purpose)
	}
}

func TestRefreshToken_RoundTrip(t *testing.T) {
	s := NewJWTService("test-secret")

	tokenStr, err := s.GenerateRefreshToken("user-2", 1, time.Hour)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	claims, err := s.ParseRefreshToken(tokenStr)
	if err != nil {
		t.Fatalf("unexpected error parsing: %v", err)
	}
	if claims.Subject != "user-2" {
		t.Errorf("expected subject user-2, got %q", claims.Subject)
	}
	if claims.Purpose != purposeRefresh {
		t.Errorf("expected purpose %q, got %q", purposeRefresh, claims.Purpose)
	}
}

func TestResetToken_RoundTrip(t *testing.T) {
	s := NewJWTService("test-secret")

	jti, tokenStr, err := s.GenerateResetToken("user-3", time.Hour)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if jti == "" {
		t.Fatal("expected a non-empty jti")
	}

	claims, err := s.ParseResetToken(tokenStr)
	if err != nil {
		t.Fatalf("unexpected error parsing: %v", err)
	}
	if claims.ID != jti {
		t.Errorf("expected claims.ID to match returned jti %q, got %q", jti, claims.ID)
	}
	if claims.Purpose != purposeResetPassword {
		t.Errorf("expected purpose %q, got %q", purposeResetPassword, claims.Purpose)
	}
}

// A token minted for one purpose must never validate as another — the whole
// point of embedding Purpose in the claims (e.g. a password-reset token
// replayed as an access token must be rejected).
func TestParse_PurposeMismatch_Rejected(t *testing.T) {
	s := NewJWTService("test-secret")

	tokenStr, err := s.GenerateAccessToken("user-4", 0, time.Hour)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := s.ParseRefreshToken(tokenStr); err == nil {
		t.Fatal("expected an access token to fail refresh-token parsing")
	}
	if _, err := s.ParseResetToken(tokenStr); err == nil {
		t.Fatal("expected an access token to fail reset-token parsing")
	}
}

func TestParse_ExpiredToken_Rejected(t *testing.T) {
	s := NewJWTService("test-secret")

	tokenStr, err := s.GenerateAccessToken("user-5", 0, -time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := s.ParseAccessToken(tokenStr); err == nil {
		t.Fatal("expected an expired token to fail parsing")
	}
}

func TestParse_WrongSecret_Rejected(t *testing.T) {
	s1 := NewJWTService("secret-one")
	s2 := NewJWTService("secret-two")

	tokenStr, err := s1.GenerateAccessToken("user-6", 0, time.Hour)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := s2.ParseAccessToken(tokenStr); err == nil {
		t.Fatal("expected a token signed with a different secret to fail parsing")
	}
}

func TestParse_MalformedToken_Rejected(t *testing.T) {
	s := NewJWTService("test-secret")

	if _, err := s.ParseAccessToken("not-a-valid-jwt"); err == nil {
		t.Fatal("expected a malformed token string to fail parsing")
	}
}

// A token signed with alg=none (or any non-HMAC method) must be rejected
// outright — accepting it would let an attacker forge tokens without ever
// knowing the server's secret.
func TestParse_UnexpectedSigningMethod_Rejected(t *testing.T) {
	s := NewJWTService("test-secret")

	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user-7",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
		Purpose: purposeAccess,
	}
	unsigned := jwt.NewWithClaims(jwt.SigningMethodNone, claims)
	tokenStr, err := unsigned.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("unexpected error building alg=none token: %v", err)
	}

	if _, err := s.ParseAccessToken(tokenStr); err == nil {
		t.Fatal("expected an alg=none token to be rejected")
	}
}

func TestGenerate_ProducesUniqueJTIs(t *testing.T) {
	s := NewJWTService("test-secret")

	jti1, _, err := s.GenerateResetToken("user-8", time.Hour)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	jti2, _, err := s.GenerateResetToken("user-8", time.Hour)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if jti1 == jti2 {
		t.Fatal("expected distinct jtis across separate token generations")
	}
}
