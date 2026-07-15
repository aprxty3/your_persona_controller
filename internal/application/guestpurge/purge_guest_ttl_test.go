package guestpurge

import (
	"context"
	"errors"
	"testing"

	"github.com/aprxty3/your_persona_controller.git/internal/application/guestpurge/mocks"
	"github.com/aprxty3/your_persona_controller.git/internal/domain/account"
	accountmocks "github.com/aprxty3/your_persona_controller.git/internal/domain/account/mocks"
	"github.com/aprxty3/your_persona_controller.git/internal/domain/testresult"
	testresultmocks "github.com/aprxty3/your_persona_controller.git/internal/domain/testresult/mocks"
	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
	"github.com/stretchr/testify/mock"
)

func testLogger() logger.Logger { return logger.NewLogger("test") }

func TestExecute_FindExpiredError_Propagates(t *testing.T) {
	trRepo := testresultmocks.NewMockTestResultRepository(t)
	trRepo.EXPECT().FindExpiredGuestResults(mock.Anything).Return(nil, errors.New("db down")).Once()
	uc := NewPurgeGuestTTLUseCase(nil, trRepo, nil, nil, testLogger())

	if _, err := uc.Execute(context.Background()); err == nil {
		t.Fatal("expected the repository error to propagate")
	}
}

// Nothing expired and no orphans: the whole run completes without ever
// touching uc.db (which is nil in this test) — proving the empty-result path
// never reaches the per-row db.Transaction.
func TestExecute_NothingExpiredNoOrphans_ReturnsZero(t *testing.T) {
	trRepo := testresultmocks.NewMockTestResultRepository(t)
	trRepo.EXPECT().FindExpiredGuestResults(mock.Anything).Return(nil, nil).Once()
	guestRepo := accountmocks.NewMockGuestSessionRepository(t)
	guestRepo.EXPECT().FindExpiredUnclaimed(mock.Anything).Return(nil, nil).Once()
	uc := NewPurgeGuestTTLUseCase(nil, trRepo, guestRepo, nil, testLogger())

	purged, err := uc.Execute(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if purged != 0 {
		t.Fatalf("expected 0 purged, got %d", purged)
	}
}

// A PDF deletion failure must skip that row (continue) BEFORE reaching
// db.Transaction — the only way to unit-test this loop without a real DB.
func TestExecute_PDFDeleteFails_SkipsRowBeforeTransaction(t *testing.T) {
	pdfURL := "guest/sess-1/r1.pdf"
	trRepo := testresultmocks.NewMockTestResultRepository(t)
	trRepo.EXPECT().FindExpiredGuestResults(mock.Anything).Return([]testresult.TestResult{
		{ID: "r1", PDFUrl: &pdfURL},
	}, nil).Once()
	pdfStorage := mocks.NewMockPDFStorage(t)
	pdfStorage.EXPECT().DeleteByURL(mock.Anything, pdfURL).Return(errors.New("r2 down")).Once()
	guestRepo := accountmocks.NewMockGuestSessionRepository(t)
	guestRepo.EXPECT().FindExpiredUnclaimed(mock.Anything).Return(nil, nil).Once()
	// guestRepo has no EXPECT() for DeleteBySessionID: the row never made it
	// past the PDF failure, so no session should be scheduled for deletion.

	uc := NewPurgeGuestTTLUseCase(nil, trRepo, guestRepo, pdfStorage, testLogger())

	purged, err := uc.Execute(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if purged != 0 {
		t.Fatalf("expected 0 purged when the only row's PDF delete fails, got %d", purged)
	}
}

// --- purgeOrphanSessions: no db.Transaction involved, fully mockable ---

func TestPurgeOrphanSessions_FindError_ReturnsZero(t *testing.T) {
	guestRepo := accountmocks.NewMockGuestSessionRepository(t)
	guestRepo.EXPECT().FindExpiredUnclaimed(mock.Anything).Return(nil, errors.New("db down")).Once()
	uc := NewPurgeGuestTTLUseCase(nil, nil, guestRepo, nil, testLogger())

	if n := uc.purgeOrphanSessions(context.Background()); n != 0 {
		t.Fatalf("expected 0 on a find error, got %d", n)
	}
}

// Per-row skip-on-error: one failing delete must not stop the others from
// being purged.
func TestPurgeOrphanSessions_PerRowSkipOnError(t *testing.T) {
	guestRepo := accountmocks.NewMockGuestSessionRepository(t)
	guestRepo.EXPECT().FindExpiredUnclaimed(mock.Anything).Return([]account.GuestSession{
		{SessionID: "s1"},
		{SessionID: "s2"},
	}, nil).Once()
	guestRepo.EXPECT().DeleteBySessionID(mock.Anything, "s1").Return(errors.New("db down")).Once()
	guestRepo.EXPECT().DeleteBySessionID(mock.Anything, "s2").Return(nil).Once()
	uc := NewPurgeGuestTTLUseCase(nil, nil, guestRepo, nil, testLogger())

	n := uc.purgeOrphanSessions(context.Background())
	if n != 1 {
		t.Fatalf("expected 1 orphan purged (s2 survives s1's failure), got %d", n)
	}
}

func TestPurgeOrphanSessions_AllSucceed(t *testing.T) {
	guestRepo := accountmocks.NewMockGuestSessionRepository(t)
	guestRepo.EXPECT().FindExpiredUnclaimed(mock.Anything).Return([]account.GuestSession{
		{SessionID: "s1"}, {SessionID: "s2"},
	}, nil).Once()
	guestRepo.EXPECT().DeleteBySessionID(mock.Anything, "s1").Return(nil).Once()
	guestRepo.EXPECT().DeleteBySessionID(mock.Anything, "s2").Return(nil).Once()
	uc := NewPurgeGuestTTLUseCase(nil, nil, guestRepo, nil, testLogger())

	if n := uc.purgeOrphanSessions(context.Background()); n != 2 {
		t.Fatalf("expected 2 orphans purged, got %d", n)
	}
}
