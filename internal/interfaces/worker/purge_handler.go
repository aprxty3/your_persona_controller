package worker

import (
	"context"

	"github.com/aprxty3/your_persona_controller.git/internal/application/guestpurge"
	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
	"github.com/hibiken/asynq"
)

// PurgeHandler processes purge:guest-ttl cron jobs.
type PurgeHandler struct {
	uc  *guestpurge.PurgeGuestTTLUseCase
	log logger.Logger
}

// NewPurgeHandler constructs a new PurgeHandler.
func NewPurgeHandler(uc *guestpurge.PurgeGuestTTLUseCase, log logger.Logger) *PurgeHandler {
	return &PurgeHandler{uc: uc, log: log.With("worker", "purge")}
}

func (h *PurgeHandler) ProcessPurge(ctx context.Context, _ *asynq.Task) error {
	if _, err := h.uc.Execute(ctx); err != nil {
		h.log.Error("guest ttl purge failed", "error", err)
		return err
	}
	return nil
}
