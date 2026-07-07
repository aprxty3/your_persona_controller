package user

import (
	"context"
	"time"
)

// User represents the core business entity for a registered member.
type User struct {
	ID              string
	Email           string
	PasswordHash    string
	DisplayName     string
	Age             int
	Status          string  // student|undergraduate|employed|others
	ReferredByCode  *string // FK to REFERRAL_CODE.code — nullable
	PreferredLocale string  // default "en"
	EmailVerifiedAt *time.Time
	CreatedAt       time.Time
	DeletedAt       *time.Time
	AnonymizedAt    *time.Time
	TokenVersion    int

	FailedLoginCount int
	LockedUntil      *time.Time // nil = not locked
}

// IsEmailVerified returns true if the user has verified their email.
func (u *User) IsEmailVerified() bool {
	return u.EmailVerifiedAt != nil
}

// IsLocked returns true if the account is currently under a lockout penalty.
func (u *User) IsLocked() bool {
	if u.LockedUntil == nil {
		return false
	}
	return time.Now().Before(*u.LockedUntil)
}

// Repository defines the contract for User data persistence.
type Repository interface {
	Create(ctx context.Context, user *User) error
	FindByID(ctx context.Context, id string) (*User, error)

	FindByEmail(ctx context.Context, email string) (*User, error)

	Update(ctx context.Context, user *User) error

	IncrementTokenVersion(ctx context.Context, id string) error

	UpdateLoginAttempt(ctx context.Context, id string, failedCount int, lockedUntil *time.Time) error
}
