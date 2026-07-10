package verificationtoken

import (
	"context"
)

// Repository defines the contract for VerificationToken data persistence.
type Repository interface {
	Create(ctx context.Context, token *VerificationToken) error
	FindActiveByUserAndType(ctx context.Context, userID string, tokenType TokenType) (*VerificationToken, error)
	IncrementAttemptCount(ctx context.Context, tokenID string) error
	MarkUsed(ctx context.Context, tokenID string) error
	ExpireAllActiveForUser(ctx context.Context, userID string, tokenType TokenType) error
}
