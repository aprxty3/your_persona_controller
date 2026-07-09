package verificationtoken

import (
	"context"
)

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
