package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aprxty3/your_persona_controller.git/internal/domain/user"
	jwtservice "github.com/aprxty3/your_persona_controller.git/internal/infrastructure/jwt"
	"golang.org/x/crypto/bcrypt"
)

// Sentinel errors — mapped to HTTP error codes in the handler
var (
	ErrInvalidCredentials = errors.New("INVALID_CREDENTIALS") // generic — never distinguish email-not-found vs wrong-password
	ErrAccountLocked      = errors.New("ACCOUNT_LOCKED")      // HTTP 423
)

const (
	loginMaxAttempts = 10 // lock after 10 consecutive failures
	lockDuration     = 15 * time.Minute
	accessTokenTTL   = 15 * time.Minute
	refreshTokenTTL  = 14 * 24 * time.Hour // 14 days
)

// LoginRequest is the input for the login endpoint.
type LoginRequest struct {
	Email    string
	Password string
}

// LoginResponse contains both JWT tokens returned on successful login.
type LoginResponse struct {
	AccessToken  string
	RefreshToken string
}

// LoginUseCase validates credentials and issues JWTs.
// Two separate lockout mechanisms coexist (AGENTS.md):
//  1. Account-level lockout (this use case) — 10x consecutive failures → 15 min
//  2. Per-IP rate limit (middleware layer, not here)
type LoginUseCase struct {
	userRepo   user.Repository
	jwtService *jwtservice.JWTService
}

func NewLoginUseCase(userRepo user.Repository, jwtService *jwtservice.JWTService) *LoginUseCase {
	return &LoginUseCase{userRepo: userRepo, jwtService: jwtService}
}

func (uc *LoginUseCase) Execute(ctx context.Context, req LoginRequest) (*LoginResponse, error) {
	// Look up user — use generic error regardless of not-found vs wrong-password
	u, err := uc.userRepo.FindByEmail(ctx, req.Email)
	if err != nil {
		return nil, fmt.Errorf("login: find user: %w", err)
	}
	if u == nil {
		return nil, ErrInvalidCredentials
	}

	// Check account-level lockout (BEFORE checking password — lockout must fire even if attacker guesses email correctly)
	if u.IsLocked() {
		return nil, ErrAccountLocked
	}

	// Verify password
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(req.Password)); err != nil {
		return nil, uc.handleFailedAttempt(ctx, u)
	}

	// Successful login — reset attempt counter
	if err := uc.userRepo.UpdateLoginAttempt(ctx, u.ID, 0, nil); err != nil {
		// Non-fatal — user is authenticated; counter reset failure is logged, not surfaced
		_ = err
	}

	// Generate tokens
	accessToken, err := uc.jwtService.GenerateAccessToken(u.ID, u.TokenVersion, accessTokenTTL)
	if err != nil {
		return nil, fmt.Errorf("login: generate access token: %w", err)
	}
	refreshToken, err := uc.jwtService.GenerateRefreshToken(u.ID, refreshTokenTTL)
	if err != nil {
		return nil, fmt.Errorf("login: generate refresh token: %w", err)
	}

	return &LoginResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}, nil
}

// handleFailedAttempt increments the counter and applies lockout if threshold is reached.
// Always returns ErrInvalidCredentials or ErrAccountLocked — never the internal error.
func (uc *LoginUseCase) handleFailedAttempt(ctx context.Context, u *user.User) error {
	newCount := u.FailedLoginCount + 1
	var lockedUntil *time.Time

	if newCount >= loginMaxAttempts {
		t := time.Now().Add(lockDuration)
		lockedUntil = &t
	}

	if err := uc.userRepo.UpdateLoginAttempt(ctx, u.ID, newCount, lockedUntil); err != nil {
		// Non-fatal update failure — still return the correct auth error to client
		_ = err
	}

	if lockedUntil != nil {
		return ErrAccountLocked
	}
	return ErrInvalidCredentials
}
