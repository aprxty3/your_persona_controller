package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/aprxty3/your_persona_controller.git/internal/application"
	"github.com/aprxty3/your_persona_controller.git/internal/domain/user"
	"github.com/aprxty3/your_persona_controller.git/internal/domain/verificationtoken"
	"github.com/aprxty3/your_persona_controller.git/internal/infrastructure/cache/redis"
	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
	"github.com/aprxty3/your_persona_controller.git/pkg/otp"
	"github.com/aprxty3/your_persona_controller.git/pkg/taskqueue"
	"github.com/google/uuid"
)

// ForgotPasswordRequest specifies the account email requesting a reset (FR-H4 step 1/3).
type ForgotPasswordRequest struct {
	Email string
}

// ForgotPasswordResponse carries rate limit metadata when throttled.
type ForgotPasswordResponse struct {
	RetryAfterSeconds int
}

// ForgotPasswordUseCase issues a password-reset OTP. The HTTP response MUST be
// identical whether or not the email is registered (anti account-enumeration,
// AGENTS.md Security Rules) — an unknown email is a silent no-op, never an error.
type ForgotPasswordUseCase struct {
	userRepo      user.Repository
	tokenRepo     verificationtoken.Repository
	rateLimiter   *redis.OTPRateLimitService
	dispatcher    taskqueue.Dispatcher
	log           logger.Logger
	otpLength     int
	otpExpiryMins int
}

// NewForgotPasswordUseCase builds a new ForgotPasswordUseCase.
func NewForgotPasswordUseCase(
	userRepo user.Repository,
	tokenRepo verificationtoken.Repository,
	rateLimiter *redis.OTPRateLimitService,
	dispatcher taskqueue.Dispatcher,
	log logger.Logger,
) *ForgotPasswordUseCase {
	return &ForgotPasswordUseCase{
		userRepo:      userRepo,
		tokenRepo:     tokenRepo,
		rateLimiter:   rateLimiter,
		dispatcher:    dispatcher,
		log:           log.With("usecase", "forgot_password"),
		otpLength:     6,
		otpExpiryMins: 15,
	}
}

// Execute rate-limits, revokes previous reset OTPs, and dispatches a new one.
func (uc *ForgotPasswordUseCase) Execute(ctx context.Context, req ForgotPasswordRequest) (*ForgotPasswordResponse, error) {
	if err := application.ValidateRequired("email", req.Email); err != nil {
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
		// Silent no-op: response stays generic to block account enumeration.
		uc.log.Info("forgot password no-op", "reason", "user_not_found")
		return &ForgotPasswordResponse{}, nil
	}

	// Single-valid-token invariant: max 1 active reset OTP per user at any time.
	if err := uc.tokenRepo.ExpireAllActiveForUser(ctx, u.ID, verificationtoken.TokenTypePasswordReset); err != nil {
		uc.log.Error("forgot password failed", "step", "invalidate_previous_tokens", "user_id", u.ID, "error", err)
		return nil, fmt.Errorf("forgot_password: invalidate previous tokens: %w", err)
	}

	otpCode, err := otp.GenerateOTP(uc.otpLength)
	if err != nil {
		uc.log.Error("forgot password failed", "step", "generate_code", "user_id", u.ID, "error", err)
		return nil, fmt.Errorf("forgot_password: generate code: %w", err)
	}

	token := &verificationtoken.VerificationToken{
		ID:        uuid.New().String(),
		UserID:    u.ID,
		Token:     otpCode,
		Type:      verificationtoken.TokenTypePasswordReset,
		ExpiresAt: time.Now().Add(time.Duration(uc.otpExpiryMins) * time.Minute),
	}
	if err := uc.tokenRepo.Create(ctx, token); err != nil {
		uc.log.Error("forgot password failed", "step", "persist_token", "user_id", u.ID, "error", err)
		return nil, fmt.Errorf("forgot_password: persist token: %w", err)
	}

	if err := uc.rateLimiter.SetCooldown(ctx, redis.ScopePasswordReset, req.Email); err != nil {
		// Non-fatal: proceed to email delivery regardless
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
		// Non-fatal to client since token exists and retry is available
		uc.log.Warn("failed to enqueue reset otp email", "user_id", u.ID, "error", err)
	}

	uc.log.Info("password reset otp sent", "user_id", u.ID)
	return &ForgotPasswordResponse{}, nil
}
