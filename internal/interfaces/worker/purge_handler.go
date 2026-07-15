package worker

import (
	"context"

	"github.com/aprxty3/your_persona_controller.git/internal/application/auditpurge"
	"github.com/aprxty3/your_persona_controller.git/internal/application/guestpurge"
	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
	"github.com/hibiken/asynq"
)

// PurgeHandler processes the parameterless cron-triggered retention sweeps:
// purge:guest-ttl and purge:audit-ttl.
type PurgeHandler struct {
	guestUC *guestpurge.PurgeGuestTTLUseCase
	auditUC *auditpurge.PurgeAuditTTLUseCase
	log     logger.Logger
}

// NewPurgeHandler constructs a new PurgeHandler.
func NewPurgeHandler(guestUC *guestpurge.PurgeGuestTTLUseCase, auditUC *auditpurge.PurgeAuditTTLUseCase, log logger.Logger) *PurgeHandler {
	return &PurgeHandler{guestUC: guestUC, auditUC: auditUC, log: log.With("worker", "purge")}
}

// ProcessPurge handles purge:guest-ttl.
func (h *PurgeHandler) ProcessPurge(ctx context.Context, _ *asynq.Task) error {
	if _, err := h.guestUC.Execute(ctx); err != nil {
		h.log.Error("guest ttl purge failed", "error", err)
		return err
	}
	return nil
}

// ProcessAuditPurge handles purge:audit-ttl.
func (h *PurgeHandler) ProcessAuditPurge(ctx context.Context, _ *asynq.Task) error {
	if _, err := h.auditUC.Execute(ctx); err != nil {
		h.log.Error("audit ttl purge failed", "error", err)
		return err
	}
	return nil
}
