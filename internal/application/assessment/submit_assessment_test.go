package assessment

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aprxty3/your_persona_controller.git/internal/application"
	"github.com/aprxty3/your_persona_controller.git/internal/domain/testresult"
	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
)

func testLogger() logger.Logger { return logger.NewLogger("test") }

type mockIdempotencyService struct {
	checkFn   func(ctx context.Context, key, hash string) (*SubmitResponse, error)
	saveCalls int
}

func (m *mockIdempotencyService) Check(ctx context.Context, key, hash string) (*SubmitResponse, error) {
	if m.checkFn != nil {
		return m.checkFn(ctx, key, hash)
	}
	return nil, nil
}

func (m *mockIdempotencyService) Save(ctx context.Context, key, hash string, resp *SubmitResponse, ttl time.Duration) error {
	m.saveCalls++
	return nil
}

type mockLockService struct {
	acquireFn    func(ctx context.Context, key string, ttl time.Duration) (bool, error)
	acquireCalls int
	releaseCalls int
}

func (m *mockLockService) AcquireLock(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	m.acquireCalls++
	if m.acquireFn != nil {
		return m.acquireFn(ctx, key, ttl)
	}
	return true, nil
}

func (m *mockLockService) ReleaseLock(ctx context.Context, key string) error {
	m.releaseCalls++
	return nil
}

type mockSubmitTestResultRepo struct {
	memberUsage    int64
	guestUsage     int64
	completedCount int64
	err            error
}

func (m *mockSubmitTestResultRepo) CountMonthlyUsage(ctx context.Context, userID string) (int64, error) {
	return m.memberUsage, m.err
}

func (m *mockSubmitTestResultRepo) CountMonthlyUsageByGuestSession(ctx context.Context, guestSessionID string) (int64, error) {
	return m.guestUsage, m.err
}

func (m *mockSubmitTestResultRepo) CountCompletedByUser(ctx context.Context, userID string) (int64, error) {
	return m.completedCount, m.err
}

type mockAIGeneratorService struct {
	called     bool
	generateFn func(ctx context.Context, text, locale string) (summary, rawPrompt string, tokens int, err error)
}

func (m *mockAIGeneratorService) GenerateSummary(ctx context.Context, text, locale string) (string, string, int, error) {
	m.called = true
	if m.generateFn != nil {
		return m.generateFn(ctx, text, locale)
	}
	return "", "", 0, nil
}

func newTestSubmitUseCase(idemSvc IdempotencyService, lockSvc DistributedLockService, trRepo TestResultRepository, aiSvc AIGeneratorService) *SubmitAssessmentUseCase {
	return &SubmitAssessmentUseCase{
		testResultRepo: trRepo,
		aiSvc:          aiSvc,
		lockSvc:        lockSvc,
		idempotencySvc: idemSvc,
		log:            testLogger(),
	}
}

func validAnswers() []AnswerInput {
	return []AnswerInput{{QuestionID: "q1", Value: "4"}}
}

func TestExecute_EmptyAnswers_RejectedBeforeAnyDependency(t *testing.T) {
	uc := newTestSubmitUseCase(&mockIdempotencyService{}, &mockLockService{}, &mockSubmitTestResultRepo{}, &mockAIGeneratorService{})

	_, err := uc.Execute(context.Background(), SubmitRequest{Answers: nil})
	if !errors.Is(err, application.ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

// A reused idempotency key with a DIFFERENT payload hash must be rejected
// immediately — before the lock is even attempted, and Gemini must never run.
func TestExecute_IdempotencyKeyReused_RejectsWithoutTouchingLockOrAI(t *testing.T) {
	idemSvc := &mockIdempotencyService{checkFn: func(ctx context.Context, key, hash string) (*SubmitResponse, error) {
		return nil, application.ErrIdempotencyKeyReused
	}}
	lockSvc := &mockLockService{}
	aiSvc := &mockAIGeneratorService{}
	uc := newTestSubmitUseCase(idemSvc, lockSvc, &mockSubmitTestResultRepo{}, aiSvc)

	_, err := uc.Execute(context.Background(), SubmitRequest{IdempotencyKey: "key-1", SessionID: "sess-1", Answers: validAnswers()})
	if !errors.Is(err, application.ErrIdempotencyKeyReused) {
		t.Fatalf("expected ErrIdempotencyKeyReused, got %v", err)
	}
	if lockSvc.acquireCalls != 0 {
		t.Fatalf("expected the lock to never be attempted, got %d calls", lockSvc.acquireCalls)
	}
	if aiSvc.called {
		t.Fatal("expected Gemini to never be called")
	}
}

// A matching idempotency key returns the previously-saved response verbatim
// — no Gemini call, no fresh insert (nothing beyond the cache read happens).
func TestExecute_IdempotencyCacheHit_ReturnsCachedWithoutAICall(t *testing.T) {
	cached := &SubmitResponse{ResultID: "cached-result-id", Status: "completed"}
	idemSvc := &mockIdempotencyService{checkFn: func(ctx context.Context, key, hash string) (*SubmitResponse, error) {
		return cached, nil
	}}
	lockSvc := &mockLockService{}
	aiSvc := &mockAIGeneratorService{}
	uc := newTestSubmitUseCase(idemSvc, lockSvc, &mockSubmitTestResultRepo{}, aiSvc)

	resp, err := uc.Execute(context.Background(), SubmitRequest{IdempotencyKey: "key-1", SessionID: "sess-1", Answers: validAnswers()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != cached {
		t.Fatalf("expected the exact cached response to be returned, got %v", resp)
	}
	if lockSvc.acquireCalls != 0 {
		t.Fatalf("expected the lock to never be attempted on a cache hit, got %d calls", lockSvc.acquireCalls)
	}
	if aiSvc.called {
		t.Fatal("expected Gemini to never be called on a cache hit")
	}
}

func TestExecute_LockNotAcquired_RejectsWithLockError(t *testing.T) {
	idemSvc := &mockIdempotencyService{}
	lockSvc := &mockLockService{acquireFn: func(ctx context.Context, key string, ttl time.Duration) (bool, error) {
		return false, nil
	}}
	uc := newTestSubmitUseCase(idemSvc, lockSvc, &mockSubmitTestResultRepo{}, &mockAIGeneratorService{})

	_, err := uc.Execute(context.Background(), SubmitRequest{IdempotencyKey: "key-1", SessionID: "sess-1", Answers: validAnswers()})
	if !errors.Is(err, application.ErrLockNotAcquired) {
		t.Fatalf("expected ErrLockNotAcquired, got %v", err)
	}
}

func TestExecute_MemberQuotaExceeded_RejectsAndReleasesLock(t *testing.T) {
	idemSvc := &mockIdempotencyService{}
	lockSvc := &mockLockService{}
	trRepo := &mockSubmitTestResultRepo{memberUsage: application.MemberMonthlyQuota}
	uc := newTestSubmitUseCase(idemSvc, lockSvc, trRepo, &mockAIGeneratorService{})

	_, err := uc.Execute(context.Background(), SubmitRequest{IdempotencyKey: "key-1", SessionID: "user-1", IsMember: true, Answers: validAnswers()})
	if !errors.Is(err, application.ErrQuotaExceeded) {
		t.Fatalf("expected ErrQuotaExceeded, got %v", err)
	}
	if lockSvc.releaseCalls != 1 {
		t.Fatalf("expected the lock to be released exactly once even on early return, got %d", lockSvc.releaseCalls)
	}
}

func TestExecute_GuestQuotaExceeded_Rejects(t *testing.T) {
	idemSvc := &mockIdempotencyService{}
	lockSvc := &mockLockService{}
	trRepo := &mockSubmitTestResultRepo{guestUsage: application.GuestMonthlyQuota}
	uc := newTestSubmitUseCase(idemSvc, lockSvc, trRepo, &mockAIGeneratorService{})

	_, err := uc.Execute(context.Background(), SubmitRequest{IdempotencyKey: "key-1", SessionID: "guest-session-1", IsMember: false, Answers: validAnswers()})
	if !errors.Is(err, application.ErrQuotaExceeded) {
		t.Fatalf("expected ErrQuotaExceeded, got %v", err)
	}
}

func TestExecute_MemberUnderQuota_GuestPathNotChecked(t *testing.T) {
	idemSvc := &mockIdempotencyService{}
	lockSvc := &mockLockService{acquireFn: func(ctx context.Context, key string, ttl time.Duration) (bool, error) {
		return false, nil
	}}
	trRepo := &mockSubmitTestResultRepo{memberUsage: application.MemberMonthlyQuota - 1, guestUsage: application.GuestMonthlyQuota}
	uc := newTestSubmitUseCase(idemSvc, lockSvc, trRepo, &mockAIGeneratorService{})

	_, err := uc.Execute(context.Background(), SubmitRequest{IdempotencyKey: "key-1", SessionID: "user-1", IsMember: true, Answers: validAnswers()})
	if !errors.Is(err, application.ErrLockNotAcquired) {
		t.Fatalf("expected ErrLockNotAcquired, got %v", err)
	}
}

func TestRunAIPhase_NoEssays_SkipsGeminiEntirely(t *testing.T) {
	aiSvc := &mockAIGeneratorService{}
	uc := newTestSubmitUseCase(&mockIdempotencyService{}, &mockLockService{}, &mockSubmitTestResultRepo{}, aiSvc)

	outcome := uc.runAIPhase(context.Background(), "result-1", nil, "en")
	if aiSvc.called {
		t.Fatal("expected Gemini to never be called when there are no essay answers")
	}
	if outcome.status != testresult.StatusFallbackStatic {
		t.Fatalf("expected fallback_static status, got %s", outcome.status)
	}
	if outcome.called {
		t.Fatal("expected outcome.called=false")
	}
}

func TestRunAIPhase_GeminiError_DegradesToFallbackStatic(t *testing.T) {
	aiSvc := &mockAIGeneratorService{generateFn: func(ctx context.Context, text, locale string) (string, string, int, error) {
		return "", "", 0, errors.New("gemini: upstream 503")
	}}
	uc := newTestSubmitUseCase(&mockIdempotencyService{}, &mockLockService{}, &mockSubmitTestResultRepo{}, aiSvc)

	outcome := uc.runAIPhase(context.Background(), "result-1", []string{"my essay answer"}, "en")
	if outcome.status != testresult.StatusFallbackStatic {
		t.Fatalf("expected fallback_static status, got %s", outcome.status)
	}
	if !outcome.flaggedAnomaly {
		t.Fatal("expected flaggedAnomaly=true on a Gemini error")
	}
	if !outcome.called {
		t.Fatal("expected outcome.called=true (Gemini WAS invoked, it just failed)")
	}
}

func TestRunAIPhase_InvalidOutput_DegradesToFallbackStatic(t *testing.T) {
	aiSvc := &mockAIGeneratorService{generateFn: func(ctx context.Context, text, locale string) (string, string, int, error) {
		return "too short", "prompt", 10, nil // below aivalidator.minLength
	}}
	uc := newTestSubmitUseCase(&mockIdempotencyService{}, &mockLockService{}, &mockSubmitTestResultRepo{}, aiSvc)

	outcome := uc.runAIPhase(context.Background(), "result-1", []string{"my essay answer"}, "en")
	if outcome.status != testresult.StatusFallbackStatic {
		t.Fatalf("expected fallback_static status for invalid output, got %s", outcome.status)
	}
	if !outcome.flaggedAnomaly {
		t.Fatal("expected flaggedAnomaly=true on invalid output")
	}
}

func TestRunAIPhase_ValidOutput_Completes(t *testing.T) {
	longSummary := "This is a perfectly valid, sufficiently long AI-generated summary of the essay answers provided by the user."
	aiSvc := &mockAIGeneratorService{generateFn: func(ctx context.Context, text, locale string) (string, string, int, error) {
		return longSummary, "prompt", 123, nil
	}}
	uc := newTestSubmitUseCase(&mockIdempotencyService{}, &mockLockService{}, &mockSubmitTestResultRepo{}, aiSvc)

	outcome := uc.runAIPhase(context.Background(), "result-1", []string{"my essay answer"}, "en")
	if outcome.status != testresult.StatusCompleted {
		t.Fatalf("expected completed status, got %s", outcome.status)
	}
	if outcome.flaggedAnomaly {
		t.Fatal("expected flaggedAnomaly=false on a valid response")
	}
	if outcome.summary != longSummary {
		t.Fatalf("expected the real summary to be kept, got %q", outcome.summary)
	}
	if outcome.totalTokens == nil || *outcome.totalTokens != 123 {
		t.Fatalf("expected totalTokens=123, got %v", outcome.totalTokens)
	}
}
