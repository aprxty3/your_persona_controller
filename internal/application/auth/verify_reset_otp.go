package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/aprxty3/your_persona_controller.git/internal/application"
	"github.com/aprxty3/your_persona_controller.git/internal/domain/user"
	"github.com/aprxty3/your_persona_controller.git/internal/domain/verificationtoken"
	"github.com/aprxty3/your_persona_controller.git/internal/infrastructure/cache/redis"
	jwtservice "github.com/aprxty3/your_persona_controller.git/internal/infrastructure/jwt"
	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
)

// ResetTokenTTL is the validity window of the short-lived reset_token JWT
// AND its single-use jti record in Redis — the two MUST stay identical (FR-H4).
const ResetTokenTTL = 15 * time.Minute

// VerifyResetOTPRequest carries the reset OTP to be exchanged (FR-H4 step 2/3).
type VerifyResetOTPRequest struct {
	Email string
	OTP   string
}

// VerifyResetOTPResponse returns the short-lived single-use reset_token.
type VerifyResetOTPResponse struct {
	ResetToken        string `json:"reset_token"`
	AttemptsRemaining int    `json:"-"`
}

// VerifyResetOTPUseCase exchanges a valid reset OTP for a reset_token JWT.
// The OTP itself is never a credential that can change the password — it only
// buys a narrower, single-use token (defense in depth, see PRD FR-H4).
type VerifyResetOTPUseCase struct {
	userRepo   user.Repository
	tokenRepo  verificationtoken.Repository
	jwtService *jwtservice.JWTService
	tokenStore *redis.TokenStore
	log        logger.Logger
}

// NewVerifyResetOTPUseCase constructs a new VerifyResetOTPUseCase.
func NewVerifyResetOTPUseCase(
	userRepo user.Repository,
	tokenRepo verificationtoken.Repository,
	jwtService *jwtservice.JWTService,
	tokenStore *redis.TokenStore,
	log logger.Logger,
) *VerifyResetOTPUseCase {
	return &VerifyResetOTPUseCase{
		userRepo:   userRepo,
		tokenRepo:  tokenRepo,
		jwtService: jwtService,
		tokenStore: tokenStore,
		log:        log.With("usecase", "verify_reset_otp"),
	}
}

// Execute validates the reset OTP and mints a registered single-use reset_token.
func (uc *VerifyResetOTPUseCase) Execute(ctx context.Context, req VerifyResetOTPRequest) (*VerifyResetOTPResponse, error) {
	if err := application.ValidateRequired("email", req.Email); err != nil {
		return nil, err
	}
	if err := application.ValidateRequired("otp", req.OTP); err != nil {
		return nil, err
	}

	u, err := uc.userRepo.FindByEmail(ctx, req.Email)
	if err != nil {
		uc.log.Error("verify reset otp failed", "step", "lookup_user", "error", err)
		return nil, fmt.Errorf("verify_reset_otp: lookup user: %w", err)
	}
	if u == nil {
		// Same generic error as a wrong code — do not reveal that the email is unknown.
		uc.log.Warn("verify reset otp rejected", "reason", "user_not_found")
		return nil, application.ErrInvalidOTP
	}

	token, remaining, err := validateOTPAttempt(ctx, uc.tokenRepo, u.ID, req.OTP, verificationtoken.TokenTypePasswordReset, uc.log)
	if err != nil {
		return &VerifyResetOTPResponse{AttemptsRemaining: remaining}, err
	}

	if err := uc.tokenRepo.MarkUsed(ctx, token.ID); err != nil {
		uc.log.Error("verify reset otp failed", "step", "mark_token_used", "user_id", u.ID, "error", err)
		return nil, fmt.Errorf("verify_reset_otp: mark token used: %w", err)
	}

	jti, resetToken, err := uc.jwtService.GenerateResetToken(u.ID, ResetTokenTTL)
	if err != nil {
		uc.log.Error("verify reset otp failed", "step", "issue_reset_token", "user_id", u.ID, "error", err)
		return nil, fmt.Errorf("verify_reset_otp: issue reset token: %w", err)
	}

	// Register the jti so /reset-password can consume it exactly once (GETDEL).
	// Fail-closed: without the Redis record the reset token would be unusable
	// anyway, so surface the error now instead of a confusing failure later.
	if err := uc.tokenStore.StoreResetJTI(ctx, jti, u.ID, ResetTokenTTL); err != nil {
		uc.log.Error("verify reset otp failed", "step", "store_reset_jti", "user_id", u.ID, "error", err)
		return nil, fmt.Errorf("verify_reset_otp: store reset jti: %w", err)
	}

	uc.log.Info("reset otp verified", "user_id", u.ID)
	return &VerifyResetOTPResponse{ResetToken: resetToken, AttemptsRemaining: MaxWrongOTPAttempts}, nil
}
