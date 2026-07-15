package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aprxty3/your_persona_controller.git/internal/application"
	"github.com/aprxty3/your_persona_controller.git/internal/application/auth/mocks"
	"github.com/aprxty3/your_persona_controller.git/internal/domain/account"
	accountmocks "github.com/aprxty3/your_persona_controller.git/internal/domain/account/mocks"
	"github.com/aprxty3/your_persona_controller.git/internal/infrastructure/cache/redis"
	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
	taskqueuemocks "github.com/aprxty3/your_persona_controller.git/pkg/taskqueue/mocks"
	"github.com/stretchr/testify/mock"
)

func testLogger() logger.Logger { return logger.NewLogger("test") }

func newActiveToken(code string) *account.VerificationToken {
	return &account.VerificationToken{
		ID:        "token-1",
		UserID:    "user-1",
		Token:     code,
		Type:      account.TokenTypeEmailVerification,
		ExpiresAt: time.Now().Add(15 * time.Minute),
	}
}

// --- validateOTPAttempt: the shared OTP gate behind both verify-email and reset-otp ---

// The 5th consecutive wrong guess must reject with ErrOTPMaxAttempts (not
// ErrInvalidOTP) — this is the attempt actually observed by the caller as
// exhausting the budget.
func TestValidateOTPAttempt_FifthWrongGuess_RejectsWithMaxAttempts(t *testing.T) {
	tok := newActiveToken("123456")
	tokenRepo := accountmocks.NewMockVerificationTokenRepository(t)
	tokenRepo.EXPECT().FindActiveByUserAndType(mock.Anything, "user-1", account.TokenTypeEmailVerification).Return(tok, nil).Times(application.MaxWrongOTPAttempts)
	tokenRepo.EXPECT().IncrementAttemptCount(mock.Anything, "token-1").RunAndReturn(func(ctx context.Context, id string) error {
		tok.AttemptCount++
		return nil
	}).Times(application.MaxWrongOTPAttempts)

	var lastErr error
	for i := 0; i < application.MaxWrongOTPAttempts; i++ {
		_, _, lastErr = validateOTPAttempt(context.Background(), tokenRepo, "user-1", "000000", account.TokenTypeEmailVerification, testLogger())
	}

	if !errors.Is(lastErr, application.ErrOTPMaxAttempts) {
		t.Fatalf("expected ErrOTPMaxAttempts on the %dth wrong guess, got %v", application.MaxWrongOTPAttempts, lastErr)
	}
	if tok.AttemptCount != application.MaxWrongOTPAttempts {
		t.Fatalf("expected attempt_count to be %d, got %d", application.MaxWrongOTPAttempts, tok.AttemptCount)
	}
}

// Once attempt_count has reached the ceiling, every subsequent call must be
// rejected outright — including one that supplies the CORRECT code — and
// must not increment attempt_count any further (IncrementAttemptCount must
// never be called).
func TestValidateOTPAttempt_AfterMaxAttempts_CorrectCodeStillRejected(t *testing.T) {
	tok := newActiveToken("123456")
	tok.AttemptCount = application.MaxWrongOTPAttempts
	tokenRepo := accountmocks.NewMockVerificationTokenRepository(t)
	tokenRepo.EXPECT().FindActiveByUserAndType(mock.Anything, "user-1", account.TokenTypeEmailVerification).Return(tok, nil).Once()

	_, _, err := validateOTPAttempt(context.Background(), tokenRepo, "user-1", "123456", account.TokenTypeEmailVerification, testLogger())

	if !errors.Is(err, application.ErrOTPMaxAttempts) {
		t.Fatalf("expected ErrOTPMaxAttempts even with the correct code, got %v", err)
	}
}

func TestValidateOTPAttempt_CorrectCodeBeforeLimit_Succeeds(t *testing.T) {
	tok := newActiveToken("123456")
	tokenRepo := accountmocks.NewMockVerificationTokenRepository(t)
	tokenRepo.EXPECT().FindActiveByUserAndType(mock.Anything, "user-1", account.TokenTypeEmailVerification).Return(tok, nil).Once()

	token, remaining, err := validateOTPAttempt(context.Background(), tokenRepo, "user-1", "123456", account.TokenTypeEmailVerification, testLogger())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token == nil || token.ID != "token-1" {
		t.Fatalf("expected the matched token to be returned, got %v", token)
	}
	if remaining != application.MaxWrongOTPAttempts {
		t.Fatalf("expected full remaining budget on first-try success, got %d", remaining)
	}
}

func TestValidateOTPAttempt_WrongCode_IncrementsAndReturnsRemaining(t *testing.T) {
	tok := newActiveToken("123456")
	tokenRepo := accountmocks.NewMockVerificationTokenRepository(t)
	tokenRepo.EXPECT().FindActiveByUserAndType(mock.Anything, "user-1", account.TokenTypeEmailVerification).Return(tok, nil).Once()
	tokenRepo.EXPECT().IncrementAttemptCount(mock.Anything, "token-1").RunAndReturn(func(ctx context.Context, id string) error {
		tok.AttemptCount++
		return nil
	}).Once()

	_, remaining, err := validateOTPAttempt(context.Background(), tokenRepo, "user-1", "000000", account.TokenTypeEmailVerification, testLogger())
	if !errors.Is(err, application.ErrInvalidOTP) {
		t.Fatalf("expected ErrInvalidOTP, got %v", err)
	}
	if remaining != application.MaxWrongOTPAttempts-1 {
		t.Fatalf("expected %d attempts remaining, got %d", application.MaxWrongOTPAttempts-1, remaining)
	}
}

func TestValidateOTPAttempt_NoActiveToken_ExpiredError(t *testing.T) {
	tokenRepo := accountmocks.NewMockVerificationTokenRepository(t)
	tokenRepo.EXPECT().FindActiveByUserAndType(mock.Anything, "user-1", account.TokenTypeEmailVerification).Return(nil, nil).Once()

	_, _, err := validateOTPAttempt(context.Background(), tokenRepo, "user-1", "123456", account.TokenTypeEmailVerification, testLogger())
	if !errors.Is(err, application.ErrOTPExpired) {
		t.Fatalf("expected ErrOTPExpired when no active token exists, got %v", err)
	}
}

func TestValidateOTPAttempt_ExpiredToken_ExpiredError(t *testing.T) {
	tok := newActiveToken("123456")
	tok.ExpiresAt = time.Now().Add(-1 * time.Minute)
	tokenRepo := accountmocks.NewMockVerificationTokenRepository(t)
	tokenRepo.EXPECT().FindActiveByUserAndType(mock.Anything, "user-1", account.TokenTypeEmailVerification).Return(tok, nil).Once()

	_, _, err := validateOTPAttempt(context.Background(), tokenRepo, "user-1", "123456", account.TokenTypeEmailVerification, testLogger())
	if !errors.Is(err, application.ErrOTPExpired) {
		t.Fatalf("expected ErrOTPExpired for a time-expired token, got %v", err)
	}
}

// --- AccountUseCase.ResendEmailOTP / ForgotPassword ---

func TestResendEmailOTP_RateLimited_RejectsBeforeUserLookup(t *testing.T) {
	rateLimiter := mocks.NewMockOTPRateLimiter(t)
	rateLimiter.EXPECT().CheckAndConsume(mock.Anything, redis.ScopeEmailVerification, "a@example.com").Return(30, nil).Once()
	userRepo := accountmocks.NewMockUserRepository(t) // no EXPECT(): FindByEmail must never be called

	uc := &AccountUseCase{userRepo: userRepo, rateLimiter: rateLimiter, log: testLogger()}

	_, err := uc.ResendEmailOTP(context.Background(), ResendEmailOTPRequest{Email: "a@example.com"})
	if !errors.Is(err, application.ErrRateLimited) {
		t.Fatalf("expected ErrRateLimited, got %v", err)
	}
}

// Unknown email is a silent no-op (prevents account enumeration) — not an error.
func TestResendEmailOTP_UnknownEmail_SilentNoOp(t *testing.T) {
	rateLimiter := mocks.NewMockOTPRateLimiter(t)
	rateLimiter.EXPECT().CheckAndConsume(mock.Anything, redis.ScopeEmailVerification, "nobody@example.com").Return(0, nil).Once()
	userRepo := accountmocks.NewMockUserRepository(t)
	userRepo.EXPECT().FindByEmail(mock.Anything, "nobody@example.com").Return(nil, nil).Once()

	uc := &AccountUseCase{userRepo: userRepo, rateLimiter: rateLimiter, log: testLogger()}

	resp, err := uc.ResendEmailOTP(context.Background(), ResendEmailOTPRequest{Email: "nobody@example.com"})
	if err != nil {
		t.Fatalf("expected a silent no-op, got error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected a non-nil response")
	}
}

func TestResendEmailOTP_KnownEmail_IssuesNewOTP(t *testing.T) {
	user := &account.User{ID: "user-1", Email: "a@example.com", PreferredLocale: "en"}
	rateLimiter := mocks.NewMockOTPRateLimiter(t)
	rateLimiter.EXPECT().CheckAndConsume(mock.Anything, redis.ScopeEmailVerification, "a@example.com").Return(0, nil).Once()
	rateLimiter.EXPECT().SetCooldown(mock.Anything, redis.ScopeEmailVerification, "a@example.com").Return(nil).Once()
	userRepo := accountmocks.NewMockUserRepository(t)
	userRepo.EXPECT().FindByEmail(mock.Anything, "a@example.com").Return(user, nil).Once()
	tokenRepo := accountmocks.NewMockVerificationTokenRepository(t)
	tokenRepo.EXPECT().ExpireAllActiveForUser(mock.Anything, "user-1", account.TokenTypeEmailVerification).Return(nil).Once()
	tokenRepo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil).Once()
	dispatcher := taskqueuemocks.NewMockDispatcher(t)
	dispatcher.EXPECT().EnqueueEmail(mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()

	uc := &AccountUseCase{userRepo: userRepo, tokenRepo: tokenRepo, dispatcher: dispatcher, rateLimiter: rateLimiter, log: testLogger()}

	if _, err := uc.ResendEmailOTP(context.Background(), ResendEmailOTPRequest{Email: "a@example.com"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestForgotPassword_RateLimited_RejectsBeforeUserLookup(t *testing.T) {
	rateLimiter := mocks.NewMockOTPRateLimiter(t)
	rateLimiter.EXPECT().CheckAndConsume(mock.Anything, redis.ScopePasswordReset, "a@example.com").Return(60, nil).Once()
	userRepo := accountmocks.NewMockUserRepository(t)

	uc := &AccountUseCase{userRepo: userRepo, rateLimiter: rateLimiter, log: testLogger()}

	_, err := uc.ForgotPassword(context.Background(), ForgotPasswordRequest{Email: "a@example.com"})
	if !errors.Is(err, application.ErrRateLimited) {
		t.Fatalf("expected ErrRateLimited, got %v", err)
	}
}

func TestForgotPassword_KnownEmail_IssuesResetOTP(t *testing.T) {
	user := &account.User{ID: "user-1", Email: "a@example.com", PreferredLocale: "id"}
	rateLimiter := mocks.NewMockOTPRateLimiter(t)
	rateLimiter.EXPECT().CheckAndConsume(mock.Anything, redis.ScopePasswordReset, "a@example.com").Return(0, nil).Once()
	rateLimiter.EXPECT().SetCooldown(mock.Anything, redis.ScopePasswordReset, "a@example.com").Return(nil).Once()
	userRepo := accountmocks.NewMockUserRepository(t)
	userRepo.EXPECT().FindByEmail(mock.Anything, "a@example.com").Return(user, nil).Once()
	tokenRepo := accountmocks.NewMockVerificationTokenRepository(t)
	tokenRepo.EXPECT().ExpireAllActiveForUser(mock.Anything, "user-1", account.TokenTypePasswordReset).Return(nil).Once()
	tokenRepo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil).Once()
	dispatcher := taskqueuemocks.NewMockDispatcher(t)
	dispatcher.EXPECT().EnqueueEmail(mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()

	uc := &AccountUseCase{userRepo: userRepo, tokenRepo: tokenRepo, dispatcher: dispatcher, rateLimiter: rateLimiter, log: testLogger()}

	if _, err := uc.ForgotPassword(context.Background(), ForgotPasswordRequest{Email: "a@example.com"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
