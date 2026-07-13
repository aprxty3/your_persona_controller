package worker

import (
	"context"
	"encoding/json"
	"fmt"

	appdeletion "github.com/aprxty3/your_persona_controller.git/internal/application/deletionrequest"
	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
	"github.com/aprxty3/your_persona_controller.git/pkg/taskqueue"
	"github.com/hibiken/asynq"
)

// AnonymizeHandler processes
type AnonymizeHandler struct {
	uc  *appdeletion.AnonymizeUseCase
	log logger.Logger
}

// NewAnonymizeHandler constructs a new AnonymizeHandler.
func NewAnonymizeHandler(uc *appdeletion.AnonymizeUseCase, log logger.Logger) *AnonymizeHandler {
	return &AnonymizeHandler{uc: uc, log: log.With("worker", "anonymize")}
}

func (h *AnonymizeHandler) ProcessScan(ctx context.Context, _ *asynq.Task) error {
	if _, err := h.uc.ProcessExpired(ctx); err != nil {
		h.log.Error("deletion scan failed", "error", err)
		return err
	}
	return nil
}

func (h *AnonymizeHandler) ProcessAnonymize(ctx context.Context, t *asynq.Task) error {
	var payload taskqueue.AnonymizeUserPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("anonymize worker: unmarshal payload: %w", err)
	}

	return h.uc.Anonymize(ctx, appdeletion.AnonymizeRequest{
		UserID:            payload.UserID,
		DeletionRequestID: payload.DeletionRequestID,
	})
}
