package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aprxty3/your_persona_controller.git/internal/application"
	"github.com/aprxty3/your_persona_controller.git/internal/application/auth/mocks"
	accountmocks "github.com/aprxty3/your_persona_controller.git/internal/domain/account/mocks"
	"github.com/aprxty3/your_persona_controller.git/internal/infrastructure/cache/redis"
	"github.com/stretchr/testify/mock"
)

func validGuestSessionRequest() CreateGuestSessionRequest {
	return CreateGuestSessionRequest{DisplayName: "Budi", Age: application.MinimumAge, Status: "student", Locale: "en", IPAddress: "1.2.3.4"}
}

// allowingIPLimiter returns an IPRateLimiter mock that always allows —
// the default for every test not specifically exercising the TICKET-22
// rate-limit gate itself.
func allowingIPLimiter(t *testing.T) *mocks.MockIPRateLimiter {
	limiter := mocks.NewMockIPRateLimiter(t)
	limiter.EXPECT().Allow(mock.Anything, redis.ScopeGuestSessionIP, mock.Anything).Return(true, 0, nil).Maybe()
	return limiter
}

func TestCreateGuestSession_MissingDisplayName_Rejected(t *testing.T) {
	repo := accountmocks.NewMockGuestSessionRepository(t) // no EXPECT(): Create must never be called
	uc := NewCreateGuestSessionUseCase(repo, allowingIPLimiter(t), testLogger())

	req := validGuestSessionRequest()
	req.DisplayName = ""
	_, err := uc.Execute(context.Background(), req)
	if !errors.Is(err, application.ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

func TestCreateGuestSession_BelowMinimumAge_Rejected(t *testing.T) {
	repo := accountmocks.NewMockGuestSessionRepository(t)
	uc := NewCreateGuestSessionUseCase(repo, allowingIPLimiter(t), testLogger())

	req := validGuestSessionRequest()
	req.Age = application.MinimumAge - 1
	_, err := uc.Execute(context.Background(), req)
	if !errors.Is(err, application.ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput for under-age, got %v", err)
	}
}

func TestCreateGuestSession_InvalidStatus_Rejected(t *testing.T) {
	repo := accountmocks.NewMockGuestSessionRepository(t)
	uc := NewCreateGuestSessionUseCase(repo, allowingIPLimiter(t), testLogger())

	req := validGuestSessionRequest()
	req.Status = "not-a-real-status"
	_, err := uc.Execute(context.Background(), req)
	if !errors.Is(err, application.ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput for invalid status, got %v", err)
	}
}

func TestCreateGuestSession_UnsupportedLocale_Rejected(t *testing.T) {
	repo := accountmocks.NewMockGuestSessionRepository(t)
	uc := NewCreateGuestSessionUseCase(repo, allowingIPLimiter(t), testLogger())

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
	uc := NewCreateGuestSessionUseCase(repo, allowingIPLimiter(t), testLogger())

	_, err := uc.Execute(context.Background(), validGuestSessionRequest())
	if err == nil {
		t.Fatal("expected the repository error to propagate")
	}
}

// --- TICKET-22: per-IP rate limit ---

func TestCreateGuestSession_RateLimited_RejectsBeforeValidationOrRepo(t *testing.T) {
	repo := accountmocks.NewMockGuestSessionRepository(t) // no EXPECT(): Create must never be called
	limiter := mocks.NewMockIPRateLimiter(t)
	limiter.EXPECT().Allow(mock.Anything, redis.ScopeGuestSessionIP, "1.2.3.4").Return(false, 3600, nil).Once()
	uc := NewCreateGuestSessionUseCase(repo, limiter, testLogger())

	req := validGuestSessionRequest()
	req.DisplayName = "" // would also fail validation — proves rate limit is checked first
	resp, err := uc.Execute(context.Background(), req)
	if !errors.Is(err, application.ErrRateLimited) {
		t.Fatalf("expected ErrRateLimited, got %v", err)
	}
	if resp == nil || resp.RetryAfterSeconds != 3600 {
		t.Fatalf("expected RetryAfterSeconds=3600, got %+v", resp)
	}
}

func TestCreateGuestSession_RateLimiterRedisError_FailsOpenAndProceeds(t *testing.T) {
	repo := accountmocks.NewMockGuestSessionRepository(t)
	repo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil).Once()
	limiter := mocks.NewMockIPRateLimiter(t)
	limiter.EXPECT().Allow(mock.Anything, redis.ScopeGuestSessionIP, mock.Anything).Return(false, 0, errors.New("redis down")).Once()
	uc := NewCreateGuestSessionUseCase(repo, limiter, testLogger())

	_, err := uc.Execute(context.Background(), validGuestSessionRequest())
	if err != nil {
		t.Fatalf("expected a Redis error to fail open, got %v", err)
	}
}

func TestCreateGuestSession_Success(t *testing.T) {
	repo := accountmocks.NewMockGuestSessionRepository(t)
	repo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil).Once()
	uc := NewCreateGuestSessionUseCase(repo, allowingIPLimiter(t), testLogger())

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
