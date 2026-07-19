// Package auditpurge implements the scheduled purge of expired prompt audit logs.
package auditpurge

import (
	"context"
	"fmt"

	"github.com/aprxty3/your_persona_controller.git/internal/domain/testresult"
	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
)

// PurgeAuditTTLUseCase implements the daily prompt-audit-log retention sweep:
// PROMPT_AUDIT_LOG rows carry raw Gemini prompts/responses and are promised a
// 30-day TTL — this job is what actually keeps that promise.
type PurgeAuditTTLUseCase struct {
	auditLogRepo testresult.PromptAuditLogRepository
	log          logger.Logger
}

// NewPurgeAuditTTLUseCase creates a new PurgeAuditTTLUseCase.
func NewPurgeAuditTTLUseCase(auditLogRepo testresult.PromptAuditLogRepository, log logger.Logger) *PurgeAuditTTLUseCase {
	return &PurgeAuditTTLUseCase{
		auditLogRepo: auditLogRepo,
		log:          log.With("usecase", "audit_ttl_purge"),
	}
}

// Execute deletes every prompt audit log past its expires_at. Errors are
// returned as-is so Asynq retries the run; the single DELETE is naturally
// idempotent.
func (uc *PurgeAuditTTLUseCase) Execute(ctx context.Context) (int64, error) {
	deleted, err := uc.auditLogRepo.DeleteExpired(ctx)
	if err != nil {
		return 0, fmt.Errorf("audit_ttl_purge: delete expired: %w", err)
	}
	if deleted > 0 {
		uc.log.Info("audit ttl purge done", "rows_deleted", deleted)
	}
	return deleted, nil
}
