package worker

import (
	"context"
	"encoding/json"
	"testing"

	apppdf "github.com/aprxty3/your_persona_controller.git/internal/application/pdf"
	testresultmocks "github.com/aprxty3/your_persona_controller.git/internal/domain/testresult/mocks"
	"github.com/aprxty3/your_persona_controller.git/pkg/taskqueue"
	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/mock"
)

func TestPDFProcessTask_MalformedPayload_ReturnsError(t *testing.T) {
	uc := apppdf.NewGeneratePDFUseCase(testresultmocks.NewMockTestResultRepository(t), nil, nil, nil, nil, nil, nil, nil, testLog())
	h := NewPDFHandler(uc, testLog())

	task := asynq.NewTask(taskqueue.TaskGeneratePDF, []byte("not json"))
	if err := h.ProcessTask(context.Background(), task); err == nil {
		t.Fatal("expected an error for malformed payload")
	}
}

func TestPDFProcessTask_TestResultNotFound_DroppedWithoutError(t *testing.T) {
	trRepo := testresultmocks.NewMockTestResultRepository(t)
	trRepo.EXPECT().FindByID(mock.Anything, "result-1").Return(nil, nil).Once()

	uc := apppdf.NewGeneratePDFUseCase(trRepo, nil, nil, nil, nil, nil, nil, nil, testLog())
	h := NewPDFHandler(uc, testLog())

	payload, err := json.Marshal(taskqueue.GeneratePDFPayload{TestResultID: "result-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	task := asynq.NewTask(taskqueue.TaskGeneratePDF, payload)

	if err := h.ProcessTask(context.Background(), task); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPDFProcessTask_RepoError_ReturnsError(t *testing.T) {
	trRepo := testresultmocks.NewMockTestResultRepository(t)
	trRepo.EXPECT().FindByID(mock.Anything, "result-1").Return(nil, assertErrWorker).Once()

	uc := apppdf.NewGeneratePDFUseCase(trRepo, nil, nil, nil, nil, nil, nil, nil, testLog())
	h := NewPDFHandler(uc, testLog())

	payload, err := json.Marshal(taskqueue.GeneratePDFPayload{TestResultID: "result-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	task := asynq.NewTask(taskqueue.TaskGeneratePDF, payload)

	if err := h.ProcessTask(context.Background(), task); err == nil {
		t.Fatal("expected an error to propagate for asynq retry")
	}
}

var assertErrWorker = &workerTestErr{}

type workerTestErr struct{}

func (e *workerTestErr) Error() string { return "boom" }
