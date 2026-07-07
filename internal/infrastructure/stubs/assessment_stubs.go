package stubs

import (
	"context"
	"time"

	"github.com/aprxty3/your_persona_controller.git/internal/application/assessment"
	"github.com/aprxty3/your_persona_controller.git/internal/domain/testresult"
	"github.com/aprxty3/your_persona_controller.git/internal/domain/answer"
)

// StubTestResultRepository implements assessment.TestResultRepository
type StubTestResultRepository struct{}

func NewStubTestResultRepository() assessment.TestResultRepository {
	return &StubTestResultRepository{}
}

func (r *StubTestResultRepository) Create(ctx context.Context, result *testresult.TestResult) error {
	return nil
}

func (r *StubTestResultRepository) CountMonthlyUsage(ctx context.Context, userID string) (int64, error) {
	return 0, nil
}

// StubAnswerRepository implements assessment.AnswerRepository
type StubAnswerRepository struct{}

func NewStubAnswerRepository() assessment.AnswerRepository {
	return &StubAnswerRepository{}
}

func (r *StubAnswerRepository) UpsertAnswers(ctx context.Context, testResultID string, answers []answer.Answer) error {
	return nil
}

// StubDistributedLockService implements assessment.DistributedLockService
type StubDistributedLockService struct{}

func NewStubDistributedLockService() assessment.DistributedLockService {
	return &StubDistributedLockService{}
}

func (s *StubDistributedLockService) AcquireLock(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	return true, nil
}

func (s *StubDistributedLockService) ReleaseLock(ctx context.Context, key string) error {
	return nil
}

// StubIdempotencyService implements assessment.IdempotencyService
type StubIdempotencyService struct{}

func NewStubIdempotencyService() assessment.IdempotencyService {
	return &StubIdempotencyService{}
}

func (s *StubIdempotencyService) Check(ctx context.Context, key string, payloadHash string) (*assessment.SubmitResponse, error) {
	return nil, nil
}

func (s *StubIdempotencyService) Save(ctx context.Context, key string, payloadHash string, response *assessment.SubmitResponse, ttl time.Duration) error {
	return nil
}

// StubPDFQueueService implements assessment.PDFQueueService
type StubPDFQueueService struct{}

func NewStubPDFQueueService() assessment.PDFQueueService {
	return &StubPDFQueueService{}
}

func (s *StubPDFQueueService) EnqueueGeneratePDF(ctx context.Context, testResultID string) error {
	return nil
}
