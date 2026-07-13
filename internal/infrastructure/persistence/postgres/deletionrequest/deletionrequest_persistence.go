package deletionrequest

import (
	"context"
	"errors"
	"time"

	"github.com/aprxty3/your_persona_controller.git/internal/domain/deletionrequest"
	"github.com/aprxty3/your_persona_controller.git/internal/infrastructure/persistence/postgres"
	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
	"gorm.io/gorm"
)

// Repository implements deletionrequest.Repository backed by PostgreSQL via GORM.
type Repository struct {
	db  *gorm.DB
	log logger.Logger
}

// NewRepository constructs a new deletion request Repository.
func NewRepository(db *gorm.DB, log logger.Logger) deletionrequest.Repository {
	return &Repository{db: db, log: log.With("repository", "deletionrequest")}
}

// Create inserts a new deletion request (initial status: pending_grace).
func (r *Repository) Create(ctx context.Context, req *deletionrequest.DataDeletionRequest) error {
	m := toModel(req)
	if err := r.db.WithContext(ctx).Create(&m).Error; err != nil {
		r.log.Error("query failed", "op", "Create", "error", err)
		return err
	}
	return nil
}

// FindActiveByUserID returns the most recent request still in a non-terminal
// state (pending_grace or processing), or nil if none exists.
func (r *Repository) FindActiveByUserID(ctx context.Context, userID string) (*deletionrequest.DataDeletionRequest, error) {
	var m postgres.DataDeletionRequestModel
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND status IN ?", userID, []string{string(deletionrequest.StatusPendingGrace), string(deletionrequest.StatusProcessing)}).
		Order("requested_at DESC").
		First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		r.log.Error("query failed", "op", "FindActiveByUserID", "error", err)
		return nil, err
	}
	entity := toEntity(&m)
	return &entity, nil
}

// UpdateStatus changes the status of a request.
func (r *Repository) UpdateStatus(ctx context.Context, id string, status deletionrequest.DeletionStatus, completedAt *time.Time) error {
	updates := map[string]interface{}{
		"status":       string(status),
		"completed_at": completedAt,
	}
	err := r.db.WithContext(ctx).
		Model(&postgres.DataDeletionRequestModel{}).
		Where("id = ?", id).
		Updates(updates).
		Error
	if err != nil {
		r.log.Error("query failed", "op", "UpdateStatus", "error", err)
	}
	return err
}

// FindExpiredGracePeriod returns all pending_grace requests older than the grace period.
func (r *Repository) FindExpiredGracePeriod(ctx context.Context) ([]deletionrequest.DataDeletionRequest, error) {
	const gracePeriod = 14 * 24 * time.Hour

	var models []postgres.DataDeletionRequestModel
	err := r.db.WithContext(ctx).
		Where("status = ? AND requested_at <= ?", string(deletionrequest.StatusPendingGrace), time.Now().Add(-gracePeriod)).
		Find(&models).Error
	if err != nil {
		r.log.Error("query failed", "op", "FindExpiredGracePeriod", "error", err)
		return nil, err
	}

	entities := make([]deletionrequest.DataDeletionRequest, len(models))
	for i, m := range models {
		entities[i] = toEntity(&m)
	}
	return entities, nil
}

func toModel(req *deletionrequest.DataDeletionRequest) postgres.DataDeletionRequestModel {
	return postgres.DataDeletionRequestModel{
		ID:                req.ID,
		UserID:            req.UserID,
		NotificationEmail: req.NotificationEmail,
		Status:            string(req.Status),
		RequestedAt:       req.RequestedAt,
		CompletedAt:       req.CompletedAt,
	}
}

func toEntity(m *postgres.DataDeletionRequestModel) deletionrequest.DataDeletionRequest {
	return deletionrequest.DataDeletionRequest{
		ID:                m.ID,
		UserID:            m.UserID,
		NotificationEmail: m.NotificationEmail,
		Status:            deletionrequest.DeletionStatus(m.Status),
		RequestedAt:       m.RequestedAt,
		CompletedAt:       m.CompletedAt,
	}
}
