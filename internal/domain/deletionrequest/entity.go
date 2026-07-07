package deletionrequest

import (
	"context"
	"time"
)

// DeletionStatus represents the lifecycle state of a data deletion request.
type DeletionStatus string

const (
	// StatusPendingGrace is the initial state. The grace period (14 days) is active.
	// The user can cancel during this window (FR-G2a).
	StatusPendingGrace DeletionStatus = "pending_grace"

	// StatusProcessing is set when the anonymization worker begins execution.
	StatusProcessing DeletionStatus = "processing"

	// StatusCompleted is set after anonymization and R2 file deletion finish successfully.
	// A confirmation email is sent to notification_email.
	StatusCompleted DeletionStatus = "completed"

	// StatusCancelled is set when the user clicks "Cancel Deletion" during grace period.
	StatusCancelled DeletionStatus = "cancelled"
)

// DataDeletionRequest records a user's formal request to erase their personal data,
// as required by UU PDP (Indonesian Personal Data Protection Law).
//
// Deletion is NOT immediate — it follows a 14-day grace period, after which the
// anonymization worker obscures PII (email, name, essay answers, AI summary, PDF files).
// Aggregate data (mbti_type, grit_score) is retained. See PRD Section 9.3.
//
// notification_email is snapshot from USER.email at request time, because USER.email
// will be anonymized and can no longer be used for the confirmation email.
type DataDeletionRequest struct {
	ID                string
	UserID            string
	NotificationEmail string // snapshot of USER.email — captured before anonymization
	Status            DeletionStatus
	RequestedAt       time.Time
	CompletedAt       *time.Time // nil until processing finishes
}

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
