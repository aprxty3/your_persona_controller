package asynq

import (
	"context"
	"errors"
	"testing"

	"github.com/aprxty3/your_persona_controller.git/pkg/taskqueue"
	"github.com/aprxty3/your_persona_controller.git/pkg/taskqueue/mocks"
	"github.com/stretchr/testify/mock"
)

func TestEnqueueGeneratePDF_DelegatesToDispatcherWithExpectedPayload(t *testing.T) {
	dispatcher := mocks.NewMockDispatcher(t)
	dispatcher.EXPECT().
		EnqueuePDFGeneration(mock.Anything, taskqueue.GeneratePDFPayload{TestResultID: "result-1"}).
		Return(nil).
		Once()

	svc := NewPDFQueueService(dispatcher)

	err := svc.EnqueueGeneratePDF(context.Background(), "result-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEnqueueGeneratePDF_PropagatesDispatcherError(t *testing.T) {
	dispatcher := mocks.NewMockDispatcher(t)
	sentinelErr := errors.New("asynq: redis unavailable")
	dispatcher.EXPECT().
		EnqueuePDFGeneration(mock.Anything, taskqueue.GeneratePDFPayload{TestResultID: "result-2"}).
		Return(sentinelErr).
		Once()

	svc := NewPDFQueueService(dispatcher)

	err := svc.EnqueueGeneratePDF(context.Background(), "result-2")
	if !errors.Is(err, sentinelErr) {
		t.Fatalf("expected sentinel error to propagate, got: %v", err)
	}
}
