package testresult

import (
	"time"
)

// ResultStatus represents the processing state of a test result.
type ResultStatus string

// The lifecycle states a test result's AI-summary processing moves through.
const (
	StatusProcessing     ResultStatus = "processing"
	StatusCompleted      ResultStatus = "completed"
	StatusFallbackStatic ResultStatus = "fallback_static"
)

// PDFStatus represents the async PDF generation lifecycle.
type PDFStatus string

// The lifecycle states async PDF generation moves through.
const (
	PDFStatusPending    PDFStatus = "pending"
	PDFStatusProcessing PDFStatus = "processing"
	PDFStatusCompleted  PDFStatus = "completed"
	PDFStatusFailed     PDFStatus = "failed"
)

// TestResult represents the outcome of a psychological assessment.
type TestResult struct {
	ID               string
	UserID           *string
	GuestSessionID   *string
	ShareToken       string
	Locale           string
	MascotStyle      string
	MBTIType         string
	GritScore        int
	TraitScores      map[string]interface{}
	AISummaryText    *string
	Status           ResultStatus
	WellbeingFlag    bool
	PDFUrl           *string
	PDFStatus        PDFStatus
	PromptTokens     *int
	CompletionTokens *int
	TotalTokens      *int
	CreatedAt        time.Time
	ExpiresAt        *time.Time
}

// IsExpired returns true if a Guest result has passed its 14-day TTL.
func (r *TestResult) IsExpired() bool {
	if r.ExpiresAt == nil {
		return false
	}
	return time.Now().After(*r.ExpiresAt)
}
