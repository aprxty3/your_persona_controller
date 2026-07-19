// Package worker implements the Asynq task handlers the background worker
// process dispatches to — one file per task family (email, PDF, anonymize, purge).
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

// AnonymizeHandler processes the deletion:scan-expired and anonymize:user tasks.
type AnonymizeHandler struct {
	uc  *appdeletion.AnonymizeUseCase
	log logger.Logger
}

// NewAnonymizeHandler constructs a new AnonymizeHandler.
func NewAnonymizeHandler(uc *appdeletion.AnonymizeUseCase, log logger.Logger) *AnonymizeHandler {
	return &AnonymizeHandler{uc: uc, log: log.With("worker", "anonymize")}
}

// ProcessScan handles the deletion:scan-expired task — anonymizes every
// deletion request whose grace period has elapsed.
func (h *AnonymizeHandler) ProcessScan(ctx context.Context, _ *asynq.Task) error {
	if _, err := h.uc.ProcessExpired(ctx); err != nil {
		h.log.Error("deletion scan failed", "error", err)
		return err
	}
	return nil
}

// ProcessAnonymize handles the anonymize:user task for a single deletion request.
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
