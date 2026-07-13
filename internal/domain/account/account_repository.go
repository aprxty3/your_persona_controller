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
	Anonymize(ctx context.Context, id string, scrubbedEmail string) error
}

// GuestSessionRepository defines the contract for GuestSession data persistence.
type GuestSessionRepository interface {
	Create(ctx context.Context, session *GuestSession) error
	FindBySessionID(ctx context.Context, sessionID string) (*GuestSession, error)
	Update(ctx context.Context, session *GuestSession) error
	FindExpiredUnclaimed(ctx context.Context) ([]GuestSession, error)
	DeleteBySessionID(ctx context.Context, sessionID string) error
	AnonymizeClaimedByUser(ctx context.Context, userID string) error
}

// ReferralRepository defines the contract for ReferralCode data persistence.
type ReferralRepository interface {
	CreateCode(ctx context.Context, code *ReferralCode) error
	FindCodeByUserID(ctx context.Context, userID string) (*ReferralCode, error)
	FindCodeByCode(ctx context.Context, code string) (*ReferralCode, error)
	CreateEvent(ctx context.Context, event *ReferralEvent) error
	CountEventsByCodeID(ctx context.Context, referralCodeID string, eventType ReferralEventType) (int64, error)
}

// VerificationTokenRepository defines the contract for VerificationToken data persistence.
type VerificationTokenRepository interface {
	Create(ctx context.Context, token *VerificationToken) error
	FindActiveByUserAndType(ctx context.Context, userID string, tokenType TokenType) (*VerificationToken, error)
	IncrementAttemptCount(ctx context.Context, tokenID string) error
	MarkUsed(ctx context.Context, tokenID string) error
	ExpireAllActiveForUser(ctx context.Context, userID string, tokenType TokenType) error
}
