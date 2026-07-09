package auth

// Forgot/Reset Password — the full 3-step FR-H4 flow in one file:
//  1. ForgotPasswordUseCase   POST /v1/auth/forgot-password   (send OTP)
//  2. VerifyResetOTPUseCase   POST /v1/auth/verify-reset-otp  (OTP → reset_token)
//  3. ResetPasswordUseCase    POST /v1/auth/reset-password    (reset_token → new password)

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
	"github.com/aprxty3/your_persona_controller.git/pkg/otp"
	"github.com/aprxty3/your_persona_controller.git/pkg/taskqueue"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ResetTokenTTL is the validity window of the short-lived reset_token JWT
// AND its single-use jti record in Redis — the two MUST stay identical (FR-H4).
const ResetTokenTTL = 15 * time.Minute

// ---------------------------------------------------------------------------
// Step 1/3 — Forgot Password (send reset OTP)
// ---------------------------------------------------------------------------

// ForgotPasswordRequest specifies the account email requesting a reset.
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

// ---------------------------------------------------------------------------
// Step 2/3 — Verify Reset OTP (OTP → single-use reset_token)
// ---------------------------------------------------------------------------

// VerifyResetOTPRequest carries the reset OTP to be exchanged.
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

// ---------------------------------------------------------------------------
// Step 3/3 — Reset Password (consume reset_token, revoke all sessions)
// ---------------------------------------------------------------------------

// ResetPasswordRequest carries the single-use reset_token and the new password.
type ResetPasswordRequest struct {
	ResetToken  string
	NewPassword string
}

// ResetPasswordResponse auto-logs the user in after a successful reset.
type ResetPasswordResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

// ResetPasswordUseCase consumes a reset_token (atomically, single-use), updates
// the password, and revokes every existing session via token_version increment.
type ResetPasswordUseCase struct {
	db            *gorm.DB
	userRepo      user.Repository
	breachChecker PasswordBreachChecker
	jwtService    *jwtservice.JWTService
	tokenStore    *redis.TokenStore
	log           logger.Logger
}

// NewResetPasswordUseCase constructs a new ResetPasswordUseCase.
func NewResetPasswordUseCase(
	db *gorm.DB,
	userRepo user.Repository,
	breachChecker PasswordBreachChecker,
	jwtService *jwtservice.JWTService,
	tokenStore *redis.TokenStore,
	log logger.Logger,
) *ResetPasswordUseCase {
	return &ResetPasswordUseCase{
		db:            db,
		userRepo:      userRepo,
		breachChecker: breachChecker,
		jwtService:    jwtService,
		tokenStore:    tokenStore,
		log:           log.With("usecase", "reset_password"),
	}
}

// Execute performs the final password reset step and returns a fresh session.
func (uc *ResetPasswordUseCase) Execute(ctx context.Context, req ResetPasswordRequest) (*ResetPasswordResponse, error) {
	if err := application.ValidateRequired("reset_token", req.ResetToken); err != nil {
		return nil, err
	}
	// Shared password policy (FR-H1a) — same gate as registration.
	if err := ValidateNewPassword(ctx, uc.breachChecker, "new_password", req.NewPassword); err != nil {
		uc.log.Warn("reset password rejected", "reason", "password_policy", "error", err)
		return nil, err
	}

	claims, err := uc.jwtService.ParseResetToken(req.ResetToken)
	if err != nil {
		uc.log.Warn("reset password rejected", "reason", "invalid_reset_token", "error", err)
		return nil, application.ErrInvalidToken
	}

	// Single-use consumption — atomic GETDEL. Two parallel requests with the
	// same token: exactly one obtains the jti, the other gets "" and is rejected.
	// Fail-CLOSED on Redis error: this is the replay-protection gate, skipping
	// it would allow unlimited reuse within the 15-minute window.
	consumedUserID, err := uc.tokenStore.ConsumeResetJTI(ctx, claims.ID)
	if err != nil {
		uc.log.Error("reset password failed", "step", "consume_reset_jti", "error", err)
		return nil, fmt.Errorf("reset_password: consume reset jti: %w", err)
	}
	if consumedUserID == "" || consumedUserID != claims.Subject {
		uc.log.Warn("reset password rejected", "reason", "reset_token_consumed_or_mismatched")
		return nil, application.ErrInvalidToken
	}

	u, err := uc.userRepo.FindByID(ctx, claims.Subject)
	if err != nil {
		uc.log.Error("reset password failed", "step", "lookup_user", "error", err)
		return nil, fmt.Errorf("reset_password: lookup user: %w", err)
	}
	if u == nil {
		uc.log.Warn("reset password rejected", "reason", "user_not_found")
		return nil, application.ErrInvalidToken
	}

	hash, err := HashPassword(req.NewPassword)
	if err != nil {
		uc.log.Error("reset password failed", "step", "hash_password", "user_id", u.ID, "error", err)
		return nil, fmt.Errorf("reset_password: %w", err)
	}

	// Password update + session revocation + lockout reset are one atomic unit:
	// a new password with old sessions still alive would defeat the reset.
	err = uc.db.Transaction(func(tx *gorm.DB) error {
		txUserRepo := txUserRepository(tx, uc.log)

		u.PasswordHash = hash
		if err := txUserRepo.Update(ctx, u); err != nil {
			return fmt.Errorf("update password: %w", err)
		}
		// Revoke ALL existing sessions (access + refresh) — FR-H4.
		if err := txUserRepo.IncrementTokenVersion(ctx, u.ID); err != nil {
			return fmt.Errorf("increment token version: %w", err)
		}
		// A successful reset proves account ownership — clear any login lockout.
		if err := txUserRepo.UpdateLoginAttempt(ctx, u.ID, 0, nil); err != nil {
			return fmt.Errorf("clear login lockout: %w", err)
		}
		return nil
	})
	if err != nil {
		uc.log.Error("reset password failed", "step", "transaction", "user_id", u.ID, "error", err)
		return nil, fmt.Errorf("reset_password: %w", err)
	}

	// Auto-login with the NEW token version (old sessions are now all invalid).
	pair, err := IssueTokenPair(uc.jwtService, u.ID, u.TokenVersion+1)
	if err != nil {
		uc.log.Error("reset password failed", "step", "issue_tokens", "user_id", u.ID, "error", err)
		return nil, fmt.Errorf("reset_password: %w", err)
	}

	uc.log.Info("password reset completed", "user_id", u.ID)
	return &ResetPasswordResponse{AccessToken: pair.AccessToken, RefreshToken: pair.RefreshToken}, nil
}
