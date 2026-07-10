package account

import (
	"time"
)

// GuestSession holds the onboarding data for users who haven't registered yet.
// Created by POST /v1/guest-session before the assessment begins (FR-A8).
//
// SESSION LIFECYCLE:
// - Created on Onboarding form submit → set via httpOnly cookie.
// - Expires after 14 days (TTL mirrors TestResult.expires_at).
// - Claimed when Guest registers → claimed_by_user_id is set, data is copied to USER.
//
// PRIVACY: GuestSession is anonymized alongside USER data if the user
// later registers and then requests deletion (PRD Section 9.3).
type GuestSession struct {
	SessionID       string
	IPHash          string  // hashed IP for deduplication — never store raw IP
	DisplayName     string  // from Onboarding form (FR-A5)
	Age             int     // from Onboarding form (FR-A5)
	Status          string  // pelajar|mahasiswa|bekerja|lainnya (FR-A5)
	Locale          string  // Guest locale preference stored here (FR-I2)
	ClaimedByUserID *string // set when Guest → Member conversion happens (FR-F1)
	CreatedAt       time.Time
	ExpiresAt       time.Time // created_at + 14 days
}

// IsClaimed returns true if this guest session has been converted to a member account.
func (g *GuestSession) IsClaimed() bool {
	return g.ClaimedByUserID != nil
}

// IsExpired returns true if the session has passed its 14-day TTL.
func (g *GuestSession) IsExpired() bool {
	return time.Now().After(g.ExpiresAt)
}
