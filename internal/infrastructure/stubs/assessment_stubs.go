package stubs

import (
	"context"

	"github.com/aprxty3/your_persona_controller.git/internal/application/assessment"
)

// StubPDFQueueService implements assessment.PDFQueueService
type StubPDFQueueService struct{}

func NewStubPDFQueueService() assessment.PDFQueueService {
	return &StubPDFQueueService{}
}

func (s *StubPDFQueueService) EnqueueGeneratePDF(ctx context.Context, testResultID string) error {
	return nil
}
