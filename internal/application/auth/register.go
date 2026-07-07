package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aprxty3/your_persona_controller.git/internal/domain/guestsession"
	"github.com/aprxty3/your_persona_controller.git/internal/domain/user"
	"github.com/aprxty3/your_persona_controller.git/internal/domain/verificationtoken"
	"github.com/aprxty3/your_persona_controller.git/internal/infrastructure/persistence/postgres"
	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
	"github.com/aprxty3/your_persona_controller.git/pkg/otp"
	"github.com/aprxty3/your_persona_controller.git/pkg/taskqueue"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// Sentinel registration error definitions.
var (
	ErrEmailAlreadyRegistered = errors.New("email already registered")
	ErrPasswordTooShort       = errors.New("password must be at least 10 characters")
)

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

// PasswordBreachChecker defines the contract for HIBP checks (FR-H1a).
type PasswordBreachChecker interface {
	IsBreached(ctx context.Context, password string) (bool, error)
}

// RegisterUseCase orchestrates account creation, data transitions, and referral conversions.
type RegisterUseCase struct {
	db            *gorm.DB
	userRepo      user.Repository
	guestRepo     guestsession.Repository
	tokenRepo     verificationtoken.Repository
	breachChecker PasswordBreachChecker
	dispatcher    taskqueue.Dispatcher
	log           logger.Logger
	otpLength     int
	otpExpiryMins int
}

// NewRegisterUseCase constructs a new RegisterUseCase.
func NewRegisterUseCase(
	db *gorm.DB,
	userRepo user.Repository,
	guestRepo guestsession.Repository,
	tokenRepo verificationtoken.Repository,
	breachChecker PasswordBreachChecker,
	dispatcher taskqueue.Dispatcher,
	log logger.Logger,
) *RegisterUseCase {
	return &RegisterUseCase{
		db:            db,
		userRepo:      userRepo,
		guestRepo:     guestRepo,
		tokenRepo:     tokenRepo,
		breachChecker: breachChecker,
		dispatcher:    dispatcher,
		log:           log.With("usecase", "register"),
		otpLength:     6,
		otpExpiryMins: 15,
	}
}

// Execute performs atomic multi-record mutations in a single transaction.
func (uc *RegisterUseCase) Execute(ctx context.Context, req RegisterRequest) (*RegisterResponse, error) {
	if len(req.Password) < 10 {
		uc.log.Warn("registration rejected", "reason", "password_too_short")
		return nil, ErrPasswordTooShort
	}

	// HIBP check — fail-open on HIBP API failure so signups are not blocked
	if breached, err := uc.breachChecker.IsBreached(ctx, req.Password); err == nil && breached {
		uc.log.Warn("registration rejected", "reason", "password_breached")
		return nil, errors.New("password found in known breach database — please choose a different password")
	}

	existing, err := uc.userRepo.FindByEmail(ctx, req.Email)
	if err != nil {
		uc.log.Error("registration failed", "step", "lookup_email", "error", err)
		return nil, fmt.Errorf("register: lookup email: %w", err)
	}
	if existing != nil {
		uc.log.Warn("registration rejected", "reason", "email_already_registered")
		return nil, ErrEmailAlreadyRegistered
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		uc.log.Error("registration failed", "step", "bcrypt_hash", "error", err)
		return nil, fmt.Errorf("register: bcrypt hash: %w", err)
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

	// Single atomic database transaction for multi-table writes
	txErr := uc.db.Transaction(func(tx *gorm.DB) error {
		txUserRepo := txUserRepository(tx, uc.log)
		txGuestRepo := txGuestRepository(tx, uc.log)
		txTokenRepo := txTokenRepository(tx, uc.log)

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

			// Reassign assessment results (XOR constraint)
			if err := tx.Exec(
				"UPDATE test_results SET user_id = ?, guest_session_id = NULL WHERE guest_session_id = ? AND status IN ('completed','fallback_static')",
				newUser.ID, guest.SessionID,
			).Error; err != nil {
				uc.log.Error("registration failed", "step", "tx_reassign_test_results", "error", err)
				return fmt.Errorf("tx: reassign test results: %w", err)
			}

			if req.ReferralCode != nil {
				if err := createReferralEvents(ctx, tx, newUser.ID, *req.ReferralCode, guest.SessionID); err != nil {
					return err
				}
			}
		} else if req.ReferralCode != nil && guest == nil {
			if err := createSignupEvent(ctx, tx, newUser.ID, *req.ReferralCode); err != nil {
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

		// Enqueue verification email asynchronously
		payload := taskqueue.SendEmailPayload{
			Type:   "otp_verification",
			UserID: newUser.ID,
			Email:  newUser.Email,
			OTP:    otpCode,
			Locale: newUser.PreferredLocale,
		}
		if err := uc.dispatcher.EnqueueEmail(ctx, payload, taskqueue.QueueCritical); err != nil {
			// Enqueue errors do not rollback the registration — users can click resend.
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

func buildUser(req RegisterRequest, hash []byte, guest *guestsession.GuestSession) *user.User {
	u := &user.User{
		ID:              uuid.New().String(),
		Email:           req.Email,
		PasswordHash:    string(hash),
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

func createReferralEvents(ctx context.Context, tx *gorm.DB, newUserID, referralCode, guestSessionID string) error {
	var referralCodeID string
	if err := tx.Raw("SELECT id FROM referral_codes WHERE code = ?", referralCode).
		Scan(&referralCodeID).Error; err != nil || referralCodeID == "" {
		return nil // Silent ignore on invalid codes
	}

	if err := tx.Exec(
		"INSERT INTO referral_events (id, referral_code_id, referred_user_id, event_type, created_at) VALUES (?, ?, ?, 'signup', NOW())",
		uuid.New().String(), referralCodeID, newUserID,
	).Error; err != nil {
		return nil
	}

	var count int64
	tx.Raw(
		"SELECT COUNT(*) FROM test_results WHERE guest_session_id = ? AND status IN ('completed','fallback_static')",
		guestSessionID,
	).Scan(&count)
	if count > 0 {
		_ = tx.Exec(
			"INSERT INTO referral_events (id, referral_code_id, referred_user_id, event_type, created_at) VALUES (?, ?, ?, 'test_completed', NOW())",
			uuid.New().String(), referralCodeID, newUserID,
		).Error
	}

	return nil
}

func createSignupEvent(ctx context.Context, tx *gorm.DB, newUserID, referralCode string) error {
	var referralCodeID string
	if err := tx.Raw("SELECT id FROM referral_codes WHERE code = ?", referralCode).
		Scan(&referralCodeID).Error; err != nil || referralCodeID == "" {
		return nil
	}
	return tx.Exec(
		"INSERT INTO referral_events (id, referral_code_id, referred_user_id, event_type, created_at) VALUES (?, ?, ?, 'signup', NOW())",
		uuid.New().String(), referralCodeID, newUserID,
	).Error
}

func txUserRepository(tx *gorm.DB, log logger.Logger) user.Repository {
	return postgres.NewUserRepository(tx, log)
}

func txGuestRepository(tx *gorm.DB, log logger.Logger) guestsession.Repository {
	return postgres.NewGuestSessionRepository(tx, log)
}

func txTokenRepository(tx *gorm.DB, log logger.Logger) verificationtoken.Repository {
	return postgres.NewVerificationTokenRepository(tx, log)
}
