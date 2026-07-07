package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aprxty3/your_persona_controller.git/internal/domain/user"
	"github.com/aprxty3/your_persona_controller.git/internal/domain/verificationtoken"
	"github.com/aprxty3/your_persona_controller.git/internal/infrastructure/cache/redis"
	"github.com/aprxty3/your_persona_controller.git/pkg/otp"
	"github.com/aprxty3/your_persona_controller.git/pkg/taskqueue"
	"github.com/google/uuid"
)

// ErrRateLimited is raised when rolling daily cap or retry cooldown expires.
var ErrRateLimited = errors.New("RATE_LIMITED")

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
	otpLength     int
	otpExpiryMins int
}

// NewResendEmailOTPUseCase builds a new ResendEmailOTPUseCase.
func NewResendEmailOTPUseCase(
	userRepo user.Repository,
	tokenRepo verificationtoken.Repository,
	rateLimiter *redis.OTPRateLimitService,
	dispatcher taskqueue.Dispatcher,
) *ResendEmailOTPUseCase {
	return &ResendEmailOTPUseCase{
		userRepo:      userRepo,
		tokenRepo:     tokenRepo,
		rateLimiter:   rateLimiter,
		dispatcher:    dispatcher,
		otpLength:     6,
		otpExpiryMins: 15,
	}
}

// Execute performs rate-limit checking, old token revocation, and enqueues a new OTP task.
func (uc *ResendEmailOTPUseCase) Execute(ctx context.Context, req ResendEmailOTPRequest) (*ResendEmailOTPResponse, error) {
	retryAfter, err := uc.rateLimiter.CheckAndConsume(ctx, req.Email)
	if err != nil {
		return nil, fmt.Errorf("resend_otp: rate limit evaluation: %w", err)
	}
	if retryAfter > 0 {
		return &ResendEmailOTPResponse{RetryAfterSeconds: retryAfter}, ErrRateLimited
	}

	u, err := uc.userRepo.FindByEmail(ctx, req.Email)
	if err != nil {
		return nil, fmt.Errorf("resend_otp: lookup user: %w", err)
	}
	if u == nil {
		// Generic success responses to block user enumeration
		return &ResendEmailOTPResponse{}, nil
	}

	// Revoke all existing verification tokens of this type to maintain single-valid-token invariant
	if err := uc.tokenRepo.ExpireAllActiveForUser(ctx, u.ID, verificationtoken.TokenTypeEmailVerification); err != nil {
		return nil, fmt.Errorf("resend_otp: invalidate previous tokens: %w", err)
	}

	otpCode, err := otp.GenerateOTP(uc.otpLength)
	if err != nil {
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
		return nil, fmt.Errorf("resend_otp: persist token: %w", err)
	}

	// Increment daily limit counter and start OTP cooldown timer
	if err := uc.rateLimiter.SetCooldown(ctx, req.Email); err != nil {
		// Non-fatal: log rate limit state update failure but proceed to email delivery
		_ = err
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
		_ = err
	}

	return &ResendEmailOTPResponse{}, nil
}
