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

func (r *PromptAuditLogRepository) DeleteExpired(ctx context.Context) error {
	err := r.db.WithContext(ctx).
		Where("expires_at < ?", time.Now()).
		Delete(&postgres.PromptAuditLogModel{}).Error

	return postgres.LogQueryError(r.log, "DeleteExpired", err)
}
