package user

import (
	"context"
	"time"
)

// Repository defines the contract for User data persistence.
type Repository interface {
	Create(ctx context.Context, user *User) error
	FindByID(ctx context.Context, id string) (*User, error)
	FindByEmail(ctx context.Context, email string) (*User, error)
	Update(ctx context.Context, user *User) error
	IncrementTokenVersion(ctx context.Context, id string) error
	UpdateLoginAttempt(ctx context.Context, id string, failedCount int, lockedUntil *time.Time) error
}
