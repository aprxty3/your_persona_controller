package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/aprxty3/your_persona_controller.git/internal/application"
	"github.com/aprxty3/your_persona_controller.git/internal/domain/verificationtoken"
	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
	"golang.org/x/crypto/bcrypt"
)

// ---------------------------------------------------------------------------
// Password policy (FR-H1a) — shared by register & reset-password
// ---------------------------------------------------------------------------

// PasswordMinLength is the NIST-aligned minimum password length (FR-H1a).
const PasswordMinLength = 10

// PasswordBreachChecker defines the contract for HIBP checks (FR-H1a).
type PasswordBreachChecker interface {
	IsBreached(ctx context.Context, password string) (bool, error)
}

// NoopBreachChecker always reports passwords as NOT breached.
// Suitable for development and test environments.
type NoopBreachChecker struct{}

// NewNoopBreachChecker creates a new NoopBreachChecker.
func NewNoopBreachChecker() PasswordBreachChecker {
	return &NoopBreachChecker{}
}

// IsBreached mocks the HIBP check by always returning false.
func (c *NoopBreachChecker) IsBreached(_ context.Context, _ string) (bool, error) {
	return false, nil
}

// ValidateNewPassword enforces the single shared password policy (FR-H1a) for
// every flow that accepts a new password (register, reset-password): required,
// minimum length, and HIBP breach check.
//
// The breach check fails open: if the HIBP backend errors, the password is
// accepted so signups/resets are never blocked by a third-party outage.
func ValidateNewPassword(ctx context.Context, checker PasswordBreachChecker, fieldName, password string) error {
	if err := application.ValidateRequired(fieldName, password); err != nil {
		return err
	}
	if err := application.ValidateMinLength(fieldName, password, PasswordMinLength); err != nil {
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

// ---------------------------------------------------------------------------
// OTP validation gate — shared by email verification & password reset
// ---------------------------------------------------------------------------

// MaxWrongOTPAttempts defines maximum allowed invalid input attempts before token expiry.
const MaxWrongOTPAttempts = 5

// validateOTPAttempt is the single shared OTP validation gate used by both
// email verification and password reset flows: it finds the active token for
// (userID, tokenType), enforces expiry and the MaxWrongOTPAttempts policy, and
// increments attempt_count on a wrong guess.
//
// On success it returns the matched token (NOT yet marked used — the caller
// decides when to consume it). attemptsRemaining is meaningful when err is
// ErrInvalidOTP or ErrOTPMaxAttempts.
func validateOTPAttempt(
	ctx context.Context,
	tokenRepo verificationtoken.Repository,
	userID string,
	code string,
	tokenType verificationtoken.TokenType,
	log logger.Logger,
) (token *verificationtoken.VerificationToken, attemptsRemaining int, err error) {
	token, err = tokenRepo.FindActiveByUserAndType(ctx, userID, tokenType)
	if err != nil {
		log.Error("otp validation failed", "step", "find_token", "user_id", userID, "error", err)
		return nil, 0, fmt.Errorf("otp: find token: %w", err)
	}
	if token == nil {
		log.Warn("otp rejected", "reason", "no_active_token", "user_id", userID)
		return nil, 0, application.ErrOTPExpired
	}

	if token.AttemptCount >= MaxWrongOTPAttempts {
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
		remaining := MaxWrongOTPAttempts - (token.AttemptCount + 1)
		log.Warn("otp rejected", "reason", "invalid_otp", "user_id", userID, "attempts_remaining", remaining)
		if remaining <= 0 {
			return nil, 0, application.ErrOTPMaxAttempts
		}
		return nil, remaining, application.ErrInvalidOTP
	}

	return token, MaxWrongOTPAttempts, nil
}
