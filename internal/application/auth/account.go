// Package auth implements the auth domain's use cases: registration, login,
// session/token lifecycle, OTP verification, and password reset.
package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/aprxty3/your_persona_controller.git/internal/application"
	"github.com/aprxty3/your_persona_controller.git/internal/domain/account"
	"github.com/aprxty3/your_persona_controller.git/internal/infrastructure/cache/redis"
	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
	"github.com/aprxty3/your_persona_controller.git/pkg/otp"
	"github.com/aprxty3/your_persona_controller.git/pkg/taskqueue"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// PasswordBreachChecker defines the contract for HIBP checks.
type PasswordBreachChecker interface {
	IsBreached(ctx context.Context, password string) (bool, error)
}

// NoopBreachChecker always reports passwords as NOT breached.
type NoopBreachChecker struct{}

// NewNoopBreachChecker creates a new NoopBreachChecker.
func NewNoopBreachChecker() PasswordBreachChecker {
	return &NoopBreachChecker{}
}

// IsBreached mocks the HIBP check by always returning false.
func (c *NoopBreachChecker) IsBreached(_ context.Context, _ string) (bool, error) {
	return false, nil
}

// TurnstileVerifier defines the contract for Cloudflare Turnstile bot checks.
type TurnstileVerifier interface {
	Verify(ctx context.Context, token, remoteIP string) (bool, error)
}

// ValidateNewPassword enforces the single shared password policy
func ValidateNewPassword(ctx context.Context, checker PasswordBreachChecker, fieldName, password string) error {
	if err := application.ValidateRequired(fieldName, password); err != nil {
		return err
	}
	if err := application.ValidateMinLength(fieldName, password, application.PasswordMinLength); err != nil {
		return application.ErrPasswordTooShort
	}
	if breached, err := checker.IsBreached(ctx, password); err == nil && breached {
		return application.ErrPasswordBreached
	}
	return nil
}

// HashPassword produces the bcrypt hash used everywhere a password is persisted.
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("password: bcrypt hash: %w", err)
	}
	return string(hash), nil
}

// validateOTPAttempt is the single shared OTP validation gate
func validateOTPAttempt(
	ctx context.Context,
	tokenRepo account.VerificationTokenRepository,
	userID string,
	code string,
	tokenType account.TokenType,
	log logger.Logger,
) (token *account.VerificationToken, attemptsRemaining int, err error) {
	token, err = tokenRepo.FindActiveByUserAndType(ctx, userID, tokenType)
	if err != nil {
		log.Error("otp validation failed", "step", "find_token", "user_id", userID, "error", err)
		return nil, 0, fmt.Errorf("otp: find token: %w", err)
	}
	if token == nil {
		log.Warn("otp rejected", "reason", "no_active_token", "user_id", userID)
		return nil, 0, application.ErrOTPExpired
	}

	if token.AttemptCount >= application.MaxWrongOTPAttempts {
		log.Warn("otp rejected", "reason", "max_attempts", "user_id", userID)
		return nil, 0, application.ErrOTPMaxAttempts
	}

	if time.Now().After(token.ExpiresAt) {
		log.Warn("otp rejected", "reason", "expired", "user_id", userID)
		return nil, 0, application.ErrOTPExpired
	}

	if token.Token != code {
		if err := tokenRepo.IncrementAttemptCount(ctx, token.ID); err != nil {
			log.Error("otp validation failed", "step", "increment_attempts", "user_id", userID, "error", err)
			return nil, 0, fmt.Errorf("otp: increment token attempts: %w", err)
		}
		remaining := application.MaxWrongOTPAttempts - (token.AttemptCount + 1)
		log.Warn("otp rejected", "reason", "invalid_otp", "user_id", userID, "attempts_remaining", remaining)
		if remaining <= 0 {
			return nil, 0, application.ErrOTPMaxAttempts
		}
		return nil, remaining, application.ErrInvalidOTP
	}

	return token, application.MaxWrongOTPAttempts, nil
}

// ResendEmailOTPRequest specifies the target user's email.
type ResendEmailOTPRequest struct {
	Email string
}

// ResendEmailOTPResponse carries the rate limit cooling period metadata.
type ResendEmailOTPResponse struct {
	RetryAfterSeconds int
}

// ForgotPasswordRequest specifies the account email requesting a reset.
type ForgotPasswordRequest struct {
	Email string
}

// ForgotPasswordResponse carries rate limit metadata when throttled.
type ForgotPasswordResponse struct {
	RetryAfterSeconds int
}

// AccountUseCase manages the OTP lifecycle for an existing account.
type AccountUseCase struct {
	userRepo    account.UserRepository
	tokenRepo   account.VerificationTokenRepository
	dispatcher  taskqueue.Dispatcher
	rateLimiter OTPRateLimiter
	log         logger.Logger
}

// NewAccountUseCase creates a new AccountUseCase.
func NewAccountUseCase(
	userRepo account.UserRepository,
	tokenRepo account.VerificationTokenRepository,
	dispatcher taskqueue.Dispatcher,
	rateLimiter OTPRateLimiter,
	log logger.Logger,
) *AccountUseCase {
	return &AccountUseCase{
		userRepo:    userRepo,
		tokenRepo:   tokenRepo,
		dispatcher:  dispatcher,
		rateLimiter: rateLimiter,
		log:         log.With("usecase", "account"),
	}
}

// ResendEmailOTP invalidates existing codes and triggers a secure new OTP delivery.
func (uc *AccountUseCase) ResendEmailOTP(ctx context.Context, req ResendEmailOTPRequest) (*ResendEmailOTPResponse, error) {
	if err := application.ValidateEmail("email", req.Email); err != nil {
		return nil, err
	}

	retryAfter, err := uc.rateLimiter.CheckAndConsume(ctx, redis.ScopeEmailVerification, req.Email)
	if err != nil {
		uc.log.Error("resend otp failed", "step", "rate_limit_evaluation", "error", err)
		return nil, fmt.Errorf("resend_otp: rate limit evaluation: %w", err)
	}

	if retryAfter > 0 {
		uc.log.Warn("resend otp rejected", "reason", "rate_limited", "retry_after_seconds", retryAfter)
		return &ResendEmailOTPResponse{RetryAfterSeconds: retryAfter}, application.ErrRateLimited
	}

	u, err := uc.userRepo.FindByEmail(ctx, req.Email)
	if err != nil {
		uc.log.Error("resend otp failed", "step", "lookup_user", "error", err)
		return nil, fmt.Errorf("resend_otp: lookup user: %w", err)
	}
	if u == nil {
		uc.log.Warn("resend otp no-op", "reason", "user_not_found")
		return &ResendEmailOTPResponse{}, nil
	}

	if err := uc.tokenRepo.ExpireAllActiveForUser(ctx, u.ID, account.TokenTypeEmailVerification); err != nil {
		uc.log.Error("resend otp failed", "step", "invalidate_previous_tokens", "user_id", u.ID, "error", err)
		return nil, fmt.Errorf("resend_otp: invalidate previous tokens: %w", err)
	}

	otpCode, err := otp.GenerateOTP(application.OTPLength)
	if err != nil {
		uc.log.Error("resend otp failed", "step", "generate_code", "user_id", u.ID, "error", err)
		return nil, fmt.Errorf("resend_otp: generate code: %w", err)
	}

	token := &account.VerificationToken{
		ID:        uuid.New().String(),
		UserID:    u.ID,
		Token:     otpCode,
		Type:      account.TokenTypeEmailVerification,
		ExpiresAt: time.Now().Add(application.OTPExpiry),
	}
	if err := uc.tokenRepo.Create(ctx, token); err != nil {
		uc.log.Error("resend otp failed", "step", "persist_token", "user_id", u.ID, "error", err)
		return nil, fmt.Errorf("resend_otp: persist token: %w", err)
	}

	if err := uc.rateLimiter.SetCooldown(ctx, redis.ScopeEmailVerification, req.Email); err != nil {
		uc.log.Warn("failed to set otp cooldown", "user_id", u.ID, "error", err)
	}

	payload := taskqueue.SendEmailPayload{
		Type:   "otp_verification",
		UserID: u.ID,
		Email:  u.Email,
		OTP:    otpCode,
		Locale: u.PreferredLocale,
	}
	if err := uc.dispatcher.EnqueueEmail(ctx, payload, taskqueue.QueueCritical); err != nil {
		uc.log.Warn("failed to enqueue resend otp email", "user_id", u.ID, "error", err)
	}

	uc.log.Info("otp resent", "user_id", u.ID)
	return &ResendEmailOTPResponse{}, nil
}

// ForgotPassword issues a password-reset OTP.
func (uc *AccountUseCase) ForgotPassword(ctx context.Context, req ForgotPasswordRequest) (*ForgotPasswordResponse, error) {
	if err := application.ValidateEmail("email", req.Email); err != nil {
		return nil, err
	}

	retryAfter, err := uc.rateLimiter.CheckAndConsume(ctx, redis.ScopePasswordReset, req.Email)
	if err != nil {
		uc.log.Error("forgot password failed", "step", "rate_limit_evaluation", "error", err)
		return nil, fmt.Errorf("forgot_password: rate limit evaluation: %w", err)
	}
	if retryAfter > 0 {
		uc.log.Warn("forgot password rejected", "reason", "rate_limited", "retry_after_seconds", retryAfter)
		return &ForgotPasswordResponse{RetryAfterSeconds: retryAfter}, application.ErrRateLimited
	}

	u, err := uc.userRepo.FindByEmail(ctx, req.Email)
	if err != nil {
		uc.log.Error("forgot password failed", "step", "lookup_user", "error", err)
		return nil, fmt.Errorf("forgot_password: lookup user: %w", err)
	}
	if u == nil {
		uc.log.Info("forgot password no-op", "reason", "user_not_found")
		return &ForgotPasswordResponse{}, nil
	}
	if err := uc.tokenRepo.ExpireAllActiveForUser(ctx, u.ID, account.TokenTypePasswordReset); err != nil {
		uc.log.Error("forgot password failed", "step", "invalidate_previous_tokens", "user_id", u.ID, "error", err)
		return nil, fmt.Errorf("forgot_password: invalidate previous tokens: %w", err)
	}

	otpCode, err := otp.GenerateOTP(application.OTPLength)
	if err != nil {
		uc.log.Error("forgot password failed", "step", "generate_code", "user_id", u.ID, "error", err)
		return nil, fmt.Errorf("forgot_password: generate code: %w", err)
	}

	token := &account.VerificationToken{
		ID:        uuid.New().String(),
		UserID:    u.ID,
		Token:     otpCode,
		Type:      account.TokenTypePasswordReset,
		ExpiresAt: time.Now().Add(application.OTPExpiry),
	}
	if err := uc.tokenRepo.Create(ctx, token); err != nil {
		uc.log.Error("forgot password failed", "step", "persist_token", "user_id", u.ID, "error", err)
		return nil, fmt.Errorf("forgot_password: persist token: %w", err)
	}

	if err := uc.rateLimiter.SetCooldown(ctx, redis.ScopePasswordReset, req.Email); err != nil {
		uc.log.Warn("failed to set reset otp cooldown", "user_id", u.ID, "error", err)
	}

	payload := taskqueue.SendEmailPayload{
		Type:   "otp_reset",
		UserID: u.ID,
		Email:  u.Email,
		OTP:    otpCode,
		Locale: u.PreferredLocale,
	}
	if err := uc.dispatcher.EnqueueEmail(ctx, payload, taskqueue.QueueCritical); err != nil {
		uc.log.Warn("failed to enqueue reset otp email", "user_id", u.ID, "error", err)
	}

	uc.log.Info("password reset otp sent", "user_id", u.ID)
	return &ForgotPasswordResponse{}, nil
}
