package verificationtoken

import (
	"context"
	"time"
)

// TokenType represents the purpose of a verification token.
type TokenType string

const (
	TokenTypeEmailVerification TokenType = "email_verification"
	TokenTypePasswordReset     TokenType = "password_reset"
)

// VerificationToken represents a short-lived OTP code used for email verification
// or password reset. The token field holds a numeric OTP (e.g. 6 digits).
//
// SECURITY NOTES (per ERD & AGENTS.md):
//   - Lookup MUST be scoped to (user_id, type) — never by token alone, because
//     a 6-digit OTP is not globally unique across users.
//   - attempt_count prevents brute-force of the 1,000,000-combination space (FR-H2a).
//   - Each new OTP generation MUST expire-force all previous unused tokens for the
//     same (user_id, type) pair — max 1 valid token per user per type at any time.
type VerificationToken struct {
	ID           string
	UserID       string
	Token        string    // 6-digit numeric OTP
	Type         TokenType
	AttemptCount int       // incremented on each wrong guess; reject after 5 (FR-H2a)
	ExpiresAt    time.Time
	UsedAt       *time.Time // nil = not yet consumed
	CreatedAt    time.Time
}

// Repository defines the contract for VerificationToken data persistence.
type Repository interface {
	// Create inserts a new token record.
	Create(ctx context.Context, token *VerificationToken) error

	// FindActiveByUserAndType returns the single active (not expired, not used) token
	// for the given user and type. Returns nil, nil when none exists.
	FindActiveByUserAndType(ctx context.Context, userID string, tokenType TokenType) (*VerificationToken, error)

	// IncrementAttemptCount increments attempt_count for the given token ID.
	IncrementAttemptCount(ctx context.Context, tokenID string) error

	// MarkUsed sets used_at to now for the given token ID.
	MarkUsed(ctx context.Context, tokenID string) error

	// ExpireAllActiveForUser sets expires_at = NOW() for all unused tokens of the
	// given (user_id, type) pair. MUST be called before creating a new token to
	// ensure at most one valid token exists per user per type.
	ExpireAllActiveForUser(ctx context.Context, userID string, tokenType TokenType) error
}