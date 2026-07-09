package user

import (
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
