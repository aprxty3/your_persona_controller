package deletionrequest

import (
	"context"
	"errors"
	"testing"

	"github.com/aprxty3/your_persona_controller.git/internal/application/deletionrequest/mocks"
	"github.com/aprxty3/your_persona_controller.git/internal/domain/account"
	accountmocks "github.com/aprxty3/your_persona_controller.git/internal/domain/account/mocks"
	"github.com/aprxty3/your_persona_controller.git/internal/domain/deletionrequest"
	deletionmocks "github.com/aprxty3/your_persona_controller.git/internal/domain/deletionrequest/mocks"
	testresultmocks "github.com/aprxty3/your_persona_controller.git/internal/domain/testresult/mocks"
	"github.com/aprxty3/your_persona_controller.git/pkg/taskqueue"
	taskqueuemocks "github.com/aprxty3/your_persona_controller.git/pkg/taskqueue/mocks"
	"github.com/stretchr/testify/mock"
)

// --- ProcessExpired: fully mockable, no db.Transaction involved ---

func TestProcessExpired_FindError_Propagates(t *testing.T) {
	deleteRepo := deletionmocks.NewMockRepository(t)
	deleteRepo.EXPECT().FindExpiredGracePeriod(mock.Anything).Return(nil, errors.New("db down")).Once()
	uc := NewAnonymizeUseCase(nil, deleteRepo, nil, nil, nil, nil, nil, testLogger())

	if _, err := uc.ProcessExpired(context.Background()); err == nil {
		t.Fatal("expected the repository error to propagate")
	}
}

func TestProcessExpired_NoneExpired_ReturnsZero(t *testing.T) {
	deleteRepo := deletionmocks.NewMockRepository(t)
	deleteRepo.EXPECT().FindExpiredGracePeriod(mock.Anything).Return(nil, nil).Once()
	uc := NewAnonymizeUseCase(nil, deleteRepo, nil, nil, nil, nil, nil, testLogger())

	n, err := uc.ProcessExpired(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 enqueued, got %d", n)
	}
}

func TestProcessExpired_EnqueuesAndMarksProcessing(t *testing.T) {
	deleteRepo := deletionmocks.NewMockRepository(t)
	deleteRepo.EXPECT().FindExpiredGracePeriod(mock.Anything).Return([]deletionrequest.DataDeletionRequest{
		{ID: "req-1", UserID: "user-1"},
	}, nil).Once()
	deleteRepo.EXPECT().TransitionStatus(mock.Anything, "req-1", deletionrequest.StatusPendingGrace, deletionrequest.StatusProcessing, mock.Anything).Return(true, nil).Once()
	dispatcher := taskqueuemocks.NewMockDispatcher(t)
	dispatcher.EXPECT().EnqueueAnonymize(mock.Anything, taskqueue.AnonymizeUserPayload{UserID: "user-1", DeletionRequestID: "req-1"}).Return(nil).Once()
	uc := NewAnonymizeUseCase(nil, deleteRepo, nil, nil, nil, nil, dispatcher, testLogger())

	n, err := uc.ProcessExpired(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 enqueued, got %d", n)
	}
}

// A per-row enqueue failure must skip that row (no status transition, no
// count) but not abort the whole scan — a later tick retries it.
func TestProcessExpired_EnqueueFailure_SkipsRowWithoutTransition(t *testing.T) {
	deleteRepo := deletionmocks.NewMockRepository(t)
	deleteRepo.EXPECT().FindExpiredGracePeriod(mock.Anything).Return([]deletionrequest.DataDeletionRequest{
		{ID: "req-1", UserID: "user-1"},
	}, nil).Once()
	dispatcher := taskqueuemocks.NewMockDispatcher(t)
	dispatcher.EXPECT().EnqueueAnonymize(mock.Anything, mock.Anything).Return(errors.New("redis down")).Once()
	uc := NewAnonymizeUseCase(nil, deleteRepo, nil, nil, nil, nil, dispatcher, testLogger())
	// deleteRepo has no EXPECT() for TransitionStatus — asserting it's never called.

	n, err := uc.ProcessExpired(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 enqueued after the failure, got %d", n)
	}
}

// --- Anonymize: dropped-request short circuits, all pre-transaction ---

func TestAnonymize_RequestNotFound_DroppedSilently(t *testing.T) {
	deleteRepo := deletionmocks.NewMockRepository(t)
	deleteRepo.EXPECT().FindByID(mock.Anything, "req-1").Return(nil, nil).Once()
	uc := NewAnonymizeUseCase(nil, deleteRepo, nil, nil, nil, nil, nil, testLogger())

	if err := uc.Anonymize(context.Background(), AnonymizeRequest{DeletionRequestID: "req-1"}); err != nil {
		t.Fatalf("expected a silent drop, got error: %v", err)
	}
}

func TestAnonymize_RequestCancelled_DroppedSilently(t *testing.T) {
	deleteRepo := deletionmocks.NewMockRepository(t)
	deleteRepo.EXPECT().FindByID(mock.Anything, "req-1").Return(&deletionrequest.DataDeletionRequest{ID: "req-1", Status: deletionrequest.StatusCancelled}, nil).Once()
	uc := NewAnonymizeUseCase(nil, deleteRepo, nil, nil, nil, nil, nil, testLogger())

	if err := uc.Anonymize(context.Background(), AnonymizeRequest{DeletionRequestID: "req-1"}); err != nil {
		t.Fatalf("expected a silent drop for a cancelled request, got error: %v", err)
	}
}

func TestAnonymize_AlreadyCompleted_SkippedSilently(t *testing.T) {
	deleteRepo := deletionmocks.NewMockRepository(t)
	deleteRepo.EXPECT().FindByID(mock.Anything, "req-1").Return(&deletionrequest.DataDeletionRequest{ID: "req-1", Status: deletionrequest.StatusCompleted}, nil).Once()
	uc := NewAnonymizeUseCase(nil, deleteRepo, nil, nil, nil, nil, nil, testLogger())

	if err := uc.Anonymize(context.Background(), AnonymizeRequest{DeletionRequestID: "req-1"}); err != nil {
		t.Fatalf("expected a silent skip for an already-completed request, got error: %v", err)
	}
}

// A PDF deletion failure must abort BEFORE the DB scrub transaction —
// verified here since it's exactly the boundary this unit test can reach
// without a real Postgres connection (the transaction body constructs
// concrete pg repos directly, out of reach for mocks).
func TestAnonymize_PDFDeleteFails_AbortsBeforeTransaction(t *testing.T) {
	deleteRepo := deletionmocks.NewMockRepository(t)
	deleteRepo.EXPECT().FindByID(mock.Anything, "req-1").Return(&deletionrequest.DataDeletionRequest{ID: "req-1", UserID: "user-1", Status: deletionrequest.StatusProcessing}, nil).Once()
	userRepo := accountmocks.NewMockUserRepository(t)
	userRepo.EXPECT().FindByID(mock.Anything, "user-1").Return(&account.User{ID: "user-1", PreferredLocale: "en"}, nil).Once()
	testResultRepo := testresultmocks.NewMockTestResultRepository(t)
	testResultRepo.EXPECT().FindPDFURLsByUser(mock.Anything, "user-1").Return([]string{"guest/x/r1.pdf"}, nil).Once()
	pdfStorage := mocks.NewMockPDFStorage(t)
	pdfStorage.EXPECT().DeleteByURL(mock.Anything, "guest/x/r1.pdf").Return(errors.New("r2 down")).Once()

	uc := NewAnonymizeUseCase(nil, deleteRepo, userRepo, nil, testResultRepo, pdfStorage, nil, testLogger())

	err := uc.Anonymize(context.Background(), AnonymizeRequest{DeletionRequestID: "req-1"})
	if err == nil {
		t.Fatal("expected the PDF deletion error to abort before the DB transaction")
	}
}
