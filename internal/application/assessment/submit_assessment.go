package assessment

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/aprxty3/your_persona_controller.git/internal/application"
	"github.com/aprxty3/your_persona_controller.git/internal/domain/account"
	"github.com/aprxty3/your_persona_controller.git/internal/domain/content"
	"github.com/aprxty3/your_persona_controller.git/internal/domain/testresult"
	pgaccount "github.com/aprxty3/your_persona_controller.git/internal/infrastructure/persistence/postgres/account"
	pgassessment "github.com/aprxty3/your_persona_controller.git/internal/infrastructure/persistence/postgres/assessment"
	"github.com/aprxty3/your_persona_controller.git/pkg/aivalidator"
	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	essayMaxLength       = 4000
	quotaLockTTL         = 20 * time.Second
	idempotencyTTL       = 24 * time.Hour
	promptAuditRetention = 30 * 24 * time.Hour
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

type TestResultRepository interface {
	CountMonthlyUsage(ctx context.Context, userID string) (int64, error)
	CountMonthlyUsageByGuestSession(ctx context.Context, guestSessionID string) (int64, error)
	CountCompletedByUser(ctx context.Context, userID string) (int64, error)
}

type QuestionRepository interface {
	FindByIDs(ctx context.Context, ids []string) ([]content.Question, error)
}

type AIGeneratorService interface {
	GenerateSummary(ctx context.Context, text string, locale string) (summary string, rawPrompt string, tokens int, err error)
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

type SubmitAssessmentUseCase struct {
	db             *gorm.DB
	testResultRepo TestResultRepository
	questionRepo   QuestionRepository
	userRepo       account.UserRepository
	aiSvc          AIGeneratorService
	lockSvc        DistributedLockService
	idempotencySvc IdempotencyService
	pdfQueue       PDFQueueService
	log            logger.Logger
}

// NewSubmitAssessmentUseCase acts as the constructor for Dependency Injection (Wire).
func NewSubmitAssessmentUseCase(
	db *gorm.DB,
	trRepo TestResultRepository,
	questionRepo QuestionRepository,
	userRepo account.UserRepository,
	aiSvc AIGeneratorService,
	lockSvc DistributedLockService,
	idemSvc IdempotencyService,
	pdfQueue PDFQueueService,
	log logger.Logger,
) *SubmitAssessmentUseCase {
	return &SubmitAssessmentUseCase{
		db:             db,
		testResultRepo: trRepo,
		questionRepo:   questionRepo,
		userRepo:       userRepo,
		aiSvc:          aiSvc,
		lockSvc:        lockSvc,
		idempotencySvc: idemSvc,
		pdfQueue:       pdfQueue,
		log:            log.With("usecase", "submit_assessment"),
	}
}

// Execute orchestrates the entire assessment submission flow.
func (uc *SubmitAssessmentUseCase) Execute(ctx context.Context, req SubmitRequest) (*SubmitResponse, error) {
	if len(req.Answers) == 0 {
		return nil, fmt.Errorf("%w: answers is required", application.ErrInvalidInput)
	}

	payloadHash := hashAnswers(req.Answers)
	idemKey := "idempotency_key:" + req.IdempotencyKey

	if cached, err := uc.idempotencySvc.Check(ctx, idemKey, payloadHash); err != nil {
		if errors.Is(err, application.ErrIdempotencyKeyReused) {
			return nil, err
		}
		uc.log.Warn("idempotency check failed, proceeding without cache", "error", err)
	} else if cached != nil {
		return cached, nil
	}

	lockKey := "quota_lock:" + req.SessionID
	acquired, err := uc.lockSvc.AcquireLock(ctx, lockKey, quotaLockTTL)
	if err != nil {
		return nil, fmt.Errorf("submit: acquire lock: %w", err)
	}
	if !acquired {
		return nil, application.ErrLockNotAcquired
	}
	defer func() {
		if releaseErr := uc.lockSvc.ReleaseLock(ctx, lockKey); releaseErr != nil {
			uc.log.Warn("release lock failed", "key", lockKey, "error", releaseErr)
		}
	}()

	if req.IsMember {
		usage, err := uc.testResultRepo.CountMonthlyUsage(ctx, req.SessionID)
		if err != nil {
			return nil, fmt.Errorf("submit: count monthly usage: %w", err)
		}
		if usage >= application.MemberMonthlyQuota {
			return nil, application.ErrQuotaExceeded
		}
	} else {
		// Soft Guest quota per session_id: clearing the cookie
		// mints a new session and resets this — accepted by design; the check
		// only blocks the same session spamming paid Gemini calls.
		usage, err := uc.testResultRepo.CountMonthlyUsageByGuestSession(ctx, req.SessionID)
		if err != nil {
			return nil, fmt.Errorf("submit: count guest monthly usage: %w", err)
		}
		if usage >= application.GuestMonthlyQuota {
			return nil, application.ErrQuotaExceeded
		}
	}

	questionByID, err := uc.loadQuestions(ctx, req.Answers)
	if err != nil {
		return nil, err
	}

	essayTexts, err := extractAndValidateEssays(req.Answers, questionByID)
	if err != nil {
		return nil, err
	}

	isWellbeingFlagged := scanForCrisisLanguage(essayTexts, req.Locale)
	resultID := uuid.New().String()
	aiCtx := context.WithoutCancel(ctx)
	outcome := uc.runAIPhase(aiCtx, resultID, essayTexts, req.Locale)

	// Scores are pure math over the answers — computed for every outcome,
	// including fallback_static, since they don't depend on Gemini.
	scores := ComputeScores(req.Answers, questionByID)
	for _, dim := range scores.NeutralFallbackDimensions {
		uc.log.Warn("scoring: no valid answer for dimension, defaulted to neutral 50", "dimension", dim, "result_id", resultID)
	}

	referredByCode, referralEligible := uc.checkReferralEligibility(ctx, req)

	result := &testresult.TestResult{
		ID:            resultID,
		Locale:        req.Locale,
		MascotStyle:   MascotStyleA, // visual-only default; changeable later via PATCH /v1/results/:id/mascot-style (TICKET-04)
		ShareToken:    uuid.New().String(),
		MBTIType:      scores.MBTIType,
		GritScore:     scores.GritScore,
		TraitScores:   scores.TraitScores,
		AISummaryText: &outcome.summary,
		Status:        outcome.status,
		WellbeingFlag: isWellbeingFlagged,
		PDFStatus:     testresult.PDFStatusPending,
		TotalTokens:   outcome.totalTokens,
		CreatedAt:     time.Now(),
	}
	if req.IsMember {
		result.UserID = &req.SessionID
	} else {
		result.GuestSessionID = &req.SessionID
		expires := time.Now().Add(application.GuestDataRetention)
		result.ExpiresAt = &expires
	}

	answers := make([]testresult.Answer, len(req.Answers))
	for i, a := range req.Answers {
		answers[i] = testresult.Answer{
			ID:           uuid.New().String(),
			TestResultID: resultID,
			QuestionID:   a.QuestionID,
			Value:        a.Value,
		}
	}

	txErr := uc.db.Transaction(func(tx *gorm.DB) error {
		if err := pgassessment.NewTestResultRepository(tx, uc.log).Create(ctx, result); err != nil {
			return fmt.Errorf("tx: create test result: %w", err)
		}
		if err := pgassessment.NewAnswerRepository(tx, uc.log).UpsertAnswers(ctx, resultID, answers); err != nil {
			return fmt.Errorf("tx: upsert answers: %w", err)
		}

		if outcome.called {
			auditLog := &testresult.PromptAuditLog{
				ID:             uuid.New().String(),
				TestResultID:   resultID,
				RawPrompt:      outcome.rawPrompt,
				RawResponse:    outcome.rawResponse,
				FlaggedAnomaly: outcome.flaggedAnomaly,
				CreatedAt:      time.Now(),
				ExpiresAt:      time.Now().Add(promptAuditRetention),
			}
			if err := pgassessment.NewPromptAuditLogRepository(tx, uc.log).Create(ctx, auditLog); err != nil {
				return fmt.Errorf("tx: create audit log: %w", err)
			}
		}

		if referralEligible {
			referralRepo := pgaccount.NewReferralRepository(tx, uc.log)
			code, err := referralRepo.FindCodeByCode(ctx, *referredByCode)
			if err != nil {
				return fmt.Errorf("tx: find referral code: %w", err)
			}
			if code == nil {
				uc.log.Warn("referral code not found, skipping event", "code", *referredByCode)
			} else {
				event := &account.ReferralEvent{
					ID:             uuid.New().String(),
					ReferralCodeID: code.ID,
					ReferredUserID: req.SessionID,
					EventType:      account.EventTypeTestCompleted,
					CreatedAt:      time.Now(),
				}
				if err := referralRepo.CreateEvent(ctx, event); err != nil {
					return fmt.Errorf("tx: create referral event: %w", err)
				}
			}
		}

		return nil
	})
	if txErr != nil {
		uc.log.Error("submit failed", "step", "transaction", "result_id", resultID, "error", txErr)
		return nil, fmt.Errorf("submit: %w", txErr)
	}

	if err := uc.pdfQueue.EnqueueGeneratePDF(ctx, resultID); err != nil {
		uc.log.Warn("enqueue pdf generation failed", "result_id", resultID, "error", err)
	}

	resp := &SubmitResponse{
		ResultID:      resultID,
		MBTIType:      result.MBTIType,
		GritScore:     result.GritScore,
		AISummaryText: outcome.summary,
		WellbeingFlag: isWellbeingFlagged,
		Status:        string(outcome.status),
	}

	if err := uc.idempotencySvc.Save(ctx, idemKey, payloadHash, resp, idempotencyTTL); err != nil {
		uc.log.Warn("save idempotency cache failed", "key", idemKey, "error", err)
	}

	return resp, nil
}

// loadQuestions bulk-fetches question metadata for every answered question in a single query.
func (uc *SubmitAssessmentUseCase) loadQuestions(ctx context.Context, answers []AnswerInput) (map[string]content.Question, error) {
	ids := make([]string, len(answers))
	for i, a := range answers {
		ids[i] = a.QuestionID
	}

	questions, err := uc.questionRepo.FindByIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("submit: load questions: %w", err)
	}

	byID := make(map[string]content.Question, len(questions))
	for _, q := range questions {
		byID[q.ID] = q
	}
	return byID, nil
}

// extractAndValidateEssays enforces the per-essay character cap and returns the essay-type answer values in submission order.
func extractAndValidateEssays(answers []AnswerInput, questionByID map[string]content.Question) ([]string, error) {
	var essays []string
	for _, a := range answers {
		q, ok := questionByID[a.QuestionID]
		if !ok {
			return nil, fmt.Errorf("%w: unknown question_id %q", application.ErrInvalidInput, a.QuestionID)
		}
		if q.Type != content.TypeEssayPrompt {
			continue
		}
		if err := application.ValidateMaxLength("answer.value", a.Value, essayMaxLength); err != nil {
			return nil, err
		}
		essays = append(essays, a.Value)
	}
	return essays, nil
}

// checkReferralEligibility returns the code the user registered with and whether this submission qualifies
func (uc *SubmitAssessmentUseCase) checkReferralEligibility(ctx context.Context, req SubmitRequest) (*string, bool) {
	if !req.IsMember {
		return nil, false
	}

	user, err := uc.userRepo.FindByID(ctx, req.SessionID)
	if err != nil {
		uc.log.Warn("referral eligibility check failed, skipping", "error", err)
		return nil, false
	}
	if user == nil || user.ReferredByCode == nil {
		return nil, false
	}

	priorCount, err := uc.testResultRepo.CountCompletedByUser(ctx, req.SessionID)
	if err != nil {
		uc.log.Warn("referral eligibility check failed, skipping", "error", err)
		return nil, false
	}
	if priorCount > 0 {
		return nil, false
	}

	return user.ReferredByCode, true
}

// aiOutcome captures everything Execute needs from the AI phase, regardless
// of whether Gemini was actually called or the call succeeded.
type aiOutcome struct {
	called         bool // true if Gemini was actually invoked (essays were present)
	summary        string
	status         testresult.ResultStatus
	rawPrompt      string
	rawResponse    string
	flaggedAnomaly bool
	totalTokens    *int
}

// runAIPhase calls Gemini (if there's essay content to analyze), validates
// the output, and degrades to a static fallback on any failure.
func (uc *SubmitAssessmentUseCase) runAIPhase(ctx context.Context, resultID string, essayTexts []string, locale string) aiOutcome {
	if len(essayTexts) == 0 {
		return aiOutcome{status: testresult.StatusFallbackStatic, summary: fallbackText(locale)}
	}

	combinedEssay := strings.Join(essayTexts, "\n\n")
	summary, rawPrompt, tokens, err := uc.aiSvc.GenerateSummary(ctx, combinedEssay, locale)

	outcome := aiOutcome{called: true, rawPrompt: rawPrompt}

	switch {
	case err != nil:
		uc.log.Warn("gemini call failed, falling back to static result", "result_id", resultID, "error", err)
		outcome.status = testresult.StatusFallbackStatic
		outcome.summary = fallbackText(locale)
		outcome.flaggedAnomaly = true
		outcome.rawResponse = "ERROR: " + err.Error()
	default:
		if valErr := aivalidator.ValidateOutput(summary, locale); valErr != nil {
			uc.log.Warn("gemini output failed validation, falling back to static result", "result_id", resultID, "error", valErr)
			outcome.status = testresult.StatusFallbackStatic
			outcome.summary = fallbackText(locale)
			outcome.flaggedAnomaly = true
			outcome.rawResponse = summary
		} else {
			outcome.status = testresult.StatusCompleted
			outcome.summary = summary
			outcome.rawResponse = summary
			outcome.totalTokens = &tokens
		}
	}

	return outcome
}

// hashAnswers deterministically hashes the answer set regardless of submission order.
func hashAnswers(answers []AnswerInput) string {
	sorted := make([]AnswerInput, len(answers))
	copy(sorted, answers)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].QuestionID < sorted[j].QuestionID })

	h := sha256.New()
	for _, a := range sorted {
		h.Write([]byte(a.QuestionID))
		h.Write([]byte{0})
		h.Write([]byte(a.Value))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}
