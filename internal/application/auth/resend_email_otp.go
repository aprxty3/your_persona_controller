package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aprxty3/your_persona_controller.git/internal/domain/user"
	"github.com/aprxty3/your_persona_controller.git/internal/domain/verificationtoken"
	"github.com/aprxty3/your_persona_controller.git/internal/infrastructure/cache/redis"
	"github.com/aprxty3/your_persona_controller.git/pkg/taskqueue"
	"github.com/google/uuid"
)

// ErrRateLimited is returned when OTP cooldown or daily cap is exceeded.
var ErrRateLimited = errors.New("RATE_LIMITED")

// ResendEmailOTPRequest is the input for the resend-email-otp endpoint.
type ResendEmailOTPRequest struct {
	Email string
}

// ResendEmailOTPResponse carries the retry_after_seconds for the meta field when rate-limited.
type ResendEmailOTPResponse struct {
	RetryAfterSeconds int // > 0 only when rate-limited
}

// ResendEmailOTPUseCase sends a fresh OTP after invalidating any existing active token.
type ResendEmailOTPUseCase struct {
	userRepo      user.Repository
	tokenRepo     verificationtoken.Repository
	rateLimiter   *redis.OTPRateLimitService
	dispatcher    taskqueue.Dispatcher
	otpLength     int
	otpExpiryMins int
}

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

func (uc *ResendEmailOTPUseCase) Execute(ctx context.Context, req ResendEmailOTPRequest) (*ResendEmailOTPResponse, error) {
	// Rate limit check
	retryAfter, err := uc.rateLimiter.CheckAndConsume(ctx, req.Email)
	if err != nil {
		return nil, fmt.Errorf("resend_otp: rate limit check: %w", err)
	}
	if retryAfter > 0 {
		return &ResendEmailOTPResponse{RetryAfterSeconds: retryAfter}, ErrRateLimited
	}

	// Find user
	u, err := uc.userRepo.FindByEmail(ctx, req.Email)
	if err != nil {
		return nil, fmt.Errorf("resend_otp: find user: %w", err)
	}
	if u == nil {
		return &ResendEmailOTPResponse{}, nil
	}

	// Expire all existing active tokens for this user+type (single-token invariant)
	if err := uc.tokenRepo.ExpireAllActiveForUser(ctx, u.ID, verificationtoken.TokenTypeEmailVerification); err != nil {
		return nil, fmt.Errorf("resend_otp: expire old tokens: %w", err)
	}

	// Create new OTP token
	otp := generateOTP(uc.otpLength)
	token := &verificationtoken.VerificationToken{
		ID:        uuid.New().String(),
		UserID:    u.ID,
		Token:     otp,
		Type:      verificationtoken.TokenTypeEmailVerification,
		ExpiresAt: time.Now().Add(time.Duration(uc.otpExpiryMins) * time.Minute),
	}
	if err := uc.tokenRepo.Create(ctx, token); err != nil {
		return nil, fmt.Errorf("resend_otp: create token: %w", err)
	}

	// Set cooldown + increment daily counter (after successful token creation)
	if err := uc.rateLimiter.SetCooldown(ctx, req.Email); err != nil {
		_ = err
	}

	// Enqueue email
	payload := taskqueue.SendEmailPayload{
		Type:   "otp_verification",
		UserID: u.ID,
		Email:  u.Email,
		OTP:    otp,
		Locale: u.PreferredLocale,
	}
	if err := uc.dispatcher.EnqueueEmail(ctx, payload, taskqueue.QueueCritical); err != nil {
		_ = err // non-fatal
	}

	return &ResendEmailOTPResponse{}, nil
}
