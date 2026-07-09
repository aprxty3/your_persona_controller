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

// Session token TTLs shared by every flow that issues a session
// (login, refresh, reset-password auto-login).
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

// ---------------------------------------------------------------------------
// Login
// ---------------------------------------------------------------------------

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

// LoginUseCase validates password hashes and generates session JWTs.
type LoginUseCase struct {
	userRepo         user.Repository
	jwtService       *jwtservice.JWTService
	log              logger.Logger
	loginMaxAttempts int
	lockDuration     time.Duration
}

// NewLoginUseCase creates a new LoginUseCase with configurable defaults.
func NewLoginUseCase(userRepo user.Repository, jwtService *jwtservice.JWTService, log logger.Logger) *LoginUseCase {
	return &LoginUseCase{
		userRepo:         userRepo,
		jwtService:       jwtService,
		log:              log.With("usecase", "login"),
		loginMaxAttempts: defaultLoginMaxAttempts,
		lockDuration:     defaultLockDuration,
	}
}

// Execute authenticates a user and increments or resets login lockout policies.
func (uc *LoginUseCase) Execute(ctx context.Context, req LoginRequest) (*LoginResponse, error) {
	u, err := uc.userRepo.FindByEmail(ctx, req.Email)
	if err != nil {
		uc.log.Error("login failed", "step", "query_email", "error", err)
		return nil, fmt.Errorf("login: query email: %w", err)
	}
	if u == nil {
		uc.log.Warn("login rejected", "reason", "invalid_credentials")
		return nil, application.ErrInvalidCredentials
	}

	// Fast-fail if the account is currently locked out
	if u.IsLocked() {
		uc.log.Warn("login rejected", "reason", "account_locked", "user_id", u.ID)
		return nil, application.ErrAccountLocked
	}

	// Block login until email is verified.
	if !u.IsEmailVerified() {
		uc.log.Warn("login rejected", "reason", "email_not_verified", "user_id", u.ID)
		return nil, application.ErrEmailNotVerified
	}

	// Verify password hash
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(req.Password)); err != nil {
		return nil, uc.handleFailedAttempt(ctx, u)
	}

	// Clear failed login count upon successful authentication
	if u.FailedLoginCount > 0 {
		if err := uc.userRepo.UpdateLoginAttempt(ctx, u.ID, 0, nil); err != nil {
			uc.log.Warn("failed to reset login attempt counter", "user_id", u.ID, "error", err)
		}
	}

	// Issue JWT token pairs via the shared session-minting helper.
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

func (uc *LoginUseCase) handleFailedAttempt(ctx context.Context, u *user.User) error {
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

// ---------------------------------------------------------------------------
// Refresh (with rotation)
// ---------------------------------------------------------------------------

// RefreshTokenRequest carries the long-lived credential to be exchanged.
type RefreshTokenRequest struct {
	RefreshToken string
}

// RefreshTokenResponse returns a fresh session pair (refresh token rotation).
type RefreshTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

// RefreshTokenUseCase exchanges a valid refresh token for a brand-new session
// pair. The presented refresh token is rotated: its jti is denylisted for its
// remaining lifetime, so it cannot be replayed after the exchange.
type RefreshTokenUseCase struct {
	userRepo   user.Repository
	jwtService *jwtservice.JWTService
	tokenStore *redis.TokenStore
	log        logger.Logger
}

// NewRefreshTokenUseCase constructs a new RefreshTokenUseCase.
func NewRefreshTokenUseCase(
	userRepo user.Repository,
	jwtService *jwtservice.JWTService,
	tokenStore *redis.TokenStore,
	log logger.Logger,
) *RefreshTokenUseCase {
	return &RefreshTokenUseCase{
		userRepo:   userRepo,
		jwtService: jwtService,
		tokenStore: tokenStore,
		log:        log.With("usecase", "refresh_token"),
	}
}

// Execute validates the refresh token and issues a rotated session pair.
func (uc *RefreshTokenUseCase) Execute(ctx context.Context, req RefreshTokenRequest) (*RefreshTokenResponse, error) {
	if err := application.ValidateRequired("refresh_token", req.RefreshToken); err != nil {
		return nil, err
	}

	claims, err := uc.jwtService.ParseRefreshToken(req.RefreshToken)
	if err != nil {
		uc.log.Warn("refresh rejected", "reason", "invalid_token", "error", err)
		return nil, application.ErrInvalidToken
	}

	// Per-session logout check. Redis failure = fail-open (AGENTS.md degradation
	// matrix): a revoked-but-unexpired token slipping through during a Redis
	// outage is preferred over blocking every session refresh.
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

	// token_version guard: logout-all / reset-password bumps the version and
	// instantly invalidates every previously issued refresh token.
	if claims.TokenVersion != u.TokenVersion {
		uc.log.Warn("refresh rejected", "reason", "token_version_mismatch", "user_id", u.ID)
		return nil, application.ErrTokenVersionMismatch
	}

	pair, err := IssueTokenPair(uc.jwtService, u.ID, u.TokenVersion)
	if err != nil {
		uc.log.Error("refresh failed", "step", "issue_tokens", "user_id", u.ID, "error", err)
		return nil, fmt.Errorf("refresh: %w", err)
	}

	// Rotation: revoke the presented token for its remaining lifetime.
	if claims.ExpiresAt != nil {
		if err := uc.tokenStore.DenylistRefreshJTI(ctx, claims.ID, time.Until(claims.ExpiresAt.Time)); err != nil {
			uc.log.Warn("failed to denylist rotated refresh token", "user_id", u.ID, "error", err)
		}
	}

	uc.log.Info("session refreshed", "user_id", u.ID)
	return &RefreshTokenResponse{AccessToken: pair.AccessToken, RefreshToken: pair.RefreshToken}, nil
}

// ---------------------------------------------------------------------------
// Logout (this session) & Logout-all (every session)
// ---------------------------------------------------------------------------

// LogoutRequest carries the refresh token of the session being terminated.
type LogoutRequest struct {
	UserID       string // from the access token (auth middleware)
	RefreshToken string
}

// LogoutUseCase terminates ONE session: the presented refresh token is
// denylisted until its natural expiry, so it can never mint new access tokens.
// The current access token is short-lived (≤15 min) and dies on its own —
// per-request denylist checks for access tokens are deliberately skipped (KISS,
// consistent with the PRD's decision to avoid full token-family revocation).
type LogoutUseCase struct {
	jwtService *jwtservice.JWTService
	tokenStore *redis.TokenStore
	log        logger.Logger
}

// NewLogoutUseCase constructs a new LogoutUseCase.
func NewLogoutUseCase(jwtService *jwtservice.JWTService, tokenStore *redis.TokenStore, log logger.Logger) *LogoutUseCase {
	return &LogoutUseCase{
		jwtService: jwtService,
		tokenStore: tokenStore,
		log:        log.With("usecase", "logout"),
	}
}

// Execute revokes the presented refresh token. Logout is idempotent: an
// already-invalid/expired refresh token is treated as success — there is
// nothing left to revoke, which is exactly the state the caller wants.
func (uc *LogoutUseCase) Execute(ctx context.Context, req LogoutRequest) error {
	if err := application.ValidateRequired("refresh_token", req.RefreshToken); err != nil {
		return err
	}

	claims, err := uc.jwtService.ParseRefreshToken(req.RefreshToken)
	if err != nil {
		uc.log.Info("logout no-op", "reason", "token_already_invalid", "user_id", req.UserID)
		return nil
	}

	// Refuse to revoke somebody else's token — the refresh token must belong
	// to the authenticated caller.
	if claims.Subject != req.UserID {
		uc.log.Warn("logout rejected", "reason", "subject_mismatch", "user_id", req.UserID)
		return application.ErrInvalidToken
	}

	if claims.ExpiresAt != nil {
		if err := uc.tokenStore.DenylistRefreshJTI(ctx, claims.ID, time.Until(claims.ExpiresAt.Time)); err != nil {
			uc.log.Warn("failed to denylist refresh token on logout", "user_id", req.UserID, "error", err)
			// Fail-open: worst case the refresh token stays valid until expiry.
		}
	}

	uc.log.Info("user logged out", "user_id", req.UserID)
	return nil
}

// LogoutAllUseCase terminates EVERY session of a user by incrementing
// USER.token_version (FR-H8). All previously issued access AND refresh tokens
// embed the old version and are rejected on their next use — no per-token
// tracking needed.
type LogoutAllUseCase struct {
	userRepo user.Repository
	log      logger.Logger
}

// NewLogoutAllUseCase constructs a new LogoutAllUseCase.
func NewLogoutAllUseCase(userRepo user.Repository, log logger.Logger) *LogoutAllUseCase {
	return &LogoutAllUseCase{
		userRepo: userRepo,
		log:      log.With("usecase", "logout_all"),
	}
}

// Execute bumps token_version, revoking all outstanding sessions at once.
func (uc *LogoutAllUseCase) Execute(ctx context.Context, userID string) error {
	if err := uc.userRepo.IncrementTokenVersion(ctx, userID); err != nil {
		uc.log.Error("logout-all failed", "step", "increment_token_version", "user_id", userID, "error", err)
		return fmt.Errorf("logout_all: increment token version: %w", err)
	}
	uc.log.Info("all sessions revoked", "user_id", userID)
	return nil
}
