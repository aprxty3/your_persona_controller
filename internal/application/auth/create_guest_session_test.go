package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aprxty3/your_persona_controller.git/internal/application"
	accountmocks "github.com/aprxty3/your_persona_controller.git/internal/domain/account/mocks"
	"github.com/stretchr/testify/mock"
)

func validGuestSessionRequest() CreateGuestSessionRequest {
	return CreateGuestSessionRequest{DisplayName: "Budi", Age: application.MinimumAge, Status: "student", Locale: "en", IPAddress: "1.2.3.4"}
}

func TestCreateGuestSession_MissingDisplayName_Rejected(t *testing.T) {
	repo := accountmocks.NewMockGuestSessionRepository(t) // no EXPECT(): Create must never be called
	uc := NewCreateGuestSessionUseCase(repo, testLogger())

	req := validGuestSessionRequest()
	req.DisplayName = ""
	_, err := uc.Execute(context.Background(), req)
	if !errors.Is(err, application.ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

func TestCreateGuestSession_BelowMinimumAge_Rejected(t *testing.T) {
	repo := accountmocks.NewMockGuestSessionRepository(t)
	uc := NewCreateGuestSessionUseCase(repo, testLogger())

	req := validGuestSessionRequest()
	req.Age = application.MinimumAge - 1
	_, err := uc.Execute(context.Background(), req)
	if !errors.Is(err, application.ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput for under-age, got %v", err)
	}
}

func TestCreateGuestSession_InvalidStatus_Rejected(t *testing.T) {
	repo := accountmocks.NewMockGuestSessionRepository(t)
	uc := NewCreateGuestSessionUseCase(repo, testLogger())

	req := validGuestSessionRequest()
	req.Status = "not-a-real-status"
	_, err := uc.Execute(context.Background(), req)
	if !errors.Is(err, application.ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput for invalid status, got %v", err)
	}
}

func TestCreateGuestSession_UnsupportedLocale_Rejected(t *testing.T) {
	repo := accountmocks.NewMockGuestSessionRepository(t)
	uc := NewCreateGuestSessionUseCase(repo, testLogger())

	req := validGuestSessionRequest()
	req.Locale = "fr"
	_, err := uc.Execute(context.Background(), req)
	if !errors.Is(err, application.ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput for unsupported locale, got %v", err)
	}
}

func TestCreateGuestSession_RepoError_Propagates(t *testing.T) {
	repo := accountmocks.NewMockGuestSessionRepository(t)
	repo.EXPECT().Create(mock.Anything, mock.Anything).Return(errors.New("db down")).Once()
	uc := NewCreateGuestSessionUseCase(repo, testLogger())

	_, err := uc.Execute(context.Background(), validGuestSessionRequest())
	if err == nil {
		t.Fatal("expected the repository error to propagate")
	}
}

func TestCreateGuestSession_Success(t *testing.T) {
	repo := accountmocks.NewMockGuestSessionRepository(t)
	repo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil).Once()
	uc := NewCreateGuestSessionUseCase(repo, testLogger())

	resp, err := uc.Execute(context.Background(), validGuestSessionRequest())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.SessionID == "" {
		t.Fatal("expected a generated session ID")
	}
	wantExpiry := time.Now().Add(application.GuestDataRetention)
	if diff := resp.ExpiresAt.Sub(wantExpiry); diff > time.Minute || diff < -time.Minute {
		t.Fatalf("expected ExpiresAt ~= now+GuestDataRetention, got %v (diff %v)", resp.ExpiresAt, diff)
	}
}
