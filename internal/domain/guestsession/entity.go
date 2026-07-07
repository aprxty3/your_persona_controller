package guestsession

import (
	"context"
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

// Repository defines the contract for GuestSession data persistence.
type Repository interface {
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
