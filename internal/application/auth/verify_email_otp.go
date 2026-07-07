package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aprxty3/your_persona_controller.git/internal/domain/user"
	"github.com/aprxty3/your_persona_controller.git/internal/domain/verificationtoken"
)

// Sentinel validation error codes matching API specifications.
var (
	ErrInvalidOTP     = errors.New("INVALID_OTP")
	ErrOTPExpired     = errors.New("OTP_EXPIRED")
	ErrOTPMaxAttempts = errors.New("OTP_MAX_ATTEMPTS")
)

// MaxWrongOTPAttempts defines maximum allowed invalid input attempts before token expiry.
const MaxWrongOTPAttempts = 5

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
}

// NewVerifyEmailOTPUseCase constructs a new VerifyEmailOTPUseCase.
func NewVerifyEmailOTPUseCase(userRepo user.Repository, tokenRepo verificationtoken.Repository) *VerifyEmailOTPUseCase {
	return &VerifyEmailOTPUseCase{userRepo: userRepo, tokenRepo: tokenRepo}
}

// Execute validates an email verification code and activates the user account.
func (uc *VerifyEmailOTPUseCase) Execute(ctx context.Context, req VerifyEmailOTPRequest) (*VerifyEmailOTPResponse, error) {
	u, err := uc.userRepo.FindByEmail(ctx, req.Email)
	if err != nil {
		return nil, fmt.Errorf("verify_email_otp: lookup user: %w", err)
	}
	if u == nil {
		return nil, ErrInvalidOTP
	}

	token, err := uc.tokenRepo.FindActiveByUserAndType(ctx, u.ID, verificationtoken.TokenTypeEmailVerification)
	if err != nil {
		return nil, fmt.Errorf("verify_email_otp: find token: %w", err)
	}
	if token == nil {
		return nil, ErrOTPExpired
	}

	if token.AttemptCount >= MaxWrongOTPAttempts {
		return &VerifyEmailOTPResponse{AttemptsRemaining: 0}, ErrOTPMaxAttempts
	}

	if time.Now().After(token.ExpiresAt) {
		return nil, ErrOTPExpired
	}

	if token.Token != req.OTP {
		if err := uc.tokenRepo.IncrementAttemptCount(ctx, token.ID); err != nil {
			return nil, fmt.Errorf("verify_email_otp: increment token attempts: %w", err)
		}
		remaining := MaxWrongOTPAttempts - (token.AttemptCount + 1)
		if remaining <= 0 {
			return &VerifyEmailOTPResponse{AttemptsRemaining: 0}, ErrOTPMaxAttempts
		}
		return &VerifyEmailOTPResponse{AttemptsRemaining: remaining}, ErrInvalidOTP
	}

	// Consume token
	if err := uc.tokenRepo.MarkUsed(ctx, token.ID); err != nil {
		return nil, fmt.Errorf("verify_email_otp: mark token used: %w", err)
	}

	now := time.Now()
	u.EmailVerifiedAt = &now
	if err := uc.userRepo.Update(ctx, u); err != nil {
		return nil, fmt.Errorf("verify_email_otp: update user: %w", err)
	}

	return &VerifyEmailOTPResponse{AttemptsRemaining: MaxWrongOTPAttempts}, nil
}
