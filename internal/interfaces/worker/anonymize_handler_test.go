package worker

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	appdeletion "github.com/aprxty3/your_persona_controller.git/internal/application/deletionrequest"
	"github.com/aprxty3/your_persona_controller.git/internal/domain/deletionrequest"
	deletiondomainmocks "github.com/aprxty3/your_persona_controller.git/internal/domain/deletionrequest/mocks"
	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
	"github.com/aprxty3/your_persona_controller.git/pkg/taskqueue"
	taskqueuemocks "github.com/aprxty3/your_persona_controller.git/pkg/taskqueue/mocks"
	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/mock"
)

func testLog() logger.Logger { return logger.NewLogger("test") }

func TestProcessScan_NoExpiredRequests_NoJobsEnqueued(t *testing.T) {
	deleteRepo := deletiondomainmocks.NewMockRepository(t)
	deleteRepo.EXPECT().FindExpiredGracePeriod(mock.Anything).Return(nil, nil).Once()

	uc := appdeletion.NewAnonymizeUseCase(nil, deleteRepo, nil, nil, nil, nil, nil, testLog())
	h := NewAnonymizeHandler(uc, testLog())

	if err := h.ProcessScan(context.Background(), nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProcessScan_ExpiredRequest_EnqueuesAndTransitions(t *testing.T) {
	deleteRepo := deletiondomainmocks.NewMockRepository(t)
	deleteRepo.EXPECT().FindExpiredGracePeriod(mock.Anything).
		Return([]deletionrequest.DataDeletionRequest{{ID: "req-1", UserID: "user-1"}}, nil).Once()
	dispatcher := taskqueuemocks.NewMockDispatcher(t)
	dispatcher.EXPECT().EnqueueAnonymize(mock.Anything, taskqueue.AnonymizeUserPayload{UserID: "user-1", DeletionRequestID: "req-1"}).Return(nil).Once()
	deleteRepo.EXPECT().TransitionStatus(mock.Anything, "req-1", deletionrequest.StatusPendingGrace, deletionrequest.StatusProcessing, mock.Anything).
		Return(true, nil).Once()

	uc := appdeletion.NewAnonymizeUseCase(nil, deleteRepo, nil, nil, nil, nil, dispatcher, testLog())
	h := NewAnonymizeHandler(uc, testLog())

	if err := h.ProcessScan(context.Background(), nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProcessScan_RepoError_ReturnsError(t *testing.T) {
	deleteRepo := deletiondomainmocks.NewMockRepository(t)
	deleteRepo.EXPECT().FindExpiredGracePeriod(mock.Anything).Return(nil, errors.New("db down")).Once()

	uc := appdeletion.NewAnonymizeUseCase(nil, deleteRepo, nil, nil, nil, nil, nil, testLog())
	h := NewAnonymizeHandler(uc, testLog())

	if err := h.ProcessScan(context.Background(), nil); err == nil {
		t.Fatal("expected an error to propagate for asynq retry")
	}
}

func newAnonymizeTask(t *testing.T, payload taskqueue.AnonymizeUserPayload) *asynq.Task {
	t.Helper()
	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return asynq.NewTask(taskqueue.TaskAnonymize, b)
}

func TestProcessAnonymize_MalformedPayload_ReturnsError(t *testing.T) {
	uc := appdeletion.NewAnonymizeUseCase(nil, deletiondomainmocks.NewMockRepository(t), nil, nil, nil, nil, nil, testLog())
	h := NewAnonymizeHandler(uc, testLog())

	task := asynq.NewTask(taskqueue.TaskAnonymize, []byte("not json"))
	if err := h.ProcessAnonymize(context.Background(), task); err == nil {
		t.Fatal("expected an error for malformed payload")
	}
}

func TestProcessAnonymize_RequestNotFound_DroppedWithoutError(t *testing.T) {
	deleteRepo := deletiondomainmocks.NewMockRepository(t)
	deleteRepo.EXPECT().FindByID(mock.Anything, "req-1").Return(nil, nil).Once()

	uc := appdeletion.NewAnonymizeUseCase(nil, deleteRepo, nil, nil, nil, nil, nil, testLog())
	h := NewAnonymizeHandler(uc, testLog())
	task := newAnonymizeTask(t, taskqueue.AnonymizeUserPayload{UserID: "user-1", DeletionRequestID: "req-1"})

	if err := h.ProcessAnonymize(context.Background(), task); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProcessAnonymize_AlreadyCancelled_DroppedWithoutError(t *testing.T) {
	deleteRepo := deletiondomainmocks.NewMockRepository(t)
	deleteRepo.EXPECT().FindByID(mock.Anything, "req-1").
		Return(&deletionrequest.DataDeletionRequest{ID: "req-1", UserID: "user-1", Status: deletionrequest.StatusCancelled}, nil).Once()

	uc := appdeletion.NewAnonymizeUseCase(nil, deleteRepo, nil, nil, nil, nil, nil, testLog())
	h := NewAnonymizeHandler(uc, testLog())
	task := newAnonymizeTask(t, taskqueue.AnonymizeUserPayload{UserID: "user-1", DeletionRequestID: "req-1"})

	if err := h.ProcessAnonymize(context.Background(), task); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProcessAnonymize_AlreadyCompleted_SkippedWithoutError(t *testing.T) {
	deleteRepo := deletiondomainmocks.NewMockRepository(t)
	deleteRepo.EXPECT().FindByID(mock.Anything, "req-1").
		Return(&deletionrequest.DataDeletionRequest{ID: "req-1", UserID: "user-1", Status: deletionrequest.StatusCompleted}, nil).Once()

	uc := appdeletion.NewAnonymizeUseCase(nil, deleteRepo, nil, nil, nil, nil, nil, testLog())
	h := NewAnonymizeHandler(uc, testLog())
	task := newAnonymizeTask(t, taskqueue.AnonymizeUserPayload{UserID: "user-1", DeletionRequestID: "req-1"})

	if err := h.ProcessAnonymize(context.Background(), task); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
