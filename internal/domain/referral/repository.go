package referral

import (
	"context"
)

// Repository defines the contract for ReferralCode data persistence.
type Repository interface {
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
