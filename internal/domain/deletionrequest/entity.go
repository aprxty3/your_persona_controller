package deletionrequest

import (
	"time"
)

// DeletionStatus represents the lifecycle state of a data deletion request.
type DeletionStatus string

const (
	StatusPendingGrace DeletionStatus = "pending_grace"
	StatusProcessing   DeletionStatus = "processing"
	StatusCompleted    DeletionStatus = "completed"
	StatusCancelled    DeletionStatus = "cancelled"
)

// DataDeletionRequest records a user's formal request to erase their personal data.
type DataDeletionRequest struct {
	ID                string
	UserID            string
	NotificationEmail string
	Status            DeletionStatus
	RequestedAt       time.Time
	CompletedAt       *time.Time
}
