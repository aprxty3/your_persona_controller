package testresult

import (
	"context"
)

// TestResultRepository defines the contract for TestResult data persistence.
type TestResultRepository interface {
	Create(ctx context.Context, result *TestResult) error
	FindByID(ctx context.Context, id string) (*TestResult, error)
	FindByShareToken(ctx context.Context, shareToken string) (*TestResult, error)
	Update(ctx context.Context, result *TestResult) error
	CountMonthlyUsage(ctx context.Context, userID string) (int64, error)
	CountMonthlyUsageByGuestSession(ctx context.Context, guestSessionID string) (int64, error)
	CountCompletedByUser(ctx context.Context, userID string) (int64, error)
	FindExpiredGuestResults(ctx context.Context) ([]TestResult, error)
	UpdatePDFStatus(ctx context.Context, id string, pdfURL *string, status PDFStatus) error
	ReassignGuestResults(ctx context.Context, userID, guestSessionID string) error
	CountCompletedByGuestSession(ctx context.Context, guestSessionID string) (int64, error)
	FindPDFURLsByUser(ctx context.Context, userID string) ([]string, error)
	ScrubPersonalDataByUser(ctx context.Context, userID string) error
	FindHistoryByUser(ctx context.Context, userID string, page, limit int) (results []TestResult, total int64, err error)
	DeleteByID(ctx context.Context, id string) error
}

// AnswerRepository defines the contract for Answer data persistence.
type AnswerRepository interface {
	UpsertAnswers(ctx context.Context, testResultID string, answers []Answer) error
	FindByTestResultID(ctx context.Context, testResultID string) ([]Answer, error)
	DeleteByTestResultID(ctx context.Context, testResultID string) error
	ScrubEssayAnswersByUser(ctx context.Context, userID string) error
}

// PromptAuditLogRepository defines the contract for PromptAuditLog data persistence.
type PromptAuditLogRepository interface {
	Create(ctx context.Context, log *PromptAuditLog) error
	DeleteByTestResultID(ctx context.Context, testResultID string) error
	DeleteExpired(ctx context.Context) (int64, error)
	DeleteByUserID(ctx context.Context, userID string) error
}
