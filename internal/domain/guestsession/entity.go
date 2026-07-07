package guestsession

import (
	"context"
	"time"
)

// GuestSession holds the onboarding data for users who haven't registered yet.
type GuestSession struct {
	SessionID       string
	IPHash          string
	DisplayName     string
	Age             int
	Status          string
	ClaimedByUserID *string
	CreatedAt       time.Time
	ExpiresAt       time.Time
}

// Repository defines the contract for GuestSession data persistence.
type Repository interface {
	Create(ctx context.Context, session *GuestSession) error
	FindBySessionID(ctx context.Context, sessionID string) (*GuestSession, error)
	Update(ctx context.Context, session *GuestSession) error
}
