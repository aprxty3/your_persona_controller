package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/aprxty3/your_persona_controller.git/internal/application"
	"github.com/aprxty3/your_persona_controller.git/internal/domain/guestsession"
	"github.com/aprxty3/your_persona_controller.git/internal/domain/referral"
	"github.com/aprxty3/your_persona_controller.git/internal/domain/testresult"
	"github.com/aprxty3/your_persona_controller.git/internal/domain/user"
	"github.com/aprxty3/your_persona_controller.git/internal/domain/verificationtoken"
	"github.com/aprxty3/your_persona_controller.git/internal/infrastructure/cache/redis"
	jwtservice "github.com/aprxty3/your_persona_controller.git/internal/infrastructure/jwt"
	pgguestsession "github.com/aprxty3/your_persona_controller.git/internal/infrastructure/persistence/postgres/guestsession"
	pgreferral "github.com/aprxty3/your_persona_controller.git/internal/infrastructure/persistence/postgres/referral"
	pgtestresult "github.com/aprxty3/your_persona_controller.git/internal/infrastructure/persistence/postgres/testresult"
	pguser "github.com/aprxty3/your_persona_controller.git/internal/infrastructure/persistence/postgres/user"
	pgverificationtoken "github.com/aprxty3/your_persona_controller.git/internal/infrastructure/persistence/postgres/verificationtoken"
	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
	"github.com/aprxty3/your_persona_controller.git/pkg/otp"
	"github.com/aprxty3/your_persona_controller.git/pkg/taskqueue"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// AccountUseCase manages user registration, email verification, and password resets.
type AccountUseCase struct {
	db             *gorm.DB
	userRepo       user.Repository
	guestRepo      guestsession.Repository
	tokenRepo      verificationtoken.Repository
	referralRepo   referral.Repository
	testResultRepo testresult.Repository
	breachChecker  PasswordBreachChecker
	dispatcher     taskqueue.Dispatcher
	rateLimiter    *redis.OTPRateLimitService
	jwtService     *jwtservice.JWTService
	tokenStore     *redis.TokenStore
	log            logger.Logger
	otpLength      int
	otpExpiryMins  int
}

// NewAccountUseCase creates a new AccountUseCase.
func NewAccountUseCase(
	db *gorm.DB,
	userRepo user.Repository,
	guestRepo guestsession.Repository,
	tokenRepo verificationtoken.Repository,
	referralRepo referral.Repository,
	testResultRepo testresult.Repository,
	breachChecker PasswordBreachChecker,
	dispatcher taskqueue.Dispatcher,
	rateLimiter *redis.OTPRateLimitService,
	jwtService *jwtservice.JWTService,
	tokenStore *redis.TokenStore,
	log logger.Logger,
) *AccountUseCase {
	return &AccountUseCase{
		db:             db,
		userRepo:       userRepo,
		guestRepo:      guestRepo,
		tokenRepo:      tokenRepo,
		referralRepo:   referralRepo,
		testResultRepo: testResultRepo,
		breachChecker:  breachChecker,
		dispatcher:     dispatcher,
		rateLimiter:    rateLimiter,
		jwtService:     jwtService,
		tokenStore:     tokenStore,
		log:            log.With("usecase", "account"),
		otpLength:      6,
		otpExpiryMins:  15,
	}
}

// PasswordMinLength is the NIST-aligned minimum password length (FR-H1a).
const PasswordMinLength = 10

// PasswordBreachChecker defines the contract for HIBP checks (FR-H1a).
type PasswordBreachChecker interface {
	IsBreached(ctx context.Context, password string) (bool, error)
}

// NoopBreachChecker always reports passwords as NOT breached.
type NoopBreachChecker struct{}

// VerifyEmailOTPRequest represents payload structure for OTP validation.
type VerifyEmailOTPRequest struct {
	Email string
	OTP   string
}

// VerifyEmailOTPResponse carries remaining attempt statistics on failure.
type VerifyEmailOTPResponse struct {
	AttemptsRemaining int
}

// ResendEmailOTPRequest specifies the target user's email.
type ResendEmailOTPRequest struct {
	Email string
}

// ResendEmailOTPResponse carries the rate limit cooling period metadata.
type ResendEmailOTPResponse struct {
	RetryAfterSeconds int
}

// MaxWrongOTPAttempts defines maximum allowed invalid input attempts before token expiry.
const MaxWrongOTPAttempts = 5

// RegisterRequest holds the validated input for account creation.
type RegisterRequest struct {
	Email           string
	Password        string
	PreferredLocale string
	ReferralCode    *string // optional
	GuestSessionID  *string // from httpOnly cookie; nil when registering directly
}

// RegisterResponse contains the created user's UUID.
type RegisterResponse struct {
	UserID string
}

// ResetTokenTTL is the validity window of the short-lived reset_token JWT
const ResetTokenTTL = 15 * time.Minute

// ForgotPasswordRequest specifies the account email requesting a reset.
type ForgotPasswordRequest struct {
	Email string
}

// ForgotPasswordResponse carries rate limit metadata when throttled.
type ForgotPasswordResponse struct {
	RetryAfterSeconds int
}

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

// NewNoopBreachChecker creates a new NoopBreachChecker.
func NewNoopBreachChecker() PasswordBreachChecker {
	return &NoopBreachChecker{}
}

// IsBreached mocks the HIBP check by always returning false.
func (c *NoopBreachChecker) IsBreached(_ context.Context, _ string) (bool, error) {
	return false, nil
}

func txUserRepository(tx *gorm.DB, log logger.Logger) user.Repository {
	return pguser.NewUserRepository(tx, log)
}

func txGuestRepository(tx *gorm.DB, log logger.Logger) guestsession.Repository {
	return pgguestsession.NewGuestSessionRepository(tx, log)
}

func txTokenRepository(tx *gorm.DB, log logger.Logger) verificationtoken.Repository {
	return pgverificationtoken.NewVerificationTokenRepository(tx, log)
}

func txReferralRepository(tx *gorm.DB, log logger.Logger) referral.Repository {
	return pgreferral.NewReferralRepository(tx, log)
}

func txTestResultRepository(tx *gorm.DB, log logger.Logger) testresult.Repository {
	return pgtestresult.NewTestResultRepository(tx, log)
}

// ValidateNewPassword enforces the single shared password policy
func ValidateNewPassword(ctx context.Context, checker PasswordBreachChecker, fieldName, password string) error {
	if err := application.ValidateRequired(fieldName, password); err != nil {
		return err
	}
	if err := application.ValidateMinLength(fieldName, password, PasswordMinLength); err != nil {
		return application.ErrPasswordTooShort
	}
	if breached, err := checker.IsBreached(ctx, password); err == nil && breached {
		return application.ErrPasswordBreached
	}
	return nil
}

// HashPassword produces the bcrypt hash used everywhere a password is persisted.
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("password: bcrypt hash: %w", err)
	}
	return string(hash), nil
}

// validateOTPAttempt is the single shared OTP validation gate
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

// RegisterUseCase orchestrates account creation, data transitions, and referral conversions.
func (uc *AccountUseCase) Register(ctx context.Context, req RegisterRequest) (*RegisterResponse, error) {
	if err := application.ValidateRequired("email", req.Email); err != nil {
		return nil, err
	}
	if err := application.ValidateLocale("preferred_locale", req.PreferredLocale); err != nil {
		return nil, err
	}
	if req.ReferralCode != nil && *req.ReferralCode == "" {
		return nil, fmt.Errorf("%w: referral_code must not be an empty string — omit the field or pass null if you don't have one",
			application.ErrInvalidInput,
		)
	}

	// Shared password policy (FR-H1a): required, min length, HIBP breach check.
	if err := ValidateNewPassword(ctx, uc.breachChecker, "password", req.Password); err != nil {
		uc.log.Warn("registration rejected", "reason", "password_policy", "error", err)
		return nil, err
	}

	existing, err := uc.userRepo.FindByEmail(ctx, req.Email)
	if err != nil {
		uc.log.Error("registration failed", "step", "lookup_email", "error", err)
		return nil, fmt.Errorf("register: lookup email: %w", err)
	}
	if existing != nil {
		uc.log.Warn("registration rejected", "reason", "email_already_registered")
		return nil, application.ErrEmailAlreadyRegistered
	}

	hash, err := HashPassword(req.Password)
	if err != nil {
		uc.log.Error("registration failed", "step", "bcrypt_hash", "error", err)
		return nil, fmt.Errorf("register: %w", err)
	}

	var guest *guestsession.GuestSession
	if req.GuestSessionID != nil {
		guest, err = uc.guestRepo.FindBySessionID(ctx, *req.GuestSessionID)
		if err != nil {
			uc.log.Error("registration failed", "step", "find_guest_session", "error", err)
			return nil, fmt.Errorf("register: find guest session: %w", err)
		}
	}

	newUser := buildUser(req, hash, guest)
	txErr := uc.db.Transaction(func(tx *gorm.DB) error {
		txUserRepo := txUserRepository(tx, uc.log)
		txGuestRepo := txGuestRepository(tx, uc.log)
		txTokenRepo := txTokenRepository(tx, uc.log)
		txReferralRepo := txReferralRepository(tx, uc.log)
		txTestResultRepo := txTestResultRepository(tx, uc.log)

		if err := txUserRepo.Create(ctx, newUser); err != nil {
			uc.log.Error("registration failed", "step", "tx_create_user", "error", err)
			return fmt.Errorf("tx: create user: %w", err)
		}

		if guest != nil && !guest.IsClaimed() {
			guest.ClaimedByUserID = &newUser.ID
			if err := txGuestRepo.Update(ctx, guest); err != nil {
				uc.log.Error("registration failed", "step", "tx_claim_guest_session", "error", err)
				return fmt.Errorf("tx: claim guest session: %w", err)
			}

			if err := txTestResultRepo.ReassignGuestResults(ctx, newUser.ID, guest.SessionID); err != nil {
				uc.log.Error("registration failed", "step", "tx_reassign_test_results", "error", err)
				return fmt.Errorf("tx: reassign test results: %w", err)
			}

			if req.ReferralCode != nil {
				if err := recordReferralConversion(ctx, txReferralRepo, txTestResultRepo, newUser.ID, *req.ReferralCode, guest.SessionID, uc.log); err != nil {
					return err
				}
			}
		} else if req.ReferralCode != nil && guest == nil {
			if err := recordSignupEvent(ctx, txReferralRepo, newUser.ID, *req.ReferralCode, uc.log); err != nil {
				_ = err
			}
		}

		otpCode, err := otp.GenerateOTP(uc.otpLength)
		if err != nil {
			uc.log.Error("registration failed", "step", "generate_otp", "error", err)
			return fmt.Errorf("tx: generate verification code: %w", err)
		}

		token := &verificationtoken.VerificationToken{
			ID:        uuid.New().String(),
			UserID:    newUser.ID,
			Token:     otpCode,
			Type:      verificationtoken.TokenTypeEmailVerification,
			ExpiresAt: time.Now().Add(time.Duration(uc.otpExpiryMins) * time.Minute),
		}
		if err := txTokenRepo.Create(ctx, token); err != nil {
			uc.log.Error("registration failed", "step", "tx_persist_otp", "error", err)
			return fmt.Errorf("tx: persist otp: %w", err)
		}

		payload := taskqueue.SendEmailPayload{
			Type:   "otp_verification",
			UserID: newUser.ID,
			Email:  newUser.Email,
			OTP:    otpCode,
			Locale: newUser.PreferredLocale,
		}
		if err := uc.dispatcher.EnqueueEmail(ctx, payload, taskqueue.QueueCritical); err != nil {
			uc.log.Warn("failed to enqueue verification email, user can use resend", "user_id", newUser.ID, "error", err)
		}

		return nil
	})

	if txErr != nil {
		return nil, txErr
	}

	uc.log.Info("user registered", "user_id", newUser.ID)
	return &RegisterResponse{UserID: newUser.ID}, nil
}

// buildUser assembles a new user.User domain entity from registration input.
func buildUser(req RegisterRequest, hash string, guest *guestsession.GuestSession) *user.User {
	u := &user.User{
		ID:              uuid.New().String(),
		Email:           req.Email,
		PasswordHash:    hash,
		PreferredLocale: req.PreferredLocale,
		CreatedAt:       time.Now(),
		TokenVersion:    0,
	}
	if guest != nil {
		u.DisplayName = guest.DisplayName
		u.Age = guest.Age
		u.Status = guest.Status
	}
	if req.ReferralCode != nil {
		u.ReferredByCode = req.ReferralCode
	}
	return u
}

// recordReferralConversion records signup + optionally test_completed events
func recordReferralConversion(
	ctx context.Context,
	refRepo referral.Repository,
	trRepo testresult.Repository,
	newUserID,
	referralCode,
	guestSessionID string,
	log logger.Logger,
) error {
	refCode, err := refRepo.FindCodeByCode(ctx, referralCode)
	if err != nil {
		log.Warn("referral event skipped", "reason", "lookup_failed", "error", err)
		return nil
	}
	if refCode == nil {
		return nil
	}

	event := &referral.ReferralEvent{
		ID:             uuid.New().String(),
		ReferralCodeID: refCode.ID,
		ReferredUserID: newUserID,
		EventType:      referral.EventTypeSignup,
		CreatedAt:      time.Now(),
	}
	if err := refRepo.CreateEvent(ctx, event); err != nil {
		log.Warn("referral signup event insert failed", "error", err)
		return nil
	}

	count, err := trRepo.CountCompletedByGuestSession(ctx, guestSessionID)
	if err != nil {
		log.Warn("referral test count check failed", "error", err)
		return nil
	}
	if count > 0 {
		completedEvent := &referral.ReferralEvent{
			ID:             uuid.New().String(),
			ReferralCodeID: refCode.ID,
			ReferredUserID: newUserID,
			EventType:      referral.EventTypeTestCompleted,
			CreatedAt:      time.Now(),
		}
		if err := refRepo.CreateEvent(ctx, completedEvent); err != nil {
			log.Warn("referral test_completed event insert failed", "error", err)
		}
	}

	return nil
}

// recordSignupEvent records a signup referral event when there is no guest session.
func recordSignupEvent(ctx context.Context, refRepo referral.Repository, newUserID, referralCode string, log logger.Logger) error {
	refCode, err := refRepo.FindCodeByCode(ctx, referralCode)
	if err != nil {
		log.Warn("signup referral event skipped", "reason", "lookup_failed", "error", err)
		return nil
	}
	if refCode == nil {
		return nil
	}

	event := &referral.ReferralEvent{
		ID:             uuid.New().String(),
		ReferralCodeID: refCode.ID,
		ReferredUserID: newUserID,
		EventType:      referral.EventTypeSignup,
		CreatedAt:      time.Now(),
	}
	if err := refRepo.CreateEvent(ctx, event); err != nil {
		log.Warn("signup referral event insert failed", "error", err)
	}
	return nil
}

// EmailOTPUseCase handles verification and resending of user registration OTP codes.
func (uc *AccountUseCase) VerifyEmailOTP(ctx context.Context, req VerifyEmailOTPRequest) (*VerifyEmailOTPResponse, error) {
	u, err := uc.userRepo.FindByEmail(ctx, req.Email)
	if err != nil {
		uc.log.Error("verify email otp failed", "step", "lookup_user", "error", err)
		return nil, fmt.Errorf("verify_email_otp: lookup user: %w", err)
	}
	if u == nil {
		uc.log.Warn("verify email otp rejected", "reason", "user_not_found")
		return nil, application.ErrInvalidOTP
	}

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

// ResendEmailOTPExecute performs rate-limit checking, old token revocation, and enqueues a new OTP task.
func (uc *AccountUseCase) ResendEmailOTP(ctx context.Context, req ResendEmailOTPRequest) (*ResendEmailOTPResponse, error) {
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
		uc.log.Warn("resend otp no-op", "reason", "user_not_found")
		return &ResendEmailOTPResponse{}, nil
	}

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

	if err := uc.rateLimiter.SetCooldown(ctx, redis.ScopeEmailVerification, req.Email); err != nil {
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
		uc.log.Warn("failed to enqueue resend otp email", "user_id", u.ID, "error", err)
	}

	uc.log.Info("otp resent", "user_id", u.ID)
	return &ResendEmailOTPResponse{}, nil
}

// ForgotPasswordUseCase issues a password-reset OTP.
func (uc *AccountUseCase) ForgotPassword(ctx context.Context, req ForgotPasswordRequest) (*ForgotPasswordResponse, error) {
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
		uc.log.Info("forgot password no-op", "reason", "user_not_found")
		return &ForgotPasswordResponse{}, nil
	}

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
		uc.log.Warn("failed to enqueue reset otp email", "user_id", u.ID, "error", err)
	}

	uc.log.Info("password reset otp sent", "user_id", u.ID)
	return &ForgotPasswordResponse{}, nil
}

// VerifyResetOTPUseCase exchanges a valid reset OTP for a reset_token JWT.
func (uc *AccountUseCase) VerifyResetOTP(ctx context.Context, req VerifyResetOTPRequest) (*VerifyResetOTPResponse, error) {
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

	if err := uc.tokenStore.StoreResetJTI(ctx, jti, u.ID, ResetTokenTTL); err != nil {
		uc.log.Error("verify reset otp failed", "step", "store_reset_jti", "user_id", u.ID, "error", err)
		return nil, fmt.Errorf("verify_reset_otp: store reset jti: %w", err)
	}

	uc.log.Info("reset otp verified", "user_id", u.ID)
	return &VerifyResetOTPResponse{ResetToken: resetToken, AttemptsRemaining: MaxWrongOTPAttempts}, nil
}

// ResetPasswordUseCase consumes a reset_token (atomically, single-use)
func (uc *AccountUseCase) ResetPassword(ctx context.Context, req ResetPasswordRequest) (*ResetPasswordResponse, error) {
	if err := application.ValidateRequired("reset_token", req.ResetToken); err != nil {
		return nil, err
	}

	if err := ValidateNewPassword(ctx, uc.breachChecker, "new_password", req.NewPassword); err != nil {
		uc.log.Warn("reset password rejected", "reason", "password_policy", "error", err)
		return nil, err
	}

	claims, err := uc.jwtService.ParseResetToken(req.ResetToken)
	if err != nil {
		uc.log.Warn("reset password rejected", "reason", "invalid_reset_token", "error", err)
		return nil, application.ErrInvalidToken
	}

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

	err = uc.db.Transaction(func(tx *gorm.DB) error {
		txUserRepo := txUserRepository(tx, uc.log)

		u.PasswordHash = hash
		if err := txUserRepo.Update(ctx, u); err != nil {
			return fmt.Errorf("update password: %w", err)
		}
		if err := txUserRepo.IncrementTokenVersion(ctx, u.ID); err != nil {
			return fmt.Errorf("increment token version: %w", err)
		}
		if err := txUserRepo.UpdateLoginAttempt(ctx, u.ID, 0, nil); err != nil {
			return fmt.Errorf("clear login lockout: %w", err)
		}
		return nil
	})
	if err != nil {
		uc.log.Error("reset password failed", "step", "transaction", "user_id", u.ID, "error", err)
		return nil, fmt.Errorf("reset_password: %w", err)
	}

	pair, err := IssueTokenPair(uc.jwtService, u.ID, u.TokenVersion+1)
	if err != nil {
		uc.log.Error("reset password failed", "step", "issue_tokens", "user_id", u.ID, "error", err)
		return nil, fmt.Errorf("reset_password: %w", err)
	}

	uc.log.Info("password reset completed", "user_id", u.ID)
	return &ResetPasswordResponse{AccessToken: pair.AccessToken, RefreshToken: pair.RefreshToken}, nil
}
