package assessment

import (
	"context"
	"errors"
	"testing"

	"github.com/aprxty3/your_persona_controller.git/internal/application"
	"github.com/aprxty3/your_persona_controller.git/internal/application/assessment/dto"
	"github.com/aprxty3/your_persona_controller.git/internal/application/assessment/mocks"
	"github.com/aprxty3/your_persona_controller.git/internal/domain/testresult"
	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
	"github.com/stretchr/testify/mock"
)

func testLogger() logger.Logger { return logger.NewLogger("test") }

// newTestSubmitUseCase wires only the collaborators a given test actually
// exercises — mockery mocks panic on any unexpected call, so leaving a
// dependency nil is itself an assertion that the code path never touches it.
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
	uc := newTestSubmitUseCase(nil, nil, nil, nil)

	_, err := uc.Execute(context.Background(), SubmitRequest{Answers: nil})
	if !errors.Is(err, application.ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

// A reused idempotency key with a DIFFERENT payload hash must be rejected
// immediately — before the lock is even attempted, and Gemini must never run.
func TestExecute_IdempotencyKeyReused_RejectsWithoutTouchingLockOrAI(t *testing.T) {
	idemSvc := mocks.NewMockIdempotencyService(t)
	idemSvc.EXPECT().Check(mock.Anything, mock.Anything, mock.Anything).Return(nil, application.ErrIdempotencyKeyReused).Once()
	uc := newTestSubmitUseCase(idemSvc, nil, nil, nil)

	_, err := uc.Execute(context.Background(), SubmitRequest{IdempotencyKey: "key-1", SessionID: "sess-1", Answers: validAnswers()})
	if !errors.Is(err, application.ErrIdempotencyKeyReused) {
		t.Fatalf("expected ErrIdempotencyKeyReused, got %v", err)
	}
}

// A matching idempotency key returns the previously-saved response verbatim
// — no Gemini call, no fresh insert (nothing beyond the cache read happens).
func TestExecute_IdempotencyCacheHit_ReturnsCachedWithoutAICall(t *testing.T) {
	cached := &dto.SubmitResponse{ResultID: "cached-result-id", Status: "completed"}
	idemSvc := mocks.NewMockIdempotencyService(t)
	idemSvc.EXPECT().Check(mock.Anything, mock.Anything, mock.Anything).Return(cached, nil).Once()
	uc := newTestSubmitUseCase(idemSvc, nil, nil, nil)

	resp, err := uc.Execute(context.Background(), SubmitRequest{IdempotencyKey: "key-1", SessionID: "sess-1", Answers: validAnswers()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != cached {
		t.Fatalf("expected the exact cached response to be returned, got %v", resp)
	}
}

func TestExecute_LockNotAcquired_RejectsWithLockError(t *testing.T) {
	idemSvc := mocks.NewMockIdempotencyService(t)
	idemSvc.EXPECT().Check(mock.Anything, mock.Anything, mock.Anything).Return(nil, nil).Once()
	lockSvc := mocks.NewMockDistributedLockService(t)
	lockSvc.EXPECT().AcquireLock(mock.Anything, mock.Anything, mock.Anything).Return(false, nil).Once()
	uc := newTestSubmitUseCase(idemSvc, lockSvc, nil, nil)

	_, err := uc.Execute(context.Background(), SubmitRequest{IdempotencyKey: "key-1", SessionID: "sess-1", Answers: validAnswers()})
	if !errors.Is(err, application.ErrLockNotAcquired) {
		t.Fatalf("expected ErrLockNotAcquired, got %v", err)
	}
}

func TestExecute_MemberQuotaExceeded_RejectsAndReleasesLock(t *testing.T) {
	idemSvc := mocks.NewMockIdempotencyService(t)
	idemSvc.EXPECT().Check(mock.Anything, mock.Anything, mock.Anything).Return(nil, nil).Once()
	lockSvc := mocks.NewMockDistributedLockService(t)
	lockSvc.EXPECT().AcquireLock(mock.Anything, mock.Anything, mock.Anything).Return(true, nil).Once()
	lockSvc.EXPECT().ReleaseLock(mock.Anything, mock.Anything).Return(nil).Once()
	trRepo := mocks.NewMockTestResultRepository(t)
	trRepo.EXPECT().CountMonthlyUsage(mock.Anything, "user-1").Return(int64(application.MemberMonthlyQuota), nil).Once()
	uc := newTestSubmitUseCase(idemSvc, lockSvc, trRepo, nil)

	_, err := uc.Execute(context.Background(), SubmitRequest{IdempotencyKey: "key-1", SessionID: "user-1", IsMember: true, Answers: validAnswers()})
	if !errors.Is(err, application.ErrQuotaExceeded) {
		t.Fatalf("expected ErrQuotaExceeded, got %v", err)
	}
}

// Guest quota (TICKET-19) is checked per session_id, independently of the
// Member quota — a guest with 1 completed/fallback_static result this month
// must be rejected.
func TestExecute_GuestQuotaExceeded_Rejects(t *testing.T) {
	idemSvc := mocks.NewMockIdempotencyService(t)
	idemSvc.EXPECT().Check(mock.Anything, mock.Anything, mock.Anything).Return(nil, nil).Once()
	lockSvc := mocks.NewMockDistributedLockService(t)
	lockSvc.EXPECT().AcquireLock(mock.Anything, mock.Anything, mock.Anything).Return(true, nil).Once()
	lockSvc.EXPECT().ReleaseLock(mock.Anything, mock.Anything).Return(nil).Once()
	trRepo := mocks.NewMockTestResultRepository(t)
	trRepo.EXPECT().CountMonthlyUsageByGuestSession(mock.Anything, "guest-session-1").Return(int64(application.GuestMonthlyQuota), nil).Once()
	uc := newTestSubmitUseCase(idemSvc, lockSvc, trRepo, nil)

	_, err := uc.Execute(context.Background(), SubmitRequest{IdempotencyKey: "key-1", SessionID: "guest-session-1", IsMember: false, Answers: validAnswers()})
	if !errors.Is(err, application.ErrQuotaExceeded) {
		t.Fatalf("expected ErrQuotaExceeded, got %v", err)
	}
}

func TestExecute_MemberUnderQuota_StillReachesLoadQuestions(t *testing.T) {
	idemSvc := mocks.NewMockIdempotencyService(t)
	idemSvc.EXPECT().Check(mock.Anything, mock.Anything, mock.Anything).Return(nil, nil).Once()
	lockSvc := mocks.NewMockDistributedLockService(t)
	lockSvc.EXPECT().AcquireLock(mock.Anything, mock.Anything, mock.Anything).Return(true, nil).Once()
	lockSvc.EXPECT().ReleaseLock(mock.Anything, mock.Anything).Return(nil).Once()
	trRepo := mocks.NewMockTestResultRepository(t)
	trRepo.EXPECT().CountMonthlyUsage(mock.Anything, "user-1").Return(int64(application.MemberMonthlyQuota-1), nil).Once()
	uc := newTestSubmitUseCase(idemSvc, lockSvc, trRepo, nil)
	// questionRepo mock returns an error the instant it's called — proving
	// the quota gate passed control past the quota check and reached
	// loadQuestions, without needing to fake the rest of Execute's DB path.
	questionRepo := mocks.NewMockQuestionRepository(t)
	questionRepo.EXPECT().FindByIDs(mock.Anything, mock.Anything).Return(nil, errors.New("boundary reached")).Once()
	uc.questionRepo = questionRepo

	_, err := uc.Execute(context.Background(), SubmitRequest{IdempotencyKey: "key-1", SessionID: "user-1", IsMember: true, Answers: validAnswers()})
	if err == nil {
		t.Fatal("expected an error once loadQuestions is reached")
	}
}

// --- runAIPhase: pure Gemini-outcome logic, no DB involved ---

func TestRunAIPhase_NoEssays_SkipsGeminiEntirely(t *testing.T) {
	uc := newTestSubmitUseCase(nil, nil, nil, nil) // aiSvc nil: any call panics

	outcome := uc.runAIPhase(context.Background(), "result-1", nil, "en")
	if outcome.status != testresult.StatusFallbackStatic {
		t.Fatalf("expected fallback_static status, got %s", outcome.status)
	}
	if outcome.called {
		t.Fatal("expected outcome.called=false")
	}
}

// FR-C2: a Gemini transport/API error must degrade to fallback_static, not
// propagate as an error to the caller.
func TestRunAIPhase_GeminiError_DegradesToFallbackStatic(t *testing.T) {
	aiSvc := mocks.NewMockAIGeneratorService(t)
	aiSvc.EXPECT().GenerateSummary(mock.Anything, mock.Anything, "en").Return("", "", 0, errors.New("gemini: upstream 503")).Once()
	uc := newTestSubmitUseCase(nil, nil, nil, aiSvc)

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
	aiSvc := mocks.NewMockAIGeneratorService(t)
	aiSvc.EXPECT().GenerateSummary(mock.Anything, mock.Anything, "en").Return("too short", "prompt", 10, nil).Once() // below aivalidator.minLength
	uc := newTestSubmitUseCase(nil, nil, nil, aiSvc)

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
	aiSvc := mocks.NewMockAIGeneratorService(t)
	aiSvc.EXPECT().GenerateSummary(mock.Anything, mock.Anything, "en").Return(longSummary, "prompt", 123, nil).Once()
	uc := newTestSubmitUseCase(nil, nil, nil, aiSvc)

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
