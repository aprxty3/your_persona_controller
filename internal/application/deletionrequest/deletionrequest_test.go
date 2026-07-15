package deletionrequest

import (
	"context"
	"errors"
	"testing"

	"github.com/aprxty3/your_persona_controller.git/internal/application"
	"github.com/aprxty3/your_persona_controller.git/internal/domain/account"
	accountmocks "github.com/aprxty3/your_persona_controller.git/internal/domain/account/mocks"
	"github.com/aprxty3/your_persona_controller.git/internal/domain/deletionrequest"
	deletionmocks "github.com/aprxty3/your_persona_controller.git/internal/domain/deletionrequest/mocks"
	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
	"github.com/stretchr/testify/mock"
	"gorm.io/gorm"
)

func testLogger() logger.Logger { return logger.NewLogger("test") }

// --- RequestDeletion ---

func TestRequestDeletion_AlreadyActive_Rejected(t *testing.T) {
	deleteRepo := deletionmocks.NewMockRepository(t)
	deleteRepo.EXPECT().FindActiveByUserID(mock.Anything, "user-1").Return(&deletionrequest.DataDeletionRequest{ID: "existing"}, nil).Once()
	uc := NewDeletionUseCase(nil, deleteRepo, testLogger())

	_, err := uc.RequestDeletion(context.Background(), "user-1")
	if !errors.Is(err, application.ErrDeletionAlreadyRequested) {
		t.Fatalf("expected ErrDeletionAlreadyRequested, got %v", err)
	}
}

func TestRequestDeletion_UserNotFound_Errors(t *testing.T) {
	deleteRepo := deletionmocks.NewMockRepository(t)
	deleteRepo.EXPECT().FindActiveByUserID(mock.Anything, "user-1").Return(nil, nil).Once()
	userRepo := accountmocks.NewMockUserRepository(t)
	userRepo.EXPECT().FindByID(mock.Anything, "user-1").Return(nil, nil).Once()
	uc := NewDeletionUseCase(userRepo, deleteRepo, testLogger())

	_, err := uc.RequestDeletion(context.Background(), "user-1")
	if err == nil {
		t.Fatal("expected an error when the authenticated user can't be found")
	}
}

func TestRequestDeletion_ConcurrentDuplicate_TranslatedToAlreadyRequested(t *testing.T) {
	deleteRepo := deletionmocks.NewMockRepository(t)
	deleteRepo.EXPECT().FindActiveByUserID(mock.Anything, "user-1").Return(nil, nil).Once()
	deleteRepo.EXPECT().Create(mock.Anything, mock.Anything).Return(gorm.ErrDuplicatedKey).Once()
	userRepo := accountmocks.NewMockUserRepository(t)
	userRepo.EXPECT().FindByID(mock.Anything, "user-1").Return(&account.User{ID: "user-1", Email: "a@example.com"}, nil).Once()
	uc := NewDeletionUseCase(userRepo, deleteRepo, testLogger())

	_, err := uc.RequestDeletion(context.Background(), "user-1")
	if !errors.Is(err, application.ErrDeletionAlreadyRequested) {
		t.Fatalf("expected the partial-unique-index race to translate to ErrDeletionAlreadyRequested, got %v", err)
	}
}

func TestRequestDeletion_Success(t *testing.T) {
	deleteRepo := deletionmocks.NewMockRepository(t)
	deleteRepo.EXPECT().FindActiveByUserID(mock.Anything, "user-1").Return(nil, nil).Once()
	deleteRepo.EXPECT().Create(mock.Anything, mock.MatchedBy(func(r *deletionrequest.DataDeletionRequest) bool {
		return r.UserID == "user-1" && r.Status == deletionrequest.StatusPendingGrace && r.NotificationEmail == "a@example.com"
	})).Return(nil).Once()
	userRepo := accountmocks.NewMockUserRepository(t)
	userRepo.EXPECT().FindByID(mock.Anything, "user-1").Return(&account.User{ID: "user-1", Email: "a@example.com"}, nil).Once()
	uc := NewDeletionUseCase(userRepo, deleteRepo, testLogger())

	resp, err := uc.RequestDeletion(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Status != string(deletionrequest.StatusPendingGrace) {
		t.Fatalf("expected pending_grace status, got %s", resp.Status)
	}
}

// --- CancelDeletion ---

func TestCancelDeletion_NoActiveRequest_Rejected(t *testing.T) {
	deleteRepo := deletionmocks.NewMockRepository(t)
	deleteRepo.EXPECT().FindActiveByUserID(mock.Anything, "user-1").Return(nil, nil).Once()
	uc := NewDeletionUseCase(nil, deleteRepo, testLogger())

	err := uc.CancelDeletion(context.Background(), "user-1")
	if !errors.Is(err, application.ErrNoActiveDeletionRequest) {
		t.Fatalf("expected ErrNoActiveDeletionRequest, got %v", err)
	}
}

func TestCancelDeletion_AlreadyProcessing_Rejected(t *testing.T) {
	deleteRepo := deletionmocks.NewMockRepository(t)
	deleteRepo.EXPECT().FindActiveByUserID(mock.Anything, "user-1").Return(&deletionrequest.DataDeletionRequest{ID: "req-1", Status: deletionrequest.StatusProcessing}, nil).Once()
	uc := NewDeletionUseCase(nil, deleteRepo, testLogger())

	err := uc.CancelDeletion(context.Background(), "user-1")
	if !errors.Is(err, application.ErrDeletionAlreadyProcessing) {
		t.Fatalf("expected ErrDeletionAlreadyProcessing once grace period has moved to processing, got %v", err)
	}
}

// TransitionStatus's compare-and-swap losing (moved=false) means the grace
// period elapsed and the worker already flipped it to processing concurrently.
func TestCancelDeletion_ConcurrentTransition_LostRace_Rejected(t *testing.T) {
	deleteRepo := deletionmocks.NewMockRepository(t)
	deleteRepo.EXPECT().FindActiveByUserID(mock.Anything, "user-1").Return(&deletionrequest.DataDeletionRequest{ID: "req-1", Status: deletionrequest.StatusPendingGrace}, nil).Once()
	deleteRepo.EXPECT().TransitionStatus(mock.Anything, "req-1", deletionrequest.StatusPendingGrace, deletionrequest.StatusCancelled, mock.Anything).Return(false, nil).Once()
	uc := NewDeletionUseCase(nil, deleteRepo, testLogger())

	err := uc.CancelDeletion(context.Background(), "user-1")
	if !errors.Is(err, application.ErrDeletionAlreadyProcessing) {
		t.Fatalf("expected ErrDeletionAlreadyProcessing when the CAS transition loses, got %v", err)
	}
}

func TestCancelDeletion_Success(t *testing.T) {
	deleteRepo := deletionmocks.NewMockRepository(t)
	deleteRepo.EXPECT().FindActiveByUserID(mock.Anything, "user-1").Return(&deletionrequest.DataDeletionRequest{ID: "req-1", Status: deletionrequest.StatusPendingGrace}, nil).Once()
	deleteRepo.EXPECT().TransitionStatus(mock.Anything, "req-1", deletionrequest.StatusPendingGrace, deletionrequest.StatusCancelled, mock.Anything).Return(true, nil).Once()
	uc := NewDeletionUseCase(nil, deleteRepo, testLogger())

	if err := uc.CancelDeletion(context.Background(), "user-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
