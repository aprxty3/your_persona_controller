package assessment

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/aprxty3/your_persona_controller.git/internal/application"
	"github.com/aprxty3/your_persona_controller.git/internal/application/assessment/dto"
	"github.com/aprxty3/your_persona_controller.git/internal/application/assessment/mocks"
	"github.com/aprxty3/your_persona_controller.git/internal/domain/testresult"
	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
	"github.com/stretchr/testify/mock"
)

func testLogger() logger.Logger { return logger.NewLogger("test") }

// allowingIPLimiter returns an IPRateLimiter mock that always allows — the
// default every test not specifically exercising the TICKET-22 rate-limit
// gate itself gets, since Execute now checks it unconditionally up front.
func allowingIPLimiter(t *testing.T) *mocks.MockIPRateLimiter {
	limiter := mocks.NewMockIPRateLimiter(t)
	limiter.EXPECT().Allow(mock.Anything, application.ScopeSubmitIP, mock.Anything).Return(true, 0, nil).Maybe()
	return limiter
}

// newTestSubmitUseCase wires only the collaborators a given test actually
// exercises — mockery mocks panic on any unexpected call, so leaving a
// dependency nil is itself an assertion that the code path never touches it.
// The IP rate limiter always allows here; tests for the rate-limit gate
// itself construct SubmitAssessmentUseCase directly instead.
func newTestSubmitUseCase(t *testing.T, idemSvc IdempotencyService, lockSvc DistributedLockService, trRepo TestResultRepository, aiSvc AIGeneratorService) *SubmitAssessmentUseCase {
	return &SubmitAssessmentUseCase{
		testResultRepo: trRepo,
		aiSvc:          aiSvc,
		lockSvc:        lockSvc,
		idempotencySvc: idemSvc,
		ipRateLimiter:  allowingIPLimiter(t),
		log:            testLogger(),
	}
}

func validAnswers() []AnswerInput {
	return []AnswerInput{{QuestionID: "q1", Value: "4"}}
}

func TestExecute_EmptyAnswers_RejectedBeforeAnyDependency(t *testing.T) {
	uc := newTestSubmitUseCase(t, nil, nil, nil, nil)

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
	uc := newTestSubmitUseCase(t, idemSvc, nil, nil, nil)

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
	uc := newTestSubmitUseCase(t, idemSvc, nil, nil, nil)

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
	uc := newTestSubmitUseCase(t, idemSvc, lockSvc, nil, nil)

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
	uc := newTestSubmitUseCase(t, idemSvc, lockSvc, trRepo, nil)

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
	uc := newTestSubmitUseCase(t, idemSvc, lockSvc, trRepo, nil)

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
	uc := newTestSubmitUseCase(t, idemSvc, lockSvc, trRepo, nil)
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

// --- TICKET-22: per-IP rate limit ---

func TestExecute_RateLimited_RejectsBeforeIdempotencyCheck(t *testing.T) {
	limiter := mocks.NewMockIPRateLimiter(t)
	limiter.EXPECT().Allow(mock.Anything, application.ScopeSubmitIP, "1.2.3.4").Return(false, 1800, nil).Once()
	uc := &SubmitAssessmentUseCase{ipRateLimiter: limiter, log: testLogger()} // idempotencySvc nil: must never be reached

	resp, err := uc.Execute(context.Background(), SubmitRequest{IdempotencyKey: "key-1", SessionID: "sess-1", Answers: validAnswers(), IPAddress: "1.2.3.4"})
	if !errors.Is(err, application.ErrRateLimited) {
		t.Fatalf("expected ErrRateLimited, got %v", err)
	}
	if resp == nil || resp.RetryAfterSeconds != 1800 {
		t.Fatalf("expected RetryAfterSeconds=1800, got %+v", resp)
	}
}

func TestExecute_RateLimiterRedisError_FailsOpenAndProceeds(t *testing.T) {
	limiter := mocks.NewMockIPRateLimiter(t)
	limiter.EXPECT().Allow(mock.Anything, application.ScopeSubmitIP, mock.Anything).Return(false, 0, errors.New("redis down")).Once()
	idemSvc := mocks.NewMockIdempotencyService(t)
	idemSvc.EXPECT().Check(mock.Anything, mock.Anything, mock.Anything).Return(nil, application.ErrIdempotencyKeyReused).Once()
	uc := &SubmitAssessmentUseCase{ipRateLimiter: limiter, idempotencySvc: idemSvc, log: testLogger()}

	_, err := uc.Execute(context.Background(), SubmitRequest{IdempotencyKey: "key-1", SessionID: "sess-1", Answers: validAnswers()})
	if !errors.Is(err, application.ErrIdempotencyKeyReused) {
		t.Fatalf("expected the Redis error to fail open and reach the idempotency check, got %v", err)
	}
}

// --- filterGarbageEssays: pure selection logic, no DB involved (TICKET-23) ---

func TestFilterGarbageEssays_AllGarbage_ReturnsEmpty(t *testing.T) {
	essays := []string{strings.Repeat("a", 500), "!!!###$$$%%%^^^&&&***((()))"}
	kept := filterGarbageEssays(essays, "result-1", testLogger())
	if len(kept) != 0 {
		t.Fatalf("expected all essays filtered out, got %v", kept)
	}
}

func TestFilterGarbageEssays_AllLegit_ReturnsAllUnchanged(t *testing.T) {
	essays := []string{
		"I really enjoyed working on this team project a lot.",
		"Saya senang bekerja dalam tim untuk proyek ini kemarin.",
	}
	kept := filterGarbageEssays(essays, "result-1", testLogger())
	if len(kept) != len(essays) {
		t.Fatalf("expected both legit essays kept, got %v", kept)
	}
}

func TestFilterGarbageEssays_Mixed_KeepsOnlyLegitOnes(t *testing.T) {
	legit := "I really enjoyed working on this team project a lot."
	garbage := strings.Repeat("a", 500)
	kept := filterGarbageEssays([]string{legit, garbage}, "result-1", testLogger())
	if len(kept) != 1 || kept[0] != legit {
		t.Fatalf("expected only the legit essay kept, got %v", kept)
	}
}

func TestFilterGarbageEssays_EmptyInput_ReturnsEmpty(t *testing.T) {
	kept := filterGarbageEssays(nil, "result-1", testLogger())
	if len(kept) != 0 {
		t.Fatalf("expected empty result for empty input, got %v", kept)
	}
}

// --- runAIPhase: pure Gemini-outcome logic, no DB involved ---

func TestRunAIPhase_NoEssays_SkipsGeminiEntirely(t *testing.T) {
	uc := newTestSubmitUseCase(t, nil, nil, nil, nil) // aiSvc nil: any call panics

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
	uc := newTestSubmitUseCase(t, nil, nil, nil, aiSvc)

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
	uc := newTestSubmitUseCase(t, nil, nil, nil, aiSvc)

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
	uc := newTestSubmitUseCase(t, nil, nil, nil, aiSvc)

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
