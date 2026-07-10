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
	pgguestsession "github.com/aprxty3/your_persona_controller.git/internal/infrastructure/persistence/postgres/guestsession"
	pgreferral "github.com/aprxty3/your_persona_controller.git/internal/infrastructure/persistence/postgres/referral"
	pgtestresult "github.com/aprxty3/your_persona_controller.git/internal/infrastructure/persistence/postgres/testresult"
	pguser "github.com/aprxty3/your_persona_controller.git/internal/infrastructure/persistence/postgres/user"
	pgverificationtoken "github.com/aprxty3/your_persona_controller.git/internal/infrastructure/persistence/postgres/verificationtoken"
	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
	"github.com/aprxty3/your_persona_controller.git/pkg/otp"
	"github.com/aprxty3/your_persona_controller.git/pkg/taskqueue"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

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

// RegisterRequest holds the validated input for account creation.
type RegisterRequest struct {
	Email           string
	Password        string
	PreferredLocale string
	ReferralCode    *string // optional
	GuestSessionID  *string // from httpOnly cookie; nil when registering directly
	IPAddress       string  // raw client IP, used for per-IP rate limiting only (not persisted)
}

// RegisterResponse contains the created user's UUID.
type RegisterResponse struct {
	UserID            string
	RetryAfterSeconds int `json:"-"` // set only when the register call itself returned ErrRateLimited
}

// RegisterUseCase orchestrates account creation, data transitions, and referral conversions.
type RegisterUseCase struct {
	db             *gorm.DB
	userRepo       user.Repository
	guestRepo      guestsession.Repository
	tokenRepo      verificationtoken.Repository
	referralRepo   referral.Repository
	testResultRepo testresult.Repository
	breachChecker  PasswordBreachChecker
	dispatcher     taskqueue.Dispatcher
	ipRateLimiter  *redis.IPRateLimitService
	log            logger.Logger
	otpLength      int
	otpExpiryMins  int
}

// NewRegisterUseCase creates a new RegisterUseCase.
func NewRegisterUseCase(
	db *gorm.DB,
	userRepo user.Repository,
	guestRepo guestsession.Repository,
	tokenRepo verificationtoken.Repository,
	referralRepo referral.Repository,
	testResultRepo testresult.Repository,
	breachChecker PasswordBreachChecker,
	dispatcher taskqueue.Dispatcher,
	ipRateLimiter *redis.IPRateLimitService,
	log logger.Logger,
) *RegisterUseCase {
	return &RegisterUseCase{
		db:             db,
		userRepo:       userRepo,
		guestRepo:      guestRepo,
		tokenRepo:      tokenRepo,
		referralRepo:   referralRepo,
		testResultRepo: testResultRepo,
		breachChecker:  breachChecker,
		dispatcher:     dispatcher,
		ipRateLimiter:  ipRateLimiter,
		log:            log.With("usecase", "register"),
		otpLength:      6,
		otpExpiryMins:  15,
	}
}

// Register orchestrates account creation, data transitions, and referral conversions.
func (uc *RegisterUseCase) Register(ctx context.Context, req RegisterRequest) (*RegisterResponse, error) {
	allowed, retryAfter, err := uc.ipRateLimiter.Allow(ctx, redis.ScopeRegisterIP, req.IPAddress)
	if err != nil {
		uc.log.Warn("register ip rate limit check skipped", "reason", "redis_error", "error", err)
	} else if !allowed {
		uc.log.Warn("registration rejected", "reason", "rate_limited", "retry_after_seconds", retryAfter)
		return &RegisterResponse{RetryAfterSeconds: retryAfter}, application.ErrRateLimited
	}

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
			recordSignupEvent(ctx, txReferralRepo, newUser.ID, *req.ReferralCode, uc.log)
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

// recordReferralConversion records signup
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
func recordSignupEvent(ctx context.Context, refRepo referral.Repository, newUserID, referralCode string, log logger.Logger) {
	refCode, err := refRepo.FindCodeByCode(ctx, referralCode)
	if err != nil {
		log.Warn("signup referral event skipped", "reason", "lookup_failed", "error", err)
		return
	}
	if refCode == nil {
		return
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
}
