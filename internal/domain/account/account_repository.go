package account

import (
	"context"
	"time"
)

// UserRepository defines the contract for User data persistence.
type UserRepository interface {
	Create(ctx context.Context, user *User) error
	FindByID(ctx context.Context, id string) (*User, error)
	FindByEmail(ctx context.Context, email string) (*User, error)
	Update(ctx context.Context, user *User) error
	IncrementTokenVersion(ctx context.Context, id string) error
	UpdateLoginAttempt(ctx context.Context, id string, failedCount int, lockedUntil *time.Time) error
}

// GuestSessionRepository defines the contract for GuestSession data persistence.
type GuestSessionRepository interface {
	// Create inserts a new guest session.
	Create(ctx context.Context, session *GuestSession) error

	// FindBySessionID retrieves a session by its UUID.
	// Returns nil, nil when not found.
	FindBySessionID(ctx context.Context, sessionID string) (*GuestSession, error)

	// Update saves all mutable fields of the session.
	// Used when claiming: set claimed_by_user_id.
	Update(ctx context.Context, session *GuestSession) error

	// FindExpiredUnclaimed returns all sessions past expires_at that have not been
	// claimed. Used by the daily Guest TTL purge job (FR-G5, PRD Section 9.6).
	FindExpiredUnclaimed(ctx context.Context) ([]GuestSession, error)

	// DeleteBySessionID hard-deletes a session record. Called after R2 PDF cleanup
	// by the purge job (order: delete R2 objects → delete DB rows).
	DeleteBySessionID(ctx context.Context, sessionID string) error
}

// ReferralRepository defines the contract for ReferralCode data persistence.
type ReferralRepository interface {
	// CreateCode creates a new referral code for a user.
	CreateCode(ctx context.Context, code *ReferralCode) error

	// FindCodeByUserID returns the referral code owned by the given user, or nil if none.
	FindCodeByUserID(ctx context.Context, userID string) (*ReferralCode, error)

	// FindCodeByCode looks up a referral code by its alphanumeric string value.
	// Used during registration to validate a referral link.
	FindCodeByCode(ctx context.Context, code string) (*ReferralCode, error)

	// CreateEvent records a new referral event (signup or test_completed).
	CreateEvent(ctx context.Context, event *ReferralEvent) error

	// CountEventsByCodeID counts total referral events for reporting (aggregated, not individual).
	CountEventsByCodeID(ctx context.Context, referralCodeID string, eventType ReferralEventType) (int64, error)
}

// VerificationTokenRepository defines the contract for VerificationToken data persistence.
type VerificationTokenRepository interface {
	// Create inserts a new token record.
	Create(ctx context.Context, token *VerificationToken) error

	// FindActiveByUserAndType returns the single active (not expired, not used) token
	// for the given user and type. Returns nil, nil when none exists.
	FindActiveByUserAndType(ctx context.Context, userID string, tokenType TokenType) (*VerificationToken, error)

	// IncrementAttemptCount increments attempt_count for the given token ID.
	IncrementAttemptCount(ctx context.Context, tokenID string) error

	// MarkUsed sets used_at to now for the given token ID.
	MarkUsed(ctx context.Context, tokenID string) error

	// ExpireAllActiveForUser sets expires_at = NOW() for all unused tokens of the
	// given (user_id, type) pair. MUST be called before creating a new token to
	// ensure at most one valid token exists per user per type.
	ExpireAllActiveForUser(ctx context.Context, userID string, tokenType TokenType) error
}
