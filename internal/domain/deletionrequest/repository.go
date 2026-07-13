package deletionrequest

import (
	"context"
	"time"
)

// Repository defines the contract for DataDeletionRequest data persistence.
type Repository interface {
	Create(ctx context.Context, req *DataDeletionRequest) error
	FindByID(ctx context.Context, id string) (*DataDeletionRequest, error)
	FindActiveByUserID(ctx context.Context, userID string) (*DataDeletionRequest, error)
	UpdateStatus(ctx context.Context, id string, status DeletionStatus, completedAt *time.Time) error
	TransitionStatus(ctx context.Context, id string, from, to DeletionStatus, completedAt *time.Time) (bool, error)
	FindExpiredGracePeriod(ctx context.Context) ([]DataDeletionRequest, error)
}
