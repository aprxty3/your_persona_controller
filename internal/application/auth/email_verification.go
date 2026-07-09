package auth

// Email verification OTP flow: verify (activate account) + resend (new code).

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

// ---------------------------------------------------------------------------
// Verify Email OTP
// ---------------------------------------------------------------------------

// VerifyEmailOTPRequest represents payload structure for OTP validation.
type VerifyEmailOTPRequest struct {
	Email string
	OTP   string
}

// VerifyEmailOTPResponse carries remaining attempt statistics on failure.
type VerifyEmailOTPResponse struct {
	AttemptsRemaining int
}

// VerifyEmailOTPUseCase handles verification of user registration OTP codes.
type VerifyEmailOTPUseCase struct {
	userRepo  user.Repository
	tokenRepo verificationtoken.Repository
	log       logger.Logger
}

// NewVerifyEmailOTPUseCase constructs a new VerifyEmailOTPUseCase.
func NewVerifyEmailOTPUseCase(
	userRepo user.Repository,
	tokenRepo verificationtoken.Repository,
	log logger.Logger,
) *VerifyEmailOTPUseCase {
	return &VerifyEmailOTPUseCase{userRepo: userRepo, tokenRepo: tokenRepo, log: log.With("usecase", "verify_email_otp")}
}

// Execute validates an email verification code and activates the user account.
func (uc *VerifyEmailOTPUseCase) Execute(ctx context.Context, req VerifyEmailOTPRequest) (*VerifyEmailOTPResponse, error) {
	u, err := uc.userRepo.FindByEmail(ctx, req.Email)
	if err != nil {
		uc.log.Error("verify email otp failed", "step", "lookup_user", "error", err)
		return nil, fmt.Errorf("verify_email_otp: lookup user: %w", err)
	}
	if u == nil {
		uc.log.Warn("verify email otp rejected", "reason", "user_not_found")
		return nil, application.ErrInvalidOTP
	}

	// Shared OTP validation gate — same attempt/expiry policy as the reset flow.
	token, remaining, err := validateOTPAttempt(ctx, uc.tokenRepo, u.ID, req.OTP, verificationtoken.TokenTypeEmailVerification, uc.log)
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

	uc.log.Info("email verified", "user_id", u.ID)
	return &VerifyEmailOTPResponse{AttemptsRemaining: MaxWrongOTPAttempts}, nil
}

// ---------------------------------------------------------------------------
// Resend Email OTP
// ---------------------------------------------------------------------------

// ResendEmailOTPRequest specifies the target user's email.
type ResendEmailOTPRequest struct {
	Email string
}

// ResendEmailOTPResponse carries the rate limit cooling period metadata.
type ResendEmailOTPResponse struct {
	RetryAfterSeconds int
}

// ResendEmailOTPUseCase invalidates existing codes and triggers a secure new OTP delivery.
type ResendEmailOTPUseCase struct {
	userRepo      user.Repository
	tokenRepo     verificationtoken.Repository
	rateLimiter   *redis.OTPRateLimitService
	dispatcher    taskqueue.Dispatcher
	log           logger.Logger
	otpLength     int
	otpExpiryMins int
}

// NewResendEmailOTPUseCase builds a new ResendEmailOTPUseCase.
func NewResendEmailOTPUseCase(
	userRepo user.Repository,
	tokenRepo verificationtoken.Repository,
	rateLimiter *redis.OTPRateLimitService,
	dispatcher taskqueue.Dispatcher,
	log logger.Logger,
) *ResendEmailOTPUseCase {
	return &ResendEmailOTPUseCase{
		userRepo:      userRepo,
		tokenRepo:     tokenRepo,
		rateLimiter:   rateLimiter,
		dispatcher:    dispatcher,
		log:           log.With("usecase", "resend_email_otp"),
		otpLength:     6,
		otpExpiryMins: 15,
	}
}

// Execute performs rate-limit checking, old token revocation, and enqueues a new OTP task.
func (uc *ResendEmailOTPUseCase) Execute(ctx context.Context, req ResendEmailOTPRequest) (*ResendEmailOTPResponse, error) {
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
		// Generic success responses to block user enumeration
		uc.log.Warn("resend otp no-op", "reason", "user_not_found")
		return &ResendEmailOTPResponse{}, nil
	}

	// Revoke all existing verification tokens of this type to maintain single-valid-token invariant
	if err := uc.tokenRepo.ExpireAllActiveForUser(ctx, u.ID, verificationtoken.TokenTypeEmailVerification); err != nil {
		uc.log.Error("resend otp failed", "step", "invalidate_previous_tokens", "user_id", u.ID, "error", err)
		return nil, fmt.Errorf("resend_otp: invalidate previous tokens: %w", err)
	}

	otpCode, err := otp.GenerateOTP(uc.otpLength)
	if err != nil {
		uc.log.Error("resend otp failed", "step", "generate_code", "user_id", u.ID, "error", err)
		return nil, fmt.Errorf("resend_otp: generate code: %w", err)
	}

	token := &verificationtoken.VerificationToken{
		ID:        uuid.New().String(),
		UserID:    u.ID,
		Token:     otpCode,
		Type:      verificationtoken.TokenTypeEmailVerification,
		ExpiresAt: time.Now().Add(time.Duration(uc.otpExpiryMins) * time.Minute),
	}
	if err := uc.tokenRepo.Create(ctx, token); err != nil {
		uc.log.Error("resend otp failed", "step", "persist_token", "user_id", u.ID, "error", err)
		return nil, fmt.Errorf("resend_otp: persist token: %w", err)
	}

	// Increment daily limit counter and start OTP cooldown timer
	if err := uc.rateLimiter.SetCooldown(ctx, redis.ScopeEmailVerification, req.Email); err != nil {
		// Non-fatal: proceed to email delivery regardless
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
		// Non-fatal to client since token exists and retry is available
		uc.log.Warn("failed to enqueue resend otp email", "user_id", u.ID, "error", err)
	}

	uc.log.Info("otp resent", "user_id", u.ID)
	return &ResendEmailOTPResponse{}, nil
}
