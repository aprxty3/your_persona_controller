package asynq

import (
	"context"

	"github.com/aprxty3/your_persona_controller.git/internal/application/assessment"
	"github.com/aprxty3/your_persona_controller.git/pkg/taskqueue"
)

// PDFQueueService adapts taskqueue.Dispatcher to assessment.PDFQueueService —
// the real (non-stub) enqueuer for generate:pdf jobs.
type PDFQueueService struct {
	dispatcher taskqueue.Dispatcher
}

// NewPDFQueueService creates a new PDFQueueService.
func NewPDFQueueService(dispatcher taskqueue.Dispatcher) assessment.PDFQueueService {
	return &PDFQueueService{dispatcher: dispatcher}
}

// EnqueueGeneratePDF enqueues a generate:pdf task for testResultID.
func (s *PDFQueueService) EnqueueGeneratePDF(ctx context.Context, testResultID string) error {
	return s.dispatcher.EnqueuePDFGeneration(ctx, taskqueue.GeneratePDFPayload{TestResultID: testResultID})
}
