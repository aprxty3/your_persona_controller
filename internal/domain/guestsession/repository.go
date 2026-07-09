package guestsession

import (
	"context"
)

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
