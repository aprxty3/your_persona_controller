package testresult

import (
	"context"
)

// Repository defines the contract for TestResult data persistence.
type Repository interface {
	Create(ctx context.Context, result *TestResult) error
	FindByID(ctx context.Context, id string) (*TestResult, error)
	FindByShareToken(ctx context.Context, shareToken string) (*TestResult, error)
	Update(ctx context.Context, result *TestResult) error
	CountMonthlyUsage(ctx context.Context, userID string) (int64, error)
	FindExpiredGuestResults(ctx context.Context) ([]TestResult, error)
	UpdatePDFStatus(ctx context.Context, id string, pdfURL *string, status PDFStatus) error
	ReassignGuestResults(ctx context.Context, userID, guestSessionID string) error
	CountCompletedByGuestSession(ctx context.Context, guestSessionID string) (int64, error)
	FindPDFURLsByUser(ctx context.Context, userID string) ([]string, error)
	ScrubPersonalDataByUser(ctx context.Context, userID string) error
}
