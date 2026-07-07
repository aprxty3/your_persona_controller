package assessment

import (
	"context"
	"errors"
	"time"

	"github.com/aprxty3/your_persona_controller.git/internal/domain/answer"
	"github.com/aprxty3/your_persona_controller.git/internal/domain/testresult"
)

// DTOs for the Data Transfer between Handler and Usecase
type AnswerInput struct {
	QuestionID string
	Value      string
}

type SubmitRequest struct {
	IdempotencyKey string
	SessionID      string // Can be GuestSessionID or UserID based on auth state
	IsMember       bool
	Locale         string
	Answers        []AnswerInput
}

type SubmitResponse struct {
	ResultID      string
	MBTIType      string
	GritScore     int
	AISummaryText string
	WellbeingFlag bool
	Status        string
}

// -------------------------------------------------------------------------
// DEPENDENCY INTERFACES
// The Usecase defines what it needs. Infrastructure will provide the concrete implementation.
// -------------------------------------------------------------------------

type TestResultRepository interface {
	Create(ctx context.Context, result *testresult.TestResult) error
	CountMonthlyUsage(ctx context.Context, userID string) (int64, error)
}

type AnswerRepository interface {
	UpsertAnswers(ctx context.Context, testResultID string, answers []answer.Answer) error
}

type AIGeneratorService interface {
	// GenerateSummary calls Gemini API.
	// We pass the locale to enforce FR-I6 (Locale-aware prompt).
	GenerateSummary(ctx context.Context, text string, locale string) (summary string, tokens int, err error)
}

type DistributedLockService interface {
	AcquireLock(ctx context.Context, key string, ttl time.Duration) (bool, error)
	ReleaseLock(ctx context.Context, key string) error
}

type IdempotencyService interface {
	Check(ctx context.Context, key string, payloadHash string) (*SubmitResponse, error)
	Save(ctx context.Context, key string, payloadHash string, response *SubmitResponse, ttl time.Duration) error
}

type PDFQueueService interface {
	EnqueueGeneratePDF(ctx context.Context, testResultID string) error
}

// -------------------------------------------------------------------------
// USECASE IMPLEMENTATION
// -------------------------------------------------------------------------

type SubmitAssessmentUseCase struct {
	testResultRepo TestResultRepository
	answerRepo     AnswerRepository
	aiSvc          AIGeneratorService
	lockSvc        DistributedLockService
	idempotencySvc IdempotencyService
	pdfQueue       PDFQueueService
}

// NewSubmitAssessmentUseCase acts as the constructor for Dependency Injection (Wire).
func NewSubmitAssessmentUseCase(
	trRepo TestResultRepository,
	ansRepo AnswerRepository,
	aiSvc AIGeneratorService,
	lockSvc DistributedLockService,
	idemSvc IdempotencyService,
	pdfQueue PDFQueueService,
) *SubmitAssessmentUseCase {
	return &SubmitAssessmentUseCase{
		testResultRepo: trRepo,
		answerRepo:     ansRepo,
		aiSvc:          aiSvc,
		lockSvc:        lockSvc,
		idempotencySvc: idemSvc,
		pdfQueue:       pdfQueue,
	}
}

// Execute orchestrates the entire assessment submission flow.
func (uc *SubmitAssessmentUseCase) Execute(ctx context.Context, req SubmitRequest) (*SubmitResponse, error) {
	// 1. Idempotency Check
	// (Implementation logic: Hash the answers payload, check Redis. If exists, return cached response)

	// 2. Distributed Lock for Quota
	// (Implementation logic: Acquire lock using SessionID/UserID. If failed, return error "Please wait")

	// 3. Quota Validation
	// (Implementation logic: If IsMember, check CountMonthlyUsage < 3)

	// 4. Wellbeing Safety Net Check
	// (Implementation logic: Scan essay inputs for crisis keywords)
	isWellbeingFlagged := false // Placeholder

	// 5. AI Processing with 2-Phase Context Cancellation
	// We don't want to burn tokens if the user disconnects while waiting in the semaphore queue.
	// But ONCE the request is in-flight, we use context.WithoutCancel to finish the job.
	aiCtx := context.WithoutCancel(ctx)
	aiSummary, _, err := uc.aiSvc.GenerateSummary(aiCtx, "combined_essay_text_here", req.Locale)
	status := "completed"
	if err != nil {
		// Graceful Degradation (FR-C2): Fallback to static result if AI fails
		status = "fallback_static"
		aiSummary = "Static fallback text..."
	}

	// 6. Save TestResult to Database
	result := &testresult.TestResult{
		// Populate mapping here...
		Locale:        req.Locale,
		AISummaryText: &aiSummary,
		Status:        testresult.ResultStatus(status),
		WellbeingFlag: isWellbeingFlagged,
	}
	if err := uc.testResultRepo.Create(aiCtx, result); err != nil {
		return nil, errors.New("failed to save test result")
	}

	// 7. Upsert Answers
	// (Implementation logic: Map req.Answers to domain entities, call uc.answerRepo.UpsertAnswers)

	// 8. Enqueue PDF Generation Job (Asynchronous)
	_ = uc.pdfQueue.EnqueueGeneratePDF(aiCtx, result.ID) // Fire and forget or log error

	// 9. Save to Idempotency Cache (TTL 24h)
	resp := &SubmitResponse{
		ResultID:      result.ID,
		AISummaryText: aiSummary,
		WellbeingFlag: isWellbeingFlagged,
		Status:        status,
	}
	// uc.idempotencySvc.Save(...)

	return resp, nil
}
