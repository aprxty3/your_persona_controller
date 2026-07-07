package testresult

import (
	"context"
	"time"
)

// TestResult represents the outcome of a psychological assessment.
type TestResult struct {
	ID               string
	UserID           *string
	GuestSessionID   *string
	ShareToken       string // UUIDv4 or nanoid for secure public sharing
	Locale           string
	MascotStyle      string
	MBTIType         string
	GritScore        int
	TraitScores      map[string]interface{} // JSON structure for E/I, S/N, T/F, J/P
	AISummaryText    *string
	Status           string
	WellbeingFlag    bool
	PDFURL           *string
	PDFStatus        string
	PromptTokens     *int
	CompletionTokens *int
	TotalTokens      *int
	CreatedAt        time.Time
	ExpiresAt        *time.Time
}

// Repository defines the contract for TestResult data persistence.
type Repository interface {
	Create(ctx context.Context, result *TestResult) error
	FindByID(ctx context.Context, id string) (*TestResult, error)
	FindByShareToken(ctx context.Context, shareToken string) (*TestResult, error)
	Update(ctx context.Context, result *TestResult) error
	CountMonthlyUsage(ctx context.Context, userID string) (int64, error)
}
