package promptauditlog

import (
	"context"
)

// Repository defines the contract for PromptAuditLog data persistence.
type Repository interface {
	// Create inserts a new audit log entry.
	Create(ctx context.Context, log *PromptAuditLog) error

	// DeleteByTestResultID hard-deletes all audit logs for a given test result.
	// Used by the anonymization worker (PRD Section 9.3) and Guest TTL purge.
	DeleteByTestResultID(ctx context.Context, testResultID string) error

	// DeleteExpired removes all records where expires_at < now().
	// Called by the scheduled purge job to enforce 30-day retention.
	DeleteExpired(ctx context.Context) error
}
