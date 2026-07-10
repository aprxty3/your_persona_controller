package account

import (
	"time"
)

// GuestSession holds the onboarding data for users who haven't registered yet.
type GuestSession struct {
	SessionID       string
	IPHash          string
	DisplayName     string
	Age             int
	Status          string
	Locale          string
	ClaimedByUserID *string
	CreatedAt       time.Time
	ExpiresAt       time.Time
}

// IsClaimed returns true if this guest session has been converted to a member account.
func (g *GuestSession) IsClaimed() bool {
	return g.ClaimedByUserID != nil
}

// IsExpired returns true if the session has passed its 14-day TTL.
func (g *GuestSession) IsExpired() bool {
	return time.Now().After(g.ExpiresAt)
}
