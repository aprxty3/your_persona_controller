package deletionrequest

import (
	"context"
	"time"
)

// Repository defines the contract for DataDeletionRequest data persistence.
type Repository interface {
	Create(ctx context.Context, req *DataDeletionRequest) error
	FindActiveByUserID(ctx context.Context, userID string) (*DataDeletionRequest, error)
	UpdateStatus(ctx context.Context, id string, status DeletionStatus, completedAt *time.Time) error
	FindExpiredGracePeriod(ctx context.Context) ([]DataDeletionRequest, error)
}
