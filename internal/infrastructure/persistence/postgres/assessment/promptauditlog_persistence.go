package assessment

import (
	"context"
	"time"

	"github.com/aprxty3/your_persona_controller.git/internal/domain/testresult"
	"github.com/aprxty3/your_persona_controller.git/internal/infrastructure/persistence/postgres"
	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
	"gorm.io/gorm"
)

type PromptAuditLogRepository struct {
	db  *gorm.DB
	log logger.Logger
}

func NewPromptAuditLogRepository(db *gorm.DB, log logger.Logger) testresult.PromptAuditLogRepository {
	return &PromptAuditLogRepository{
		db:  db,
		log: log.With("repository", "promptauditlog"),
	}
}

func toPromptAuditLogModel(entity *testresult.PromptAuditLog) postgres.PromptAuditLogModel {
	return postgres.PromptAuditLogModel{
		ID:             entity.ID,
		TestResultID:   entity.TestResultID,
		RawPrompt:      entity.RawPrompt,
		RawResponse:    entity.RawResponse,
		FlaggedAnomaly: entity.FlaggedAnomaly,
		CreatedAt:      entity.CreatedAt,
		ExpiresAt:      entity.ExpiresAt,
	}
}

func (r *PromptAuditLogRepository) Create(ctx context.Context, log *testresult.PromptAuditLog) error {
	m := toPromptAuditLogModel(log)
	return postgres.LogQueryError(r.log, "Create", r.db.WithContext(ctx).Create(&m).Error)
}

func (r *PromptAuditLogRepository) DeleteByTestResultID(ctx context.Context, testResultID string) error {
	err := r.db.WithContext(ctx).
		Where("test_result_id = ?", testResultID).
		Delete(&postgres.PromptAuditLogModel{}).Error

	return postgres.LogQueryError(r.log, "DeleteByTestResultID", err)
}

func (r *PromptAuditLogRepository) DeleteExpired(ctx context.Context) (int64, error) {
	res := r.db.WithContext(ctx).
		Where("expires_at < ?", time.Now()).
		Delete(&postgres.PromptAuditLogModel{})

	if err := postgres.LogQueryError(r.log, "DeleteExpired", res.Error); err != nil {
		return 0, err
	}
	return res.RowsAffected, nil
}

// DeleteByUserID removes every prompt audit log tied to any test result
// owned by userID. The table has no user_id column of its own, so
// ownership is resolved via a subquery through test_results.
func (r *PromptAuditLogRepository) DeleteByUserID(ctx context.Context, userID string) error {
	userResultIDs := r.db.Model(&postgres.TestResultModel{}).
		Select("id").
		Where("user_id = ?", userID)

	err := r.db.WithContext(ctx).
		Where("test_result_id IN (?)", userResultIDs).
		Delete(&postgres.PromptAuditLogModel{}).Error

	return postgres.LogQueryError(r.log, "DeleteByUserID", err)
}
