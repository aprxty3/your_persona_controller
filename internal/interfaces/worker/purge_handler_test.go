package worker

import (
	"context"
	"testing"

	"github.com/aprxty3/your_persona_controller.git/internal/application/auditpurge"
	"github.com/aprxty3/your_persona_controller.git/internal/application/guestpurge"
	accountmocks "github.com/aprxty3/your_persona_controller.git/internal/domain/account/mocks"
	testresultmocks "github.com/aprxty3/your_persona_controller.git/internal/domain/testresult/mocks"
	"github.com/stretchr/testify/mock"
)

func TestProcessPurge_NoExpiredResults_NoError(t *testing.T) {
	trRepo := testresultmocks.NewMockTestResultRepository(t)
	trRepo.EXPECT().FindExpiredGuestResults(mock.Anything).Return(nil, nil).Once()
	guestRepo := accountmocks.NewMockGuestSessionRepository(t)
	guestRepo.EXPECT().FindExpiredUnclaimed(mock.Anything).Return(nil, nil).Once()

	guestUC := guestpurge.NewPurgeGuestTTLUseCase(nil, trRepo, guestRepo, nil, testLog())
	h := NewPurgeHandler(guestUC, nil, testLog())

	if err := h.ProcessPurge(context.Background(), nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProcessPurge_RepoError_ReturnsError(t *testing.T) {
	trRepo := testresultmocks.NewMockTestResultRepository(t)
	trRepo.EXPECT().FindExpiredGuestResults(mock.Anything).Return(nil, assertErrWorker).Once()

	guestUC := guestpurge.NewPurgeGuestTTLUseCase(nil, trRepo, nil, nil, testLog())
	h := NewPurgeHandler(guestUC, nil, testLog())

	if err := h.ProcessPurge(context.Background(), nil); err == nil {
		t.Fatal("expected an error to propagate for asynq retry")
	}
}

func TestProcessAuditPurge_Success_NoError(t *testing.T) {
	auditRepo := testresultmocks.NewMockPromptAuditLogRepository(t)
	auditRepo.EXPECT().DeleteExpired(mock.Anything).Return(int64(5), nil).Once()

	auditUC := auditpurge.NewPurgeAuditTTLUseCase(auditRepo, testLog())
	h := NewPurgeHandler(nil, auditUC, testLog())

	if err := h.ProcessAuditPurge(context.Background(), nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProcessAuditPurge_RepoError_ReturnsError(t *testing.T) {
	auditRepo := testresultmocks.NewMockPromptAuditLogRepository(t)
	auditRepo.EXPECT().DeleteExpired(mock.Anything).Return(int64(0), assertErrWorker).Once()

	auditUC := auditpurge.NewPurgeAuditTTLUseCase(auditRepo, testLog())
	h := NewPurgeHandler(nil, auditUC, testLog())

	if err := h.ProcessAuditPurge(context.Background(), nil); err == nil {
		t.Fatal("expected an error to propagate for asynq retry")
	}
}
