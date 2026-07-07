package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aprxty3/your_persona_controller.git/internal/domain/user"
	"github.com/aprxty3/your_persona_controller.git/internal/domain/verificationtoken"
)

// Sentinel errors mapped to specific error codes in Section 4.4 of TECHNICAL_DOCUMENTATION.
var (
	ErrInvalidOTP     = errors.New("INVALID_OTP")
	ErrOTPExpired     = errors.New("OTP_EXPIRED")
	ErrOTPMaxAttempts = errors.New("OTP_MAX_ATTEMPTS")
)

// OTPMaxAttempts is the maximum number of wrong guesses before requiring a new OTP (FR-H2a).
const OTPMaxAttempts = 5

// VerifyEmailOTPRequest is the input for the verify-email-otp endpoint.
type VerifyEmailOTPRequest struct {
	Email string
	OTP   string
}

// VerifyEmailOTPResponse carries the remaining attempts on failure for the meta field.
type VerifyEmailOTPResponse struct {
	AttemptsRemaining int // included in meta on failure
}

// VerifyEmailOTPUseCase validates an OTP submitted by the user after registration.
type VerifyEmailOTPUseCase struct {
	userRepo  user.Repository
	tokenRepo verificationtoken.Repository
}

func NewVerifyEmailOTPUseCase(userRepo user.Repository, tokenRepo verificationtoken.Repository) *VerifyEmailOTPUseCase {
	return &VerifyEmailOTPUseCase{userRepo: userRepo, tokenRepo: tokenRepo}
}

func (uc *VerifyEmailOTPUseCase) Execute(ctx context.Context, req VerifyEmailOTPRequest) (*VerifyEmailOTPResponse, error) {
	// Find user by email
	u, err := uc.userRepo.FindByEmail(ctx, req.Email)
	if err != nil {
		return nil, fmt.Errorf("verify_email_otp: find user: %w", err)
	}
	if u == nil {
		// Generic error — don't reveal whether the email exists
		return nil, ErrInvalidOTP
	}

	// Find active OTP token scoped to (user_id, email_verification)
	token, err := uc.tokenRepo.FindActiveByUserAndType(ctx, u.ID, verificationtoken.TokenTypeEmailVerification)
	if err != nil {
		return nil, fmt.Errorf("verify_email_otp: find token: %w", err)
	}
	if token == nil {
		return nil, ErrOTPExpired
	}

	// Check if already at max attempts
	if token.AttemptCount >= OTPMaxAttempts {
		return &VerifyEmailOTPResponse{AttemptsRemaining: 0}, ErrOTPMaxAttempts
	}

	// Check expiry
	if time.Now().After(token.ExpiresAt) {
		return nil, ErrOTPExpired
	}

	// Compare OTP
	if token.Token != req.OTP {
		// Increment attempt counter
		if err := uc.tokenRepo.IncrementAttemptCount(ctx, token.ID); err != nil {
			return nil, fmt.Errorf("verify_email_otp: increment attempt: %w", err)
		}
		remaining := OTPMaxAttempts - (token.AttemptCount + 1)
		if remaining <= 0 {
			return &VerifyEmailOTPResponse{AttemptsRemaining: 0}, ErrOTPMaxAttempts
		}
		return &VerifyEmailOTPResponse{AttemptsRemaining: remaining}, ErrInvalidOTP
	}

	// OTP correct — consume token and verify user
	if err := uc.tokenRepo.MarkUsed(ctx, token.ID); err != nil {
		return nil, fmt.Errorf("verify_email_otp: mark used: %w", err)
	}

	now := time.Now()
	u.EmailVerifiedAt = &now
	if err := uc.userRepo.Update(ctx, u); err != nil {
		return nil, fmt.Errorf("verify_email_otp: update user: %w", err)
	}

	return &VerifyEmailOTPResponse{AttemptsRemaining: OTPMaxAttempts}, nil
}
