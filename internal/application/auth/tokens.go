package auth

import (
	"fmt"
	"time"

	jwtservice "github.com/aprxty3/your_persona_controller.git/internal/infrastructure/jwt"
)

// Session token TTLs shared by every flow that issues a session
// (login, refresh, reset-password auto-login).
const (
	AccessTokenTTL  = 15 * time.Minute
	RefreshTokenTTL = 14 * 24 * time.Hour
)

// TokenPair carries one full JWT session credential set.
type TokenPair struct {
	AccessToken  string
	RefreshToken string
}

// IssueTokenPair is the single shared way to mint a session (access + refresh).
// Both tokens embed token_version so a version bump revokes the whole pair.
func IssueTokenPair(jwtService *jwtservice.JWTService, userID string, tokenVersion int) (*TokenPair, error) {
	accessToken, err := jwtService.GenerateAccessToken(userID, tokenVersion, AccessTokenTTL)
	if err != nil {
		return nil, fmt.Errorf("issue access token: %w", err)
	}
	refreshToken, err := jwtService.GenerateRefreshToken(userID, tokenVersion, RefreshTokenTTL)
	if err != nil {
		return nil, fmt.Errorf("issue refresh token: %w", err)
	}
	return &TokenPair{AccessToken: accessToken, RefreshToken: refreshToken}, nil
}
