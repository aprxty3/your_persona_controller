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

// Sentinel login errors mapped to specific HTTP status codes in the presentation layer.
var (
	ErrInvalidCredentials = errors.New("INVALID_CREDENTIALS") // Scoped to prevent user enumeration
	ErrAccountLocked      = errors.New("ACCOUNT_LOCKED")      // HTTP 423
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
	userRepo        user.Repository
	jwtService      *jwtservice.JWTService
	loginMaxAttempts int
	lockDuration     time.Duration
	accessTokenTTL   time.Duration
	refreshTokenTTL  time.Duration
}

// NewLoginUseCase creates a new LoginUseCase with configurable defaults.
func NewLoginUseCase(userRepo user.Repository, jwtService *jwtservice.JWTService) *LoginUseCase {
	return &LoginUseCase{
		userRepo:         userRepo,
		jwtService:       jwtService,
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
		return nil, fmt.Errorf("login: query email: %w", err)
	}
	if u == nil {
		return nil, ErrInvalidCredentials
	}

	// Fast-fail if the account is currently locked out
	if u.IsLocked() {
		return nil, ErrAccountLocked
	}

	// Verify password hash
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(req.Password)); err != nil {
		return nil, uc.handleFailedAttempt(ctx, u)
	}

	// Clear failed login count upon successful authentication
	if u.FailedLoginCount > 0 {
		if err := uc.userRepo.UpdateLoginAttempt(ctx, u.ID, 0, nil); err != nil {
			// Fail-open: log error but do not block successful authentication
			_ = err
		}
	}

	// Issue JWT token pairs
	accessToken, err := uc.jwtService.GenerateAccessToken(u.ID, u.TokenVersion, uc.accessTokenTTL)
	if err != nil {
		return nil, fmt.Errorf("login: issue access token: %w", err)
	}

	refreshToken, err := uc.jwtService.GenerateRefreshToken(u.ID, uc.refreshTokenTTL)
	if err != nil {
		return nil, fmt.Errorf("login: issue refresh token: %w", err)
	}

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
		// Log error but prioritize raising incorrect credential signal to client
		_ = err
	}

	if lockedUntil != nil {
		return ErrAccountLocked
	}
	return ErrInvalidCredentials
}
