package auth

// Session lifecycle: login → refresh (rotation) → logout / logout-all.
// Everything that mints or revokes a session pair lives in this file.

import (
	"context"
	"fmt"
	"time"

	"github.com/aprxty3/your_persona_controller.git/internal/application"
	"github.com/aprxty3/your_persona_controller.git/internal/domain/user"
	"github.com/aprxty3/your_persona_controller.git/internal/infrastructure/cache/redis"
	jwtservice "github.com/aprxty3/your_persona_controller.git/internal/infrastructure/jwt"
	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
	"golang.org/x/crypto/bcrypt"
)

// SessionUseCase manages authentication sessions, JWT issuance, and revocation.
type SessionUseCase struct {
	userRepo         user.Repository
	jwtService       *jwtservice.JWTService
	tokenStore       *redis.TokenStore
	log              logger.Logger
	loginMaxAttempts int
	lockDuration     time.Duration
}

// NewSessionUseCase creates a new SessionUseCase.
func NewSessionUseCase(
	userRepo user.Repository,
	jwtService *jwtservice.JWTService,
	tokenStore *redis.TokenStore,
	log logger.Logger,
) *SessionUseCase {
	return &SessionUseCase{
		userRepo:         userRepo,
		jwtService:       jwtService,
		tokenStore:       tokenStore,
		log:              log.With("usecase", "session"),
		loginMaxAttempts: defaultLoginMaxAttempts,
		lockDuration:     defaultLockDuration,
	}
}

// Session token TTLs shared by every flow that issues a session
const (
	AccessTokenTTL  = 15 * time.Minute
	RefreshTokenTTL = 14 * 24 * time.Hour
)

const (
	defaultLoginMaxAttempts = 10
	defaultLockDuration     = 15 * time.Minute
)

// TokenPair carries one full JWT session credential set.
type TokenPair struct {
	AccessToken  string
	RefreshToken string
}

// LoginRequest is the payload for the authenticate user endpoint.
type LoginRequest struct {
	Email    string
	Password string
}

// LoginResponse carries JWT credentials on successful login.
type LoginResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

// RefreshTokenRequest carries the long-lived credential to be exchanged.
type RefreshTokenRequest struct {
	RefreshToken string
}

// RefreshTokenResponse returns a fresh session pair (refresh token rotation).
type RefreshTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

// LogoutRequest carries the refresh token of the session being terminated.
type LogoutRequest struct {
	UserID       string
	RefreshToken string
}

// IssueTokenPair is the single shared way to mint a session (access + refresh).
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

// LoginUseCase validates password hashes and generates session JWTs.
func (uc *SessionUseCase) Login(ctx context.Context, req LoginRequest) (*LoginResponse, error) {
	u, err := uc.userRepo.FindByEmail(ctx, req.Email)
	if err != nil {
		uc.log.Error("login failed", "step", "query_email", "error", err)
		return nil, fmt.Errorf("login: query email: %w", err)
	}
	if u == nil {
		uc.log.Warn("login rejected", "reason", "invalid_credentials")
		return nil, application.ErrInvalidCredentials
	}

	if u.IsLocked() {
		uc.log.Warn("login rejected", "reason", "account_locked", "user_id", u.ID)
		return nil, application.ErrAccountLocked
	}

	if !u.IsEmailVerified() {
		uc.log.Warn("login rejected", "reason", "email_not_verified", "user_id", u.ID)
		return nil, application.ErrEmailNotVerified
	}

	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(req.Password)); err != nil {
		return nil, uc.handleFailedAttempt(ctx, u)
	}

	if u.FailedLoginCount > 0 {
		if err := uc.userRepo.UpdateLoginAttempt(ctx, u.ID, 0, nil); err != nil {
			uc.log.Warn("failed to reset login attempt counter", "user_id", u.ID, "error", err)
		}
	}

	pair, err := IssueTokenPair(uc.jwtService, u.ID, u.TokenVersion)
	if err != nil {
		uc.log.Error("login failed", "step", "issue_tokens", "user_id", u.ID, "error", err)
		return nil, fmt.Errorf("login: %w", err)
	}

	uc.log.Info("user logged in", "user_id", u.ID)
	return &LoginResponse{
		AccessToken:  pair.AccessToken,
		RefreshToken: pair.RefreshToken,
	}, nil
}

func (uc *SessionUseCase) handleFailedAttempt(ctx context.Context, u *user.User) error {
	newCount := u.FailedLoginCount + 1
	var lockedUntil *time.Time

	if newCount >= uc.loginMaxAttempts {
		lockTime := time.Now().Add(uc.lockDuration)
		lockedUntil = &lockTime
	}

	if err := uc.userRepo.UpdateLoginAttempt(ctx, u.ID, newCount, lockedUntil); err != nil {
		uc.log.Warn("failed to persist login attempt counter", "user_id", u.ID, "error", err)
	}

	if lockedUntil != nil {
		uc.log.Warn("login rejected", "reason", "account_locked", "user_id", u.ID, "failed_count", newCount)
		return application.ErrAccountLocked
	}
	uc.log.Warn("login rejected", "reason", "invalid_credentials", "user_id", u.ID, "failed_count", newCount)
	return application.ErrInvalidCredentials
}

// RefreshTokenUseCase exchanges a valid refresh token for a brand-new session
func (uc *SessionUseCase) RefreshToken(ctx context.Context, req RefreshTokenRequest) (*RefreshTokenResponse, error) {
	if err := application.ValidateRequired("refresh_token", req.RefreshToken); err != nil {
		return nil, err
	}

	claims, err := uc.jwtService.ParseRefreshToken(req.RefreshToken)
	if err != nil {
		uc.log.Warn("refresh rejected", "reason", "invalid_token", "error", err)
		return nil, application.ErrInvalidToken
	}

	denied, err := uc.tokenStore.IsRefreshJTIDenylisted(ctx, claims.ID)
	if err != nil {
		uc.log.Warn("refresh denylist check skipped", "reason", "redis_error", "error", err)
	} else if denied {
		uc.log.Warn("refresh rejected", "reason", "token_revoked", "user_id", claims.Subject)
		return nil, application.ErrInvalidToken
	}

	u, err := uc.userRepo.FindByID(ctx, claims.Subject)
	if err != nil {
		uc.log.Error("refresh failed", "step", "lookup_user", "error", err)
		return nil, fmt.Errorf("refresh: lookup user: %w", err)
	}
	if u == nil {
		uc.log.Warn("refresh rejected", "reason", "user_not_found")
		return nil, application.ErrInvalidToken
	}

	if claims.TokenVersion != u.TokenVersion {
		uc.log.Warn("refresh rejected", "reason", "token_version_mismatch", "user_id", u.ID)
		return nil, application.ErrTokenVersionMismatch
	}

	pair, err := IssueTokenPair(uc.jwtService, u.ID, u.TokenVersion)
	if err != nil {
		uc.log.Error("refresh failed", "step", "issue_tokens", "user_id", u.ID, "error", err)
		return nil, fmt.Errorf("refresh: %w", err)
	}

	if claims.ExpiresAt != nil {
		if err := uc.tokenStore.DenylistRefreshJTI(ctx, claims.ID, time.Until(claims.ExpiresAt.Time)); err != nil {
			uc.log.Warn("failed to denylist rotated refresh token", "user_id", u.ID, "error", err)
		}
	}

	uc.log.Info("session refreshed", "user_id", u.ID)
	return &RefreshTokenResponse{AccessToken: pair.AccessToken, RefreshToken: pair.RefreshToken}, nil
}

// LogoutUseCase terminates ONE session: the presented refresh token
func (uc *SessionUseCase) Logout(ctx context.Context, req LogoutRequest) error {
	if err := application.ValidateRequired("refresh_token", req.RefreshToken); err != nil {
		return err
	}

	claims, err := uc.jwtService.ParseRefreshToken(req.RefreshToken)
	if err != nil {
		uc.log.Info("logout no-op", "reason", "token_already_invalid", "user_id", req.UserID)
		return nil
	}

	if claims.Subject != req.UserID {
		uc.log.Warn("logout rejected", "reason", "subject_mismatch", "user_id", req.UserID)
		return application.ErrInvalidToken
	}

	if claims.ExpiresAt != nil {
		if err := uc.tokenStore.DenylistRefreshJTI(ctx, claims.ID, time.Until(claims.ExpiresAt.Time)); err != nil {
			uc.log.Warn("failed to denylist refresh token on logout", "user_id", req.UserID, "error", err)
		}
	}

	uc.log.Info("user logged out", "user_id", req.UserID)
	return nil
}

// LogoutAllUseCase terminates EVERY session of a user
func (uc *SessionUseCase) LogoutAll(ctx context.Context, userID string) error {
	if err := uc.userRepo.IncrementTokenVersion(ctx, userID); err != nil {
		uc.log.Error("logout-all failed", "step", "increment_token_version", "user_id", userID, "error", err)
		return fmt.Errorf("logout_all: increment token version: %w", err)
	}
	uc.log.Info("all sessions revoked", "user_id", userID)
	return nil
}
