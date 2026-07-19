package account

import (
	"time"
)

// ReferralEventType represents the event that triggered a referral record.
type ReferralEventType string

// The two referral event types that can be recorded against a referral code.
const (
	EventTypeSignup        ReferralEventType = "signup"
	EventTypeTestCompleted ReferralEventType = "test_completed"
)

// ReferralCode is the unique code owned by each Member, used for viral acquisition.
type ReferralCode struct {
	ID        string
	UserID    string
	Code      string
	CreatedAt time.Time
}

// ReferralEvent records a conversion triggered by a specific referral code.
type ReferralEvent struct {
	ID             string
	ReferralCodeID string
	ReferredUserID string
	EventType      ReferralEventType
	CreatedAt      time.Time
}
