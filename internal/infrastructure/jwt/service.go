package jwtservice

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// Claims is the standard JWT payload used for access tokens.
// token_version is embedded so middleware can reject stale tokens without a DB round-trip
// on every request — only mismatch triggers a DB check.
type Claims struct {
	jwt.RegisteredClaims
	TokenVersion int    `json:"token_version"`
	Purpose      string `json:"purpose,omitempty"` // "access" | "refresh" | "password_reset"
}

// JWTService generates and parses JWTs for auth flows.
type JWTService struct {
	secret []byte
}

func NewJWTService(secret string) *JWTService {
	return &JWTService{secret: []byte(secret)}
}

// GenerateAccessToken creates a short-lived access token with token_version embedded.
// TTL is typically 15 minutes (from JWT_ACCESS_TTL env).
func (s *JWTService) GenerateAccessToken(userID string, tokenVersion int, ttl time.Duration) (string, error) {
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			ID:        uuid.New().String(),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(ttl)),
		},
		TokenVersion: tokenVersion,
		Purpose:      "access",
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.secret)
}

// GenerateRefreshToken creates a long-lived refresh token (purpose="refresh").
// TTL is typically 7–30 days.
func (s *JWTService) GenerateRefreshToken(userID string, ttl time.Duration) (string, error) {
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			ID:        uuid.New().String(),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(ttl)),
		},
		Purpose: "refresh",
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.secret)
}

// GenerateResetToken creates a single-use password-reset token (purpose="password_reset").
// The jti claim must be stored in Redis and consumed atomically on use.
// TTL is typically 15 minutes (from OTP_EXPIRY_MINUTES env).
func (s *JWTService) GenerateResetToken(userID string, ttl time.Duration) (jti string, tokenStr string, err error) {
	jti = uuid.New().String()
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			ID:        jti,
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(ttl)),
		},
		Purpose: "password_reset",
	}
	tokenStr, err = jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.secret)
	return jti, tokenStr, err
}

// ParseAccessToken parses and validates an access token, returning its claims.
func (s *JWTService) ParseAccessToken(tokenStr string) (*Claims, error) {
	return s.parse(tokenStr, "access")
}

// ParseRefreshToken parses and validates a refresh token.
func (s *JWTService) ParseRefreshToken(tokenStr string) (*Claims, error) {
	return s.parse(tokenStr, "refresh")
}

// ParseResetToken parses and validates a password reset token.
func (s *JWTService) ParseResetToken(tokenStr string) (*Claims, error) {
	return s.parse(tokenStr, "password_reset")
}

func (s *JWTService) parse(tokenStr, expectedPurpose string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("jwt: unexpected signing method: %v", t.Header["alg"])
		}
		return s.secret, nil
	})
	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("jwt: invalid token")
	}
	if claims.Purpose != expectedPurpose {
		return nil, fmt.Errorf("jwt: token purpose mismatch: got %q, want %q", claims.Purpose, expectedPurpose)
	}

	return claims, nil
}
