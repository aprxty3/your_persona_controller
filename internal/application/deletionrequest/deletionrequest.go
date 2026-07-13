package deletionrequest

import (
	"context"
	"fmt"
	"time"

	"github.com/aprxty3/your_persona_controller.git/internal/application"
	"github.com/aprxty3/your_persona_controller.git/internal/domain/account"
	"github.com/aprxty3/your_persona_controller.git/internal/domain/deletionrequest"
	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
	"github.com/google/uuid"
)

// RequestDeletionResponse reflects the newly created request.
type RequestDeletionResponse struct {
	RequestID   string    `json:"request_id"`
	Status      string    `json:"status"`
	RequestedAt time.Time `json:"requested_at"`
}

// DeletionUseCase manages the deletion-request grace period lifecycle.
type DeletionUseCase struct {
	userRepo   account.UserRepository
	deleteRepo deletionrequest.Repository
	log        logger.Logger
}

// NewDeletionUseCase creates a new DeletionUseCase.
func NewDeletionUseCase(userRepo account.UserRepository, deleteRepo deletionrequest.Repository, log logger.Logger) *DeletionUseCase {
	return &DeletionUseCase{
		userRepo:   userRepo,
		deleteRepo: deleteRepo,
		log:        log.With("usecase", "deletionrequest"),
	}
}

// RequestDeletion starts the 14-day grace period.
func (uc *DeletionUseCase) RequestDeletion(ctx context.Context, userID string) (*RequestDeletionResponse, error) {
	existing, err := uc.deleteRepo.FindActiveByUserID(ctx, userID)
	if err != nil {
		uc.log.Error("request deletion failed", "step", "check_existing", "user_id", userID, "error", err)
		return nil, fmt.Errorf("request_deletion: check existing: %w", err)
	}
	if existing != nil {
		uc.log.Warn("request deletion rejected", "reason", "already_requested", "user_id", userID)
		return nil, application.ErrDeletionAlreadyRequested
	}

	u, err := uc.userRepo.FindByID(ctx, userID)
	if err != nil {
		uc.log.Error("request deletion failed", "step", "lookup_user", "error", err)
		return nil, fmt.Errorf("request_deletion: lookup user: %w", err)
	}
	if u == nil {
		uc.log.Error("request deletion failed", "reason", "user_not_found_after_auth", "user_id", userID)
		return nil, fmt.Errorf("request_deletion: user %s not found", userID)
	}

	req := &deletionrequest.DataDeletionRequest{
		ID:                uuid.New().String(),
		UserID:            userID,
		NotificationEmail: u.Email,
		Status:            deletionrequest.StatusPendingGrace,
		RequestedAt:       time.Now(),
	}
	if err := uc.deleteRepo.Create(ctx, req); err != nil {
		uc.log.Error("request deletion failed", "step", "persist", "user_id", userID, "error", err)
		return nil, fmt.Errorf("request_deletion: %w", err)
	}

	uc.log.Info("deletion requested", "user_id", userID, "request_id", req.ID)
	return &RequestDeletionResponse{
		RequestID:   req.ID,
		Status:      string(req.Status),
		RequestedAt: req.RequestedAt,
	}, nil
}

// CancelDeletion stops the grace period before the anonymization worker runs.
func (uc *DeletionUseCase) CancelDeletion(ctx context.Context, userID string) error {
	existing, err := uc.deleteRepo.FindActiveByUserID(ctx, userID)
	if err != nil {
		uc.log.Error("cancel deletion failed", "step", "check_existing", "user_id", userID, "error", err)
		return fmt.Errorf("cancel_deletion: check existing: %w", err)
	}
	if existing == nil {
		uc.log.Warn("cancel deletion rejected", "reason", "no_active_request", "user_id", userID)
		return application.ErrNoActiveDeletionRequest
	}
	if existing.Status != deletionrequest.StatusPendingGrace {
		uc.log.Warn("cancel deletion rejected", "reason", "already_processing", "user_id", userID)
		return application.ErrDeletionAlreadyProcessing
	}

	if err := uc.deleteRepo.UpdateStatus(ctx, existing.ID, deletionrequest.StatusCancelled, nil); err != nil {
		uc.log.Error("cancel deletion failed", "step", "update_status", "user_id", userID, "error", err)
		return fmt.Errorf("cancel_deletion: %w", err)
	}

	uc.log.Info("deletion cancelled", "user_id", userID, "request_id", existing.ID)
	return nil
}
