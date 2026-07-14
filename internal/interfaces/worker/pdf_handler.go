package worker

import (
	"context"
	"encoding/json"
	"fmt"

	apppdf "github.com/aprxty3/your_persona_controller.git/internal/application/pdf"
	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
	"github.com/aprxty3/your_persona_controller.git/pkg/taskqueue"
	"github.com/hibiken/asynq"
)

// PDFHandler processes generate:pdf background jobs.
type PDFHandler struct {
	uc  *apppdf.GeneratePDFUseCase
	log logger.Logger
}

// NewPDFHandler constructs a new PDFHandler.
func NewPDFHandler(uc *apppdf.GeneratePDFUseCase, log logger.Logger) *PDFHandler {
	return &PDFHandler{uc: uc, log: log.With("worker", "pdf")}
}

func (h *PDFHandler) ProcessTask(ctx context.Context, t *asynq.Task) error {
	var payload taskqueue.GeneratePDFPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("pdf worker: unmarshal payload: %w", err)
	}
	return h.uc.Execute(ctx, payload.TestResultID)
}
