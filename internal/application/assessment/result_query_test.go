package assessment

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aprxty3/your_persona_controller.git/internal/application"
	"github.com/aprxty3/your_persona_controller.git/internal/application/assessment/mocks"
	"github.com/aprxty3/your_persona_controller.git/internal/domain/testresult"
	"github.com/stretchr/testify/mock"
)

func TestGetByID_NotFound_404(t *testing.T) {
	repo := mocks.NewMockResultRepository(t)
	repo.EXPECT().FindByID(mock.Anything, "r1").Return(nil, nil).Once()
	uc := NewResultUseCase(repo, nil, testLogger())

	_, err := uc.GetByID(context.Background(), "r1", "", "")
	if !errors.Is(err, application.ErrResultNotFound) {
		t.Fatalf("expected ErrResultNotFound, got %v", err)
	}
}

// Retention rule (PRD 9.6): a row past its expires_at is 404 regardless of
// whether the daily purge has physically deleted it yet.
func TestGetByID_Expired_404(t *testing.T) {
	past := time.Now().Add(-1 * time.Hour)
	repo := mocks.NewMockResultRepository(t)
	repo.EXPECT().FindByID(mock.Anything, "r1").Return(&testresult.TestResult{ID: "r1", ExpiresAt: &past}, nil).Once()
	uc := NewResultUseCase(repo, nil, testLogger())

	_, err := uc.GetByID(context.Background(), "r1", "", "")
	if !errors.Is(err, application.ErrResultNotFound) {
		t.Fatalf("expected ErrResultNotFound for an expired result, got %v", err)
	}
}

// Any caller holding the unguessable UUID may view a non-expired result —
// is_owner just flags whether it's the FE's teaser/full render.
func TestGetByID_NonOwnerCanStillView_IsOwnerFalse(t *testing.T) {
	userID := "owner-1"
	repo := mocks.NewMockResultRepository(t)
	repo.EXPECT().FindByID(mock.Anything, "r1").Return(&testresult.TestResult{ID: "r1", UserID: &userID}, nil).Once()
	uc := NewResultUseCase(repo, nil, testLogger())

	resp, err := uc.GetByID(context.Background(), "r1", "someone-else", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.IsOwner {
		t.Fatal("expected IsOwner=false for a non-matching caller")
	}
}

func TestGetByID_Owner_IsOwnerTrue(t *testing.T) {
	userID := "owner-1"
	repo := mocks.NewMockResultRepository(t)
	repo.EXPECT().FindByID(mock.Anything, "r1").Return(&testresult.TestResult{ID: "r1", UserID: &userID}, nil).Once()
	uc := NewResultUseCase(repo, nil, testLogger())

	resp, err := uc.GetByID(context.Background(), "r1", "owner-1", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.IsOwner {
		t.Fatal("expected IsOwner=true for a matching caller")
	}
}

func TestUpdateMascotStyle_InvalidStyle_Rejected(t *testing.T) {
	uc := NewResultUseCase(nil, nil, testLogger()) // repo never called — validation is first

	_, err := uc.UpdateMascotStyle(context.Background(), UpdateMascotStyleRequest{ResultID: "r1", MascotStyle: "style_z"})
	if !errors.Is(err, application.ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

func TestUpdateMascotStyle_NotOwner_Forbidden(t *testing.T) {
	ownerID := "owner-1"
	repo := mocks.NewMockResultRepository(t)
	repo.EXPECT().FindByID(mock.Anything, "r1").Return(&testresult.TestResult{ID: "r1", UserID: &ownerID}, nil).Once()
	uc := NewResultUseCase(repo, nil, testLogger())

	_, err := uc.UpdateMascotStyle(context.Background(), UpdateMascotStyleRequest{ResultID: "r1", CallerUserID: "someone-else", MascotStyle: MascotStyleB})
	if !errors.Is(err, application.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestUpdateMascotStyle_Owner_Persists(t *testing.T) {
	ownerID := "owner-1"
	repo := mocks.NewMockResultRepository(t)
	repo.EXPECT().FindByID(mock.Anything, "r1").Return(&testresult.TestResult{ID: "r1", UserID: &ownerID, MascotStyle: MascotStyleA}, nil).Once()
	repo.EXPECT().Update(mock.Anything, mock.MatchedBy(func(r *testresult.TestResult) bool { return r.MascotStyle == MascotStyleB })).Return(nil).Once()
	uc := NewResultUseCase(repo, nil, testLogger())

	resp, err := uc.UpdateMascotStyle(context.Background(), UpdateMascotStyleRequest{ResultID: "r1", CallerUserID: "owner-1", MascotStyle: MascotStyleB})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.MascotStyle != MascotStyleB {
		t.Fatalf("expected mascot style in response to reflect update, got %s", resp.MascotStyle)
	}
}

func TestGetPDFStatus_Owner_ReturnsStatus(t *testing.T) {
	ownerID := "owner-1"
	repo := mocks.NewMockResultRepository(t)
	repo.EXPECT().FindByID(mock.Anything, "r1").Return(&testresult.TestResult{ID: "r1", UserID: &ownerID, PDFStatus: testresult.PDFStatusCompleted}, nil).Once()
	uc := NewResultUseCase(repo, nil, testLogger())

	resp, err := uc.GetPDFStatus(context.Background(), "r1", "owner-1", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.PDFStatus != string(testresult.PDFStatusCompleted) {
		t.Fatalf("expected completed status, got %s", resp.PDFStatus)
	}
}

func TestGetPDFDownloadURL_NotReady_Rejected(t *testing.T) {
	ownerID := "owner-1"
	repo := mocks.NewMockResultRepository(t)
	repo.EXPECT().FindByID(mock.Anything, "r1").Return(&testresult.TestResult{ID: "r1", UserID: &ownerID}, nil).Once() // PDFUrl nil
	uc := NewResultUseCase(repo, nil, testLogger())

	_, err := uc.GetPDFDownloadURL(context.Background(), "r1", "owner-1", "")
	if !errors.Is(err, application.ErrPDFNotReady) {
		t.Fatalf("expected ErrPDFNotReady, got %v", err)
	}
}

func TestGetPDFDownloadURL_Ready_ReturnsSignedURL(t *testing.T) {
	ownerID := "owner-1"
	pdfURL := "guest/x/r1.pdf"
	repo := mocks.NewMockResultRepository(t)
	repo.EXPECT().FindByID(mock.Anything, "r1").Return(&testresult.TestResult{ID: "r1", UserID: &ownerID, PDFUrl: &pdfURL}, nil).Once()
	signer := mocks.NewMockPDFSignerService(t)
	signer.EXPECT().PresignedGetURL(mock.Anything, pdfURL, mock.Anything).Return("https://signed.example/r1.pdf", nil).Once()
	uc := NewResultUseCase(repo, signer, testLogger())

	resp, err := uc.GetPDFDownloadURL(context.Background(), "r1", "owner-1", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.URL != "https://signed.example/r1.pdf" {
		t.Fatalf("expected the signed URL to be returned, got %q", resp.URL)
	}
}

func TestGetPDFDownloadURL_GuestOwnerBySessionID(t *testing.T) {
	sessionID := "guest-session-1"
	pdfURL := "guest/guest-session-1/r1.pdf"
	repo := mocks.NewMockResultRepository(t)
	repo.EXPECT().FindByID(mock.Anything, "r1").Return(&testresult.TestResult{ID: "r1", GuestSessionID: &sessionID, PDFUrl: &pdfURL}, nil).Once()
	signer := mocks.NewMockPDFSignerService(t)
	signer.EXPECT().PresignedGetURL(mock.Anything, pdfURL, mock.Anything).Return("https://signed.example/r1.pdf", nil).Once()
	uc := NewResultUseCase(repo, signer, testLogger())

	_, err := uc.GetPDFDownloadURL(context.Background(), "r1", "", "guest-session-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
