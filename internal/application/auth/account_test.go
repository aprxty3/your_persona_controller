package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aprxty3/your_persona_controller.git/internal/application"
	"github.com/aprxty3/your_persona_controller.git/internal/domain/account"
	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
)

func testLogger() logger.Logger { return logger.NewLogger("test") }

type mockVerificationTokenRepo struct {
	token        *account.VerificationToken
	incrementErr error
}

func (m *mockVerificationTokenRepo) Create(ctx context.Context, token *account.VerificationToken) error {
	return nil
}

func (m *mockVerificationTokenRepo) FindActiveByUserAndType(ctx context.Context, userID string, tokenType account.TokenType) (*account.VerificationToken, error) {
	return m.token, nil
}

func (m *mockVerificationTokenRepo) IncrementAttemptCount(ctx context.Context, tokenID string) error {
	if m.token != nil {
		m.token.AttemptCount++
	}
	return m.incrementErr
}

func (m *mockVerificationTokenRepo) MarkUsed(ctx context.Context, tokenID string) error {
	return nil
}

func (m *mockVerificationTokenRepo) ExpireAllActiveForUser(ctx context.Context, userID string, tokenType account.TokenType) error {
	return nil
}

func newActiveToken(code string) *account.VerificationToken {
	return &account.VerificationToken{
		ID:        "token-1",
		UserID:    "user-1",
		Token:     code,
		Type:      account.TokenTypeEmailVerification,
		ExpiresAt: time.Now().Add(15 * time.Minute),
	}
}

func TestValidateOTPAttempt_FifthWrongGuess_RejectsWithMaxAttempts(t *testing.T) {
	repo := &mockVerificationTokenRepo{token: newActiveToken("123456")}

	var lastErr error
	for i := 0; i < application.MaxWrongOTPAttempts; i++ {
		_, _, lastErr = validateOTPAttempt(context.Background(), repo, "user-1", "000000", account.TokenTypeEmailVerification, testLogger())
	}

	if !errors.Is(lastErr, application.ErrOTPMaxAttempts) {
		t.Fatalf("expected ErrOTPMaxAttempts on the %dth wrong guess, got %v", application.MaxWrongOTPAttempts, lastErr)
	}
	if repo.token.AttemptCount != application.MaxWrongOTPAttempts {
		t.Fatalf("expected attempt_count to be %d, got %d", application.MaxWrongOTPAttempts, repo.token.AttemptCount)
	}
}

func TestValidateOTPAttempt_AfterMaxAttempts_CorrectCodeStillRejected(t *testing.T) {
	repo := &mockVerificationTokenRepo{token: newActiveToken("123456")}
	repo.token.AttemptCount = application.MaxWrongOTPAttempts

	_, _, err := validateOTPAttempt(context.Background(), repo, "user-1", "123456", account.TokenTypeEmailVerification, testLogger())

	if !errors.Is(err, application.ErrOTPMaxAttempts) {
		t.Fatalf("expected ErrOTPMaxAttempts even with the correct code, got %v", err)
	}
	if repo.token.AttemptCount != application.MaxWrongOTPAttempts {
		t.Fatalf("expected attempt_count to stay at %d (no further increment), got %d", application.MaxWrongOTPAttempts, repo.token.AttemptCount)
	}
}

func TestValidateOTPAttempt_CorrectCodeBeforeLimit_Succeeds(t *testing.T) {
	repo := &mockVerificationTokenRepo{token: newActiveToken("123456")}

	token, remaining, err := validateOTPAttempt(context.Background(), repo, "user-1", "123456", account.TokenTypeEmailVerification, testLogger())
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

func TestValidateOTPAttempt_NoActiveToken_ExpiredError(t *testing.T) {
	repo := &mockVerificationTokenRepo{token: nil}

	_, _, err := validateOTPAttempt(context.Background(), repo, "user-1", "123456", account.TokenTypeEmailVerification, testLogger())
	if !errors.Is(err, application.ErrOTPExpired) {
		t.Fatalf("expected ErrOTPExpired when no active token exists, got %v", err)
	}
}

func TestValidateOTPAttempt_ExpiredToken_ExpiredError(t *testing.T) {
	tok := newActiveToken("123456")
	tok.ExpiresAt = time.Now().Add(-1 * time.Minute)
	repo := &mockVerificationTokenRepo{token: tok}

	_, _, err := validateOTPAttempt(context.Background(), repo, "user-1", "123456", account.TokenTypeEmailVerification, testLogger())
	if !errors.Is(err, application.ErrOTPExpired) {
		t.Fatalf("expected ErrOTPExpired for a time-expired token, got %v", err)
	}
}
