package referral

import (
	"context"
	"time"
)

// ReferralEventType represents the event that triggered a referral record.
type ReferralEventType string

const (
	EventTypeSignup        ReferralEventType = "signup"
	EventTypeTestCompleted ReferralEventType = "test_completed"
)

// ReferralCode is the unique code owned by each Member, used for viral acquisition.
// One user owns exactly one code.
type ReferralCode struct {
	ID        string
	UserID    string
	Code      string // Unique alphanumeric code, e.g. "ABCD1234"
	CreatedAt time.Time
}

// ReferralEvent records a conversion triggered by a specific referral code.
// event_type="signup"         → a new user registered using this code.
// event_type="test_completed" → the referred user completed their first test (FR-G1).
//
// PRIVACY NOTE (per TECHNICAL_DOCUMENTATION.md Section 5.5):
// Data returned to the code owner UI MUST be aggregated or masked — never expose
// the referred user's email or full name (UU PDP compliance).
type ReferralEvent struct {
	ID             string
	ReferralCodeID string
	ReferredUserID string
	EventType      ReferralEventType
	CreatedAt      time.Time
}

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