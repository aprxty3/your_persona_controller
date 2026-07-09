package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/aprxty3/your_persona_controller.git/internal/application"
	"github.com/aprxty3/your_persona_controller.git/internal/domain/verificationtoken"
	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
)

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
