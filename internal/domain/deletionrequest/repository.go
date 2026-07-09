package deletionrequest

import (
	"context"
	"time"
)

// Repository defines the contract for DataDeletionRequest data persistence.
type Repository interface {
	// Create inserts a new deletion request (initial status: pending_grace).
	Create(ctx context.Context, req *DataDeletionRequest) error

	// FindActiveByUserID returns the most recent non-cancelled request for a user,
	// or nil if none exists. Used to prevent duplicate requests.
	FindActiveByUserID(ctx context.Context, userID string) (*DataDeletionRequest, error)

	// UpdateStatus changes the status of a request.
	// Used by: Cancel (→ cancelled), Worker start (→ processing), Worker done (→ completed).
	UpdateStatus(ctx context.Context, id string, status DeletionStatus, completedAt *time.Time) error

	// FindExpiredGracePeriod returns all requests with status=pending_grace
	// whose grace period (14 days) has passed. Used by the scheduled worker
	// to trigger anonymization.
	FindExpiredGracePeriod(ctx context.Context) ([]DataDeletionRequest, error)
}
