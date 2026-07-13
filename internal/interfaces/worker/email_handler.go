package worker

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aprxty3/your_persona_controller.git/internal/infrastructure/mailer"
	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
	"github.com/aprxty3/your_persona_controller.git/pkg/taskqueue"
	"github.com/hibiken/asynq"
)

// EmailHandler processes send:email background jobs (FR-H2, FR-H4, FR-H5) —
// the consuming side of taskqueue.Dispatcher.EnqueueEmail.
type EmailHandler struct {
	mailer mailer.Mailer
	log    logger.Logger
}

// NewEmailHandler constructs a new EmailHandler.
func NewEmailHandler(m mailer.Mailer, log logger.Logger) *EmailHandler {
	return &EmailHandler{mailer: m, log: log.With("worker", "email")}
}

// ProcessTask handles a single send:email Asynq task. Returning a non-nil
// error triggers Asynq's built-in retry — see taskqueue.enqueue for retry config.
func (h *EmailHandler) ProcessTask(ctx context.Context, t *asynq.Task) error {
	var payload taskqueue.SendEmailPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("email worker: unmarshal payload: %w", err)
	}

	switch payload.Type {
	case "otp_verification", "otp_reset":
		if err := h.mailer.SendOTP(ctx, payload.Email, payload.OTP, payload.Type, payload.Locale); err != nil {
			h.log.Error("send otp email failed", "type", payload.Type, "user_id", payload.UserID, "error", err)
			return err
		}
	default:
		// Unknown/not-yet-implemented email type (e.g. deletion_confirmed) —
		// log and drop rather than retry forever on a type that will never resolve.
		h.log.Warn("unhandled email type, skipping", "type", payload.Type, "user_id", payload.UserID)
		return nil
	}

	h.log.Info("email sent", "type", payload.Type, "user_id", payload.UserID)
	return nil
}
