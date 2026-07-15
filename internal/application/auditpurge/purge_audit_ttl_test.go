package auditpurge

import (
	"context"
	"errors"
	"testing"

	testresultmocks "github.com/aprxty3/your_persona_controller.git/internal/domain/testresult/mocks"
	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
	"github.com/stretchr/testify/mock"
)

func testLogger() logger.Logger { return logger.NewLogger("test") }

func TestExecute_NoExpiredRows_ReturnsZero(t *testing.T) {
	repo := testresultmocks.NewMockPromptAuditLogRepository(t)
	repo.EXPECT().DeleteExpired(mock.Anything).Return(int64(0), nil).Once()
	uc := NewPurgeAuditTTLUseCase(repo, testLogger())

	deleted, err := uc.Execute(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if deleted != 0 {
		t.Fatalf("expected 0 rows deleted, got %d", deleted)
	}
}

func TestExecute_ExpiredRowsDeleted_ReturnsCount(t *testing.T) {
	repo := testresultmocks.NewMockPromptAuditLogRepository(t)
	repo.EXPECT().DeleteExpired(mock.Anything).Return(int64(7), nil).Once()
	uc := NewPurgeAuditTTLUseCase(repo, testLogger())

	deleted, err := uc.Execute(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if deleted != 7 {
		t.Fatalf("expected 7 rows deleted, got %d", deleted)
	}
}

func TestExecute_RepoError_Propagates(t *testing.T) {
	repo := testresultmocks.NewMockPromptAuditLogRepository(t)
	repo.EXPECT().DeleteExpired(mock.Anything).Return(int64(0), errors.New("db down")).Once()
	uc := NewPurgeAuditTTLUseCase(repo, testLogger())

	if _, err := uc.Execute(context.Background()); err == nil {
		t.Fatal("expected the repository error to propagate so Asynq retries")
	}
}
