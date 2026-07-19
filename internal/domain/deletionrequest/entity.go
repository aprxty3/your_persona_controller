// Package deletionrequest is the domain package for UU PDP account-deletion
// requests and their anonymization grace-period lifecycle.
package deletionrequest

import (
	"time"
)

// DeletionStatus represents the lifecycle state of a data deletion request.
type DeletionStatus string

// The lifecycle states a deletion request moves through.
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
