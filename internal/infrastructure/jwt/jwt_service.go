package jwtservice

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// Claims defines standard registered claims with additional attributes for access and revocation control.
type Claims struct {
	jwt.RegisteredClaims
	TokenVersion int    `json:"token_version"`
	Purpose      string `json:"purpose,omitempty"` // "access" | "refresh" | "password_reset"
}

// JWTService handles authorization credentials generation and verification.
type JWTService struct {
	secret []byte
}

// NewJWTService constructs a new JWTService.
func NewJWTService(secret string) *JWTService {
	return &JWTService{secret: []byte(secret)}
}

// GenerateAccessToken generates a short-lived token with token_version embedded.
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

// GenerateRefreshToken generates a long-lived persistence session token.
func (s *JWTService) GenerateRefreshToken(userID string, tokenVersion int, ttl time.Duration) (string, error) {
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			ID:        uuid.New().String(),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(ttl)),
		},
		TokenVersion: tokenVersion,
		Purpose:      "refresh",
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.secret)
}

// GenerateResetToken creates a single-use code exchange token.
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

// ParseAccessToken validates the access token structure and purpose.
func (s *JWTService) ParseAccessToken(tokenStr string) (*Claims, error) {
	return s.parse(tokenStr, "access")
}

// ParseRefreshToken validates the refresh token structure and purpose.
func (s *JWTService) ParseRefreshToken(tokenStr string) (*Claims, error) {
	return s.parse(tokenStr, "refresh")
}

// ParseResetToken validates the password reset token structure and purpose.
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
		return nil, fmt.Errorf("jwt: invalid token claims")
	}
	if claims.Purpose != expectedPurpose {
		return nil, fmt.Errorf("jwt: purpose mismatch: got %q, want %q", claims.Purpose, expectedPurpose)
	}

	return claims, nil
}
