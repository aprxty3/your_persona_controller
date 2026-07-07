package user

import (
	"context"
	"time"
)

// User represents the core business entity for a registered member.
//
// SECURITY NOTES (per AGENTS.md):
//   - Data sensitif dibatasi ke email saja — JANGAN tambahkan field nomor telepon/HP.
//   - TokenVersion di-embed sebagai JWT claim; token dengan versi lama otomatis ditolak.
//   - FailedLoginCount & LockedUntil dipakai untuk account-level lockout (FR-H3):
//     10x gagal berturut → lock 15 menit. Ini TERPISAH dari rate-limit per-IP (FR-H6).
type User struct {
	ID              string
	Email           string
	PasswordHash    string
	DisplayName     string
	Age             int
	Status          string  // pelajar|mahasiswa|bekerja|lainnya
	ReferredByCode  *string // FK ke REFERRAL_CODE.code — nullable
	PreferredLocale string  // default "en"
	EmailVerifiedAt *time.Time
	CreatedAt       time.Time
	DeletedAt       *time.Time
	AnonymizedAt    *time.Time
	TokenVersion    int // default 0; increment on reset-password / logout-all

	// Auth lockout fields (account-level, not IP-level — FR-H3)
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
	// Create inserts a new user record.
	Create(ctx context.Context, user *User) error

	// FindByID retrieves a user by their UUID.
	FindByID(ctx context.Context, id string) (*User, error)

	// FindByEmail retrieves a user by email address. Returns nil, nil if not found.
	FindByEmail(ctx context.Context, email string) (*User, error)

	// Update saves all mutable fields of the user.
	Update(ctx context.Context, user *User) error

	// IncrementTokenVersion atomically increments token_version for the given user ID.
	// Used by: reset-password (FR-H4), logout-all (FR-H8).
	IncrementTokenVersion(ctx context.Context, id string) error

	// UpdateLoginAttempt updates the failed login counter and lockout timestamp.
	// Set failedCount=0, lockedUntil=nil on successful login.
	UpdateLoginAttempt(ctx context.Context, id string, failedCount int, lockedUntil *time.Time) error
}
