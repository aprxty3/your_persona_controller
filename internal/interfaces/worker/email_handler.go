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

// EmailHandler processes send:email background jobs.
type EmailHandler struct {
	mailer mailer.Mailer
	log    logger.Logger
}

// NewEmailHandler constructs a new EmailHandler.
func NewEmailHandler(m mailer.Mailer, log logger.Logger) *EmailHandler {
	return &EmailHandler{mailer: m, log: log.With("worker", "email")}
}

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
	case "deletion_confirmed":
		if err := h.mailer.SendDeletionConfirmed(ctx, payload.Email, payload.Locale); err != nil {
			h.log.Error("send deletion confirmation failed", "user_id", payload.UserID, "error", err)
			return err
		}
	default:
		h.log.Warn("unhandled email type, skipping", "type", payload.Type, "user_id", payload.UserID)
		return nil
	}

	h.log.Info("email sent", "type", payload.Type, "user_id", payload.UserID)
	return nil
}
