package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/aprxty3/your_persona_controller.git/internal/application"
	"github.com/aprxty3/your_persona_controller.git/internal/application/auth/mocks"
	"github.com/aprxty3/your_persona_controller.git/internal/domain/account"
	accountmocks "github.com/aprxty3/your_persona_controller.git/internal/domain/account/mocks"
	"github.com/stretchr/testify/mock"
)

func validRegisterRequest() RegisterRequest {
	return RegisterRequest{Email: "new@example.com", Password: "Str0ngPassw0rd!", PreferredLocale: "en", IPAddress: "1.2.3.4"}
}

// Everything below tests only the pre-transaction portion of Register — the
// success path (and its db.Transaction body) genuinely needs a real
// Postgres connection (it constructs concrete pgaccount repos against the tx
// directly, not through the injected interfaces), so it belongs to
// integration tests, not this package's unit tests (AGENTS.md).

func TestRegister_RateLimited_RejectsBeforeValidation(t *testing.T) {
	ipLimiter := mocks.NewMockIPRateLimiter(t)
	ipLimiter.EXPECT().Allow(mock.Anything, mock.Anything, mock.Anything).Return(false, 30, nil).Once()
	userRepo := accountmocks.NewMockUserRepository(t) // no EXPECT(): FindByEmail must never be called

	uc := &RegisterUseCase{userRepo: userRepo, ipRateLimiter: ipLimiter, breachChecker: NewNoopBreachChecker(), log: testLogger()}

	_, err := uc.Register(context.Background(), validRegisterRequest())
	if !errors.Is(err, application.ErrRateLimited) {
		t.Fatalf("expected ErrRateLimited, got %v", err)
	}
}

func TestRegister_InvalidEmail_Rejected(t *testing.T) {
	ipLimiter := mocks.NewMockIPRateLimiter(t)
	ipLimiter.EXPECT().Allow(mock.Anything, mock.Anything, mock.Anything).Return(true, 0, nil).Once()

	uc := &RegisterUseCase{ipRateLimiter: ipLimiter, breachChecker: NewNoopBreachChecker(), log: testLogger()}

	req := validRegisterRequest()
	req.Email = "not-an-email"
	_, err := uc.Register(context.Background(), req)
	if !errors.Is(err, application.ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput for a malformed email, got %v", err)
	}
}

func TestRegister_UnsupportedLocale_Rejected(t *testing.T) {
	ipLimiter := mocks.NewMockIPRateLimiter(t)
	ipLimiter.EXPECT().Allow(mock.Anything, mock.Anything, mock.Anything).Return(true, 0, nil).Once()

	uc := &RegisterUseCase{ipRateLimiter: ipLimiter, breachChecker: NewNoopBreachChecker(), log: testLogger()}

	req := validRegisterRequest()
	req.PreferredLocale = "fr"
	_, err := uc.Register(context.Background(), req)
	if !errors.Is(err, application.ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput for an unsupported locale, got %v", err)
	}
}

func TestRegister_WeakPassword_Rejected(t *testing.T) {
	ipLimiter := mocks.NewMockIPRateLimiter(t)
	ipLimiter.EXPECT().Allow(mock.Anything, mock.Anything, mock.Anything).Return(true, 0, nil).Once()

	uc := &RegisterUseCase{ipRateLimiter: ipLimiter, breachChecker: NewNoopBreachChecker(), log: testLogger()}

	req := validRegisterRequest()
	req.Password = "short"
	_, err := uc.Register(context.Background(), req)
	if err == nil {
		t.Fatal("expected a password policy error")
	}
}

func TestRegister_EmailAlreadyRegistered_Rejected(t *testing.T) {
	ipLimiter := mocks.NewMockIPRateLimiter(t)
	ipLimiter.EXPECT().Allow(mock.Anything, mock.Anything, mock.Anything).Return(true, 0, nil).Once()
	userRepo := accountmocks.NewMockUserRepository(t)
	userRepo.EXPECT().FindByEmail(mock.Anything, "new@example.com").Return(&account.User{ID: "existing-user"}, nil).Once()

	uc := &RegisterUseCase{userRepo: userRepo, ipRateLimiter: ipLimiter, breachChecker: NewNoopBreachChecker(), log: testLogger()}

	_, err := uc.Register(context.Background(), validRegisterRequest())
	if !errors.Is(err, application.ErrEmailAlreadyRegistered) {
		t.Fatalf("expected ErrEmailAlreadyRegistered, got %v", err)
	}
}

func TestRegister_GuestSessionLookupError_Propagates(t *testing.T) {
	ipLimiter := mocks.NewMockIPRateLimiter(t)
	ipLimiter.EXPECT().Allow(mock.Anything, mock.Anything, mock.Anything).Return(true, 0, nil).Once()
	userRepo := accountmocks.NewMockUserRepository(t)
	userRepo.EXPECT().FindByEmail(mock.Anything, "new@example.com").Return(nil, nil).Once()
	guestRepo := accountmocks.NewMockGuestSessionRepository(t)
	guestRepo.EXPECT().FindBySessionID(mock.Anything, "guest-1").Return(nil, errors.New("db down")).Once()

	uc := &RegisterUseCase{userRepo: userRepo, guestRepo: guestRepo, ipRateLimiter: ipLimiter, breachChecker: NewNoopBreachChecker(), log: testLogger()}

	req := validRegisterRequest()
	sessionID := "guest-1"
	req.GuestSessionID = &sessionID
	_, err := uc.Register(context.Background(), req)
	if err == nil {
		t.Fatal("expected the guest session lookup error to propagate before reaching the transaction")
	}
}
