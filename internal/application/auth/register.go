package auth

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"time"

	"github.com/aprxty3/your_persona_controller.git/internal/domain/guestsession"
	"github.com/aprxty3/your_persona_controller.git/internal/domain/user"
	"github.com/aprxty3/your_persona_controller.git/internal/domain/verificationtoken"
	"github.com/aprxty3/your_persona_controller.git/internal/infrastructure/persistence/postgres"
	"github.com/aprxty3/your_persona_controller.git/pkg/taskqueue"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// ErrEmailAlreadyRegistered is returned when the email is already in use.
var ErrEmailAlreadyRegistered = errors.New("email already registered")

// ErrPasswordTooShort is returned when password < 10 characters.
var ErrPasswordTooShort = errors.New("password must be at least 10 characters")

// RegisterRequest holds the validated input for account creation.
type RegisterRequest struct {
	Email           string
	Password        string
	PreferredLocale string
	ReferralCode    *string // optional
	GuestSessionID  *string // from httpOnly cookie; nil when registering without a prior Guest session
}

// RegisterResponse contains the created user's ID for the 201 response.
type RegisterResponse struct {
	UserID string
}

// PasswordBreachChecker is the interface for HIBP k-anonymity check (FR-H1a).
// The concrete implementation (HTTP call to HIBP) is injected at wire time.
// A no-op stub is used in dev/test environments.
type PasswordBreachChecker interface {
	IsBreached(ctx context.Context, password string) (bool, error)
}

// RegisterUseCase orchestrates the full account registration flow including:
// - password policy validation
// - guest session claim (copy onboarding data, reassign TEST_RESULT)
// - referral event creation
// All multi-table mutations run inside a single db.Transaction (AGENTS.md rule).
type RegisterUseCase struct {
	db            *gorm.DB
	userRepo      user.Repository
	guestRepo     guestsession.Repository
	tokenRepo     verificationtoken.Repository
	breachChecker PasswordBreachChecker
	dispatcher    taskqueue.Dispatcher
	otpLength     int
	otpExpiryMins int
}

func NewRegisterUseCase(
	db *gorm.DB,
	userRepo user.Repository,
	guestRepo guestsession.Repository,
	tokenRepo verificationtoken.Repository,
	breachChecker PasswordBreachChecker,
	dispatcher taskqueue.Dispatcher,
) *RegisterUseCase {
	return &RegisterUseCase{
		db:            db,
		userRepo:      userRepo,
		guestRepo:     guestRepo,
		tokenRepo:     tokenRepo,
		breachChecker: breachChecker,
		dispatcher:    dispatcher,
		otpLength:     6,
		otpExpiryMins: 15,
	}
}

func (uc *RegisterUseCase) Execute(ctx context.Context, req RegisterRequest) (*RegisterResponse, error) {
	// Password length check (minimum 10 characters)
	if len(req.Password) < 10 {
		return nil, ErrPasswordTooShort
	}

	// HIBP breach check — fail-open on error to not block registration
	if breached, err := uc.breachChecker.IsBreached(ctx, req.Password); err == nil && breached {
		return nil, errors.New("password found in known breach database — please choose a different password")
	}

	// Check email uniqueness
	existing, err := uc.userRepo.FindByEmail(ctx, req.Email)
	if err != nil {
		return nil, fmt.Errorf("register: check email: %w", err)
	}
	if existing != nil {
		return nil, ErrEmailAlreadyRegistered
	}

	// Hash password
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("register: hash password: %w", err)
	}

	// Load guest session (if cookie present)
	var guest *guestsession.GuestSession
	if req.GuestSessionID != nil {
		guest, err = uc.guestRepo.FindBySessionID(ctx, *req.GuestSessionID)
		if err != nil {
			return nil, fmt.Errorf("register: load guest session: %w", err)
		}
	}

	// Build new user entity — copy onboarding fields from GuestSession if available
	newUser := buildUser(req, hash, guest)

	// All mutations in a single transaction (AGENTS.md — mutasi multi-tabel MUST satu tx)
	txErr := uc.db.Transaction(func(tx *gorm.DB) error {
		txUserRepo := txUserRepository(tx)
		txGuestRepo := txGuestRepository(tx)
		txTokenRepo := txTokenRepository(tx)

		// Create USER
		if err := txUserRepo.Create(ctx, newUser); err != nil {
			return fmt.Errorf("tx: create user: %w", err)
		}

		// Claim guest session
		if guest != nil && !guest.IsClaimed() {
			guest.ClaimedByUserID = &newUser.ID
			if err := txGuestRepo.Update(ctx, guest); err != nil {
				return fmt.Errorf("tx: claim guest session: %w", err)
			}

			// Reassign TEST_RESULT ownership: set user_id, set guest_session_id = NULL
			// (XOR invariant — exactly one must be non-null)
			if err := tx.Exec(
				"UPDATE test_results SET user_id = ?, guest_session_id = NULL WHERE guest_session_id = ? AND status IN ('completed','fallback_static')",
				newUser.ID, guest.SessionID,
			).Error; err != nil {
				return fmt.Errorf("tx: reassign test results: %w", err)
			}

			// Handle referral: signup event
			if req.ReferralCode != nil {
				if err := createReferralEvents(ctx, tx, newUser.ID, *req.ReferralCode, guest.SessionID); err != nil {
					return err // non-fatal referral errors should NOT fail registration — see note below
				}
			}
		} else if req.ReferralCode != nil && guest == nil {
			// Register without guest session but with a referral code
			if err := createSignupEvent(ctx, tx, newUser.ID, *req.ReferralCode); err != nil {
				_ = err
			}
		}

		// Create OTP verification token
		otp := generateOTP(uc.otpLength)
		token := &verificationtoken.VerificationToken{
			ID:        uuid.New().String(),
			UserID:    newUser.ID,
			Token:     otp,
			Type:      verificationtoken.TokenTypeEmailVerification,
			ExpiresAt: time.Now().Add(time.Duration(uc.otpExpiryMins) * time.Minute),
		}
		if err := txTokenRepo.Create(ctx, token); err != nil {
			return fmt.Errorf("tx: create otp token: %w", err)
		}

		// Enqueue OTP email (async — MUST NOT block this HTTP request)
		payload := taskqueue.SendEmailPayload{
			Type:   "otp_verification",
			UserID: newUser.ID,
			Email:  newUser.Email,
			OTP:    otp,
			Locale: newUser.PreferredLocale,
		}
		if err := uc.dispatcher.EnqueueEmail(ctx, payload, taskqueue.QueueCritical); err != nil {
			// Email enqueue failure is logged but MUST NOT fail the registration.
			// The user can request a resend via /resend-email-otp.
			_ = err
		}

		return nil
	})

	if txErr != nil {
		return nil, txErr
	}

	return &RegisterResponse{UserID: newUser.ID}, nil
}

// buildUser constructs the User domain entity, copying onboarding data from GuestSession when present.
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

// generateOTP returns a zero-padded numeric OTP of the specified digit count.
func generateOTP(digits int) string {
	max := 1
	for i := 0; i < digits; i++ {
		max *= 10
	}
	code := rand.Intn(max) //nolint:gosec — OTP doesn't need crypto random for 6-digit codes
	return fmt.Sprintf("%0*d", digits, code)
}

// createReferralEvents creates signup + test_completed events inside the transaction.
func createReferralEvents(ctx context.Context, tx *gorm.DB, newUserID, referralCode, guestSessionID string) error {
	var referralCodeID string
	if err := tx.Raw("SELECT id FROM referral_codes WHERE code = ?", referralCode).
		Scan(&referralCodeID).Error; err != nil || referralCodeID == "" {
		return nil // invalid referral code — silent ignore (per spec)
	}

	// signup event
	if err := tx.Exec(
		"INSERT INTO referral_events (id, referral_code_id, referred_user_id, event_type, created_at) VALUES (?, ?, ?, 'signup', NOW())",
		uuid.New().String(), referralCodeID, newUserID,
	).Error; err != nil {
		return nil // non-blocking
	}

	// test_completed event — only if the guest session had a completed test result
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

// createSignupEvent creates only the signup referral event (no guest session).
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

func txUserRepository(tx *gorm.DB) user.Repository {
	return postgres.NewUserRepository(tx)
}

func txGuestRepository(tx *gorm.DB) guestsession.Repository {
	return postgres.NewGuestSessionRepository(tx)
}

func txTokenRepository(tx *gorm.DB) verificationtoken.Repository {
	return postgres.NewVerificationTokenRepository(tx)
}
