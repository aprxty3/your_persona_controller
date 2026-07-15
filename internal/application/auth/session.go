package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/aprxty3/your_persona_controller.git/internal/application"
	"github.com/aprxty3/your_persona_controller.git/internal/domain/account"
	"github.com/aprxty3/your_persona_controller.git/internal/infrastructure/cache/redis"
	jwtservice "github.com/aprxty3/your_persona_controller.git/internal/infrastructure/jwt"
	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// Session token TTLs shared by every flow that issues a session
const (
	AccessTokenTTL  = 15 * time.Minute
	RefreshTokenTTL = 14 * 24 * time.Hour
)

const (
	defaultLoginMaxAttempts = 10
	defaultLockDuration     = 15 * time.Minute
)

// ResetTokenTTL is the validity window of the short-lived reset_token JWT
const ResetTokenTTL = 15 * time.Minute

// SessionTokenStore is the narrow slice of Redis-backed token bookkeeping
// SessionUseCase needs — scoped smaller than the full *redis.TokenStore, and
// declared as an interface (rather than the concrete struct) so unit tests
// can fake the single-use reset_token / refresh-token-denylist contracts
// without a real Redis connection.
type SessionTokenStore interface {
	StoreResetJTI(ctx context.Context, jti, userID string, ttl time.Duration) error
	ConsumeResetJTI(ctx context.Context, jti string) (userID string, err error)
	DenylistRefreshJTI(ctx context.Context, jti string, ttl time.Duration) error
	IsRefreshJTIDenylisted(ctx context.Context, jti string) (bool, error)
}

// TokenPair carries one full JWT session credential set.
type TokenPair struct {
	AccessToken  string
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

// LoginRequest is the payload for the authenticate user endpoint.
type LoginRequest struct {
	Email     string
	Password  string
	IPAddress string // raw client IP, used for per-IP rate limiting only (not persisted)
}

// LoginResponse carries JWT credentials on successful login.
type LoginResponse struct {
	AccessToken       string `json:"access_token"`
	RefreshToken      string `json:"refresh_token"`
	RetryAfterSeconds int    `json:"-"` // set only when login itself returned ErrRateLimited
}

// VerifyEmailOTPRequest represents payload structure for OTP validation.
type VerifyEmailOTPRequest struct {
	Email string
	OTP   string
}

// VerifyEmailOTPResponse carries a fresh session pair (auto-login) on success,
// or remaining attempt statistics on failure.
type VerifyEmailOTPResponse struct {
	AccessToken       string `json:"access_token"`
	RefreshToken      string `json:"refresh_token"`
	AttemptsRemaining int    `json:"-"`
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
	UserID       string // from the access token (auth middleware)
	RefreshToken string
}

// VerifyResetOTPRequest carries the reset OTP to be exchanged.
type VerifyResetOTPRequest struct {
	Email string
	OTP   string
}

// VerifyResetOTPResponse returns the short-lived single-use reset_token.
type VerifyResetOTPResponse struct {
	ResetToken        string `json:"reset_token"`
	AttemptsRemaining int    `json:"-"`
}

// ResetPasswordRequest carries the single-use reset_token and the new password.
type ResetPasswordRequest struct {
	ResetToken  string
	NewPassword string
}

// ResetPasswordResponse auto-logs the user in after a successful reset.
type ResetPasswordResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

// ChangePasswordRequest carries the credentials for an authenticated password change.
type ChangePasswordRequest struct {
	UserID           string // from the access token (auth middleware)
	OldPassword      string
	NewPassword      string
	RetryNewPassword string
}

// ChangePasswordResponse re-issues a session pair for the CURRENT device
type ChangePasswordResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

// SessionUseCase manages authentication sessions
type SessionUseCase struct {
	db               *gorm.DB
	userRepo         account.UserRepository
	tokenRepo        account.VerificationTokenRepository
	breachChecker    PasswordBreachChecker
	jwtService       *jwtservice.JWTService
	tokenStore       SessionTokenStore
	ipRateLimiter    IPRateLimiter
	log              logger.Logger
	loginMaxAttempts int
	lockDuration     time.Duration
}

// NewSessionUseCase creates a new SessionUseCase.
func NewSessionUseCase(
	db *gorm.DB,
	userRepo account.UserRepository,
	tokenRepo account.VerificationTokenRepository,
	breachChecker PasswordBreachChecker,
	jwtService *jwtservice.JWTService,
	tokenStore SessionTokenStore,
	ipRateLimiter IPRateLimiter,
	log logger.Logger,
) *SessionUseCase {
	return &SessionUseCase{
		db:               db,
		userRepo:         userRepo,
		tokenRepo:        tokenRepo,
		breachChecker:    breachChecker,
		jwtService:       jwtService,
		tokenStore:       tokenStore,
		ipRateLimiter:    ipRateLimiter,
		log:              log.With("usecase", "session"),
		loginMaxAttempts: defaultLoginMaxAttempts,
		lockDuration:     defaultLockDuration,
	}
}

// Login authenticates a user and increments or resets login lockout policies.
func (uc *SessionUseCase) Login(ctx context.Context, req LoginRequest) (*LoginResponse, error) {
	allowed, retryAfter, err := uc.ipRateLimiter.Allow(ctx, redis.ScopeLoginIP, req.IPAddress)
	if err != nil {
		uc.log.Warn("login ip rate limit check skipped", "reason", "redis_error", "error", err)
	} else if !allowed {
		uc.log.Warn("login rejected", "reason", "rate_limited", "retry_after_seconds", retryAfter)
		return &LoginResponse{RetryAfterSeconds: retryAfter}, application.ErrRateLimited
	}

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

func (uc *SessionUseCase) handleFailedAttempt(ctx context.Context, u *account.User) error {
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

// VerifyEmailOTP validates an email verification code, activates the account, and auto-logs the user in
func (uc *SessionUseCase) VerifyEmailOTP(ctx context.Context, req VerifyEmailOTPRequest) (*VerifyEmailOTPResponse, error) {
	u, err := uc.userRepo.FindByEmail(ctx, req.Email)
	if err != nil {
		uc.log.Error("verify email otp failed", "step", "lookup_user", "error", err)
		return nil, fmt.Errorf("verify_email_otp: lookup user: %w", err)
	}
	if u == nil {
		uc.log.Warn("verify email otp rejected", "reason", "user_not_found")
		return nil, application.ErrInvalidOTP
	}

	token, remaining, err := validateOTPAttempt(ctx, uc.tokenRepo, u.ID, req.OTP, account.TokenTypeEmailVerification, uc.log)
	if err != nil {
		return &VerifyEmailOTPResponse{AttemptsRemaining: remaining}, err
	}

	if err := uc.tokenRepo.MarkUsed(ctx, token.ID); err != nil {
		uc.log.Error("verify email otp failed", "step", "mark_token_used", "user_id", u.ID, "error", err)
		return nil, fmt.Errorf("verify_email_otp: mark token used: %w", err)
	}

	now := time.Now()
	u.EmailVerifiedAt = &now
	if err := uc.userRepo.Update(ctx, u); err != nil {
		uc.log.Error("verify email otp failed", "step", "update_user", "user_id", u.ID, "error", err)
		return nil, fmt.Errorf("verify_email_otp: update user: %w", err)
	}

	pair, err := IssueTokenPair(uc.jwtService, u.ID, u.TokenVersion)
	if err != nil {
		uc.log.Error("verify email otp failed", "step", "issue_tokens", "user_id", u.ID, "error", err)
		return nil, fmt.Errorf("verify_email_otp: %w", err)
	}

	uc.log.Info("email verified", "user_id", u.ID)
	return &VerifyEmailOTPResponse{
		AccessToken:       pair.AccessToken,
		RefreshToken:      pair.RefreshToken,
		AttemptsRemaining: application.MaxWrongOTPAttempts,
	}, nil
}

// RefreshToken exchanges a valid refresh token for a brand-new session pair.
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

// Logout terminates ONE session: the presented refresh token is denylisted until its natural expiry
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

// LogoutAll terminates EVERY session of a user by incrementing USER.token_ver
func (uc *SessionUseCase) LogoutAll(ctx context.Context, userID string) error {
	if err := uc.userRepo.IncrementTokenVersion(ctx, userID); err != nil {
		uc.log.Error("logout-all failed", "step", "increment_token_version", "user_id", userID, "error", err)
		return fmt.Errorf("logout_all: increment token version: %w", err)
	}
	uc.log.Info("all sessions revoked", "user_id", userID)
	return nil
}

// VerifyResetOTP exchanges a valid reset OTP for a reset_token JWT.
func (uc *SessionUseCase) VerifyResetOTP(ctx context.Context, req VerifyResetOTPRequest) (*VerifyResetOTPResponse, error) {
	if err := application.ValidateEmail("email", req.Email); err != nil {
		return nil, err
	}

	if err := application.ValidateRequired("otp", req.OTP); err != nil {
		return nil, err
	}

	u, err := uc.userRepo.FindByEmail(ctx, req.Email)

	if err != nil {
		uc.log.Error("verify reset otp failed", "step", "lookup_user", "error", err)
		return nil, fmt.Errorf("verify_reset_otp: lookup user: %w", err)
	}

	if u == nil {
		uc.log.Warn("verify reset otp rejected", "reason", "user_not_found")
		return nil, application.ErrInvalidOTP
	}

	token, remaining, err := validateOTPAttempt(ctx, uc.tokenRepo, u.ID, req.OTP, account.TokenTypePasswordReset, uc.log)
	if err != nil {
		return &VerifyResetOTPResponse{AttemptsRemaining: remaining}, err
	}

	if err := uc.tokenRepo.MarkUsed(ctx, token.ID); err != nil {
		uc.log.Error("verify reset otp failed", "step", "mark_token_used", "user_id", u.ID, "error", err)
		return nil, fmt.Errorf("verify_reset_otp: mark token used: %w", err)
	}

	jti, resetToken, err := uc.jwtService.GenerateResetToken(u.ID, ResetTokenTTL)
	if err != nil {
		uc.log.Error("verify reset otp failed", "step", "issue_reset_token", "user_id", u.ID, "error", err)
		return nil, fmt.Errorf("verify_reset_otp: issue reset token: %w", err)
	}

	if err := uc.tokenStore.StoreResetJTI(ctx, jti, u.ID, ResetTokenTTL); err != nil {
		uc.log.Error("verify reset otp failed", "step", "store_reset_jti", "user_id", u.ID, "error", err)
		return nil, fmt.Errorf("verify_reset_otp: store reset jti: %w", err)
	}

	uc.log.Info("reset otp verified", "user_id", u.ID)
	return &VerifyResetOTPResponse{ResetToken: resetToken, AttemptsRemaining: application.MaxWrongOTPAttempts}, nil
}

// ResetPassword consumes a reset_token (atomically, single-use)
func (uc *SessionUseCase) ResetPassword(ctx context.Context, req ResetPasswordRequest) (*ResetPasswordResponse, error) {
	if err := application.ValidateRequired("reset_token", req.ResetToken); err != nil {
		return nil, err
	}
	if err := ValidateNewPassword(ctx, uc.breachChecker, "new_password", req.NewPassword); err != nil {
		uc.log.Warn("reset password rejected", "reason", "password_policy", "error", err)
		return nil, err
	}

	claims, err := uc.jwtService.ParseResetToken(req.ResetToken)
	if err != nil {
		uc.log.Warn("reset password rejected", "reason", "invalid_reset_token", "error", err)
		return nil, application.ErrInvalidToken
	}

	consumedUserID, err := uc.tokenStore.ConsumeResetJTI(ctx, claims.ID)
	if err != nil {
		uc.log.Error("reset password failed", "step", "consume_reset_jti", "error", err)
		return nil, fmt.Errorf("reset_password: consume reset jti: %w", err)
	}
	if consumedUserID == "" || consumedUserID != claims.Subject {
		uc.log.Warn("reset password rejected", "reason", "reset_token_consumed_or_mismatched")
		return nil, application.ErrInvalidToken
	}

	u, err := uc.userRepo.FindByID(ctx, claims.Subject)
	if err != nil {
		uc.log.Error("reset password failed", "step", "lookup_user", "error", err)
		return nil, fmt.Errorf("reset_password: lookup user: %w", err)
	}
	if u == nil {
		uc.log.Warn("reset password rejected", "reason", "user_not_found")
		return nil, application.ErrInvalidToken
	}

	hash, err := HashPassword(req.NewPassword)
	if err != nil {
		uc.log.Error("reset password failed", "step", "hash_password", "user_id", u.ID, "error", err)
		return nil, fmt.Errorf("reset_password: %w", err)
	}

	err = uc.db.Transaction(func(tx *gorm.DB) error {
		txUserRepo := txUserRepository(tx, uc.log)

		u.PasswordHash = hash
		if err := txUserRepo.Update(ctx, u); err != nil {
			return fmt.Errorf("update password: %w", err)
		}

		if err := txUserRepo.IncrementTokenVersion(ctx, u.ID); err != nil {
			return fmt.Errorf("increment token version: %w", err)
		}
		if err := txUserRepo.UpdateLoginAttempt(ctx, u.ID, 0, nil); err != nil {
			return fmt.Errorf("clear login lockout: %w", err)
		}
		return nil
	})
	if err != nil {
		uc.log.Error("reset password failed", "step", "transaction", "user_id", u.ID, "error", err)
		return nil, fmt.Errorf("reset_password: %w", err)
	}

	pair, err := IssueTokenPair(uc.jwtService, u.ID, u.TokenVersion+1)
	if err != nil {
		uc.log.Error("reset password failed", "step", "issue_tokens", "user_id", u.ID, "error", err)
		return nil, fmt.Errorf("reset_password: %w", err)
	}

	uc.log.Info("password reset completed", "user_id", u.ID)
	return &ResetPasswordResponse{AccessToken: pair.AccessToken, RefreshToken: pair.RefreshToken}, nil
}

// ChangePassword lets an already-authenticated user change their password
func (uc *SessionUseCase) ChangePassword(ctx context.Context, req ChangePasswordRequest) (*ChangePasswordResponse, error) {
	if err := application.ValidateRequired("old_password", req.OldPassword); err != nil {
		return nil, err
	}
	if req.NewPassword != req.RetryNewPassword {
		uc.log.Warn("change password rejected", "reason", "confirmation_mismatch", "user_id", req.UserID)
		return nil, application.ErrPasswordConfirmationMismatch
	}
	if err := ValidateNewPassword(ctx, uc.breachChecker, "new_password", req.NewPassword); err != nil {
		uc.log.Warn("change password rejected", "reason", "password_policy", "error", err)
		return nil, err
	}

	u, err := uc.userRepo.FindByID(ctx, req.UserID)
	if err != nil {
		uc.log.Error("change password failed", "step", "lookup_user", "error", err)
		return nil, fmt.Errorf("change_password: lookup user: %w", err)
	}
	if u == nil {
		uc.log.Warn("change password rejected", "reason", "user_not_found")
		return nil, application.ErrInvalidCredentials
	}

	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(req.OldPassword)); err != nil {
		uc.log.Warn("change password rejected", "reason", "old_password_mismatch", "user_id", u.ID)
		return nil, application.ErrInvalidCredentials
	}

	hash, err := HashPassword(req.NewPassword)
	if err != nil {
		uc.log.Error("change password failed", "step", "hash_password", "user_id", u.ID, "error", err)
		return nil, fmt.Errorf("change_password: %w", err)
	}

	err = uc.db.Transaction(func(tx *gorm.DB) error {
		txUserRepo := txUserRepository(tx, uc.log)

		u.PasswordHash = hash
		if err := txUserRepo.Update(ctx, u); err != nil {
			return fmt.Errorf("update password: %w", err)
		}
		if err := txUserRepo.IncrementTokenVersion(ctx, u.ID); err != nil {
			return fmt.Errorf("increment token version: %w", err)
		}
		return nil
	})
	if err != nil {
		uc.log.Error("change password failed", "step", "transaction", "user_id", u.ID, "error", err)
		return nil, fmt.Errorf("change_password: %w", err)
	}

	pair, err := IssueTokenPair(uc.jwtService, u.ID, u.TokenVersion+1)
	if err != nil {
		uc.log.Error("change password failed", "step", "issue_tokens", "user_id", u.ID, "error", err)
		return nil, fmt.Errorf("change_password: %w", err)
	}

	uc.log.Info("password changed", "user_id", u.ID)
	return &ChangePasswordResponse{AccessToken: pair.AccessToken, RefreshToken: pair.RefreshToken}, nil
}
