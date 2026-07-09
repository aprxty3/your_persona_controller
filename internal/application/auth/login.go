package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/aprxty3/your_persona_controller.git/internal/application"
	"github.com/aprxty3/your_persona_controller.git/internal/domain/user"
	jwtservice "github.com/aprxty3/your_persona_controller.git/internal/infrastructure/jwt"
	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
	"golang.org/x/crypto/bcrypt"
)

const (
	defaultLoginMaxAttempts = 10
	defaultLockDuration     = 15 * time.Minute
	defaultAccessTokenTTL   = 15 * time.Minute
	defaultRefreshTokenTTL  = 14 * 24 * time.Hour
)

// LoginRequest is the payload for the authenticate user endpoint.
type LoginRequest struct {
	Email    string
	Password string
}

// LoginResponse carries JWT credentials on successful login.
type LoginResponse struct {
	AccessToken  string
	RefreshToken string
}

// LoginUseCase validates password hashes and generates session JWTs.
type LoginUseCase struct {
	userRepo         user.Repository
	jwtService       *jwtservice.JWTService
	log              logger.Logger
	loginMaxAttempts int
	lockDuration     time.Duration
	accessTokenTTL   time.Duration
	refreshTokenTTL  time.Duration
}

// NewLoginUseCase creates a new LoginUseCase with configurable defaults.
func NewLoginUseCase(userRepo user.Repository, jwtService *jwtservice.JWTService, log logger.Logger) *LoginUseCase {
	return &LoginUseCase{
		userRepo:         userRepo,
		jwtService:       jwtService,
		log:              log.With("usecase", "login"),
		loginMaxAttempts: defaultLoginMaxAttempts,
		lockDuration:     defaultLockDuration,
		accessTokenTTL:   defaultAccessTokenTTL,
		refreshTokenTTL:  defaultRefreshTokenTTL,
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

	// Issue JWT token pairs
	accessToken, err := uc.jwtService.GenerateAccessToken(u.ID, u.TokenVersion, uc.accessTokenTTL)
	if err != nil {
		uc.log.Error("login failed", "step", "issue_access_token", "user_id", u.ID, "error", err)
		return nil, fmt.Errorf("login: issue access token: %w", err)
	}

	refreshToken, err := uc.jwtService.GenerateRefreshToken(u.ID, uc.refreshTokenTTL)
	if err != nil {
		uc.log.Error("login failed", "step", "issue_refresh_token", "user_id", u.ID, "error", err)
		return nil, fmt.Errorf("login: issue refresh token: %w", err)
	}

	uc.log.Info("user logged in", "user_id", u.ID)
	return &LoginResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
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
