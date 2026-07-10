package account

import (
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
