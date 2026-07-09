package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/aprxty3/your_persona_controller.git/internal/application"
	"github.com/aprxty3/your_persona_controller.git/internal/domain/user"
	"github.com/aprxty3/your_persona_controller.git/internal/domain/verificationtoken"
	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
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
