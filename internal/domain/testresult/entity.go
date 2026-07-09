package testresult

import (
	"time"
)

// ResultStatus represents the processing state of a test result.
type ResultStatus string

const (
	StatusProcessing ResultStatus = "processing"

	StatusCompleted ResultStatus = "completed"

	StatusFallbackStatic ResultStatus = "fallback_static"
)

// PDFStatus represents the async PDF generation lifecycle.
type PDFStatus string

const (
	PDFStatusPending    PDFStatus = "pending"
	PDFStatusProcessing PDFStatus = "processing"
	PDFStatusCompleted  PDFStatus = "completed"
	PDFStatusFailed     PDFStatus = "failed"
)

// TestResult represents the outcome of a psychological assessment.
type TestResult struct {
	ID               string
	UserID           *string // nil if Guest
	GuestSessionID   *string // nil if Member
	ShareToken       string  // UUIDv4 / nanoid for public read-only link
	Locale           string  // snapshot at creation time
	MascotStyle      string  // "style_a" | "style_b" — purely visual
	MBTIType         string  // e.g. "INTJ"
	GritScore        int
	TraitScores      map[string]interface{} // E/I, S/N, T/F, J/P percentages
	AISummaryText    *string                // nil if fallback_static
	Status           ResultStatus
	WellbeingFlag    bool    // true if crisis language detected
	PDFUrl           *string // R2/MinIO object key — nil until worker completes
	PDFStatus        PDFStatus
	PromptTokens     *int // nil if fallback
	CompletionTokens *int // nil if fallback
	TotalTokens      *int // nil if fallback — used for cost tracking
	CreatedAt        time.Time
	ExpiresAt        *time.Time // Guest only — mirrors GUEST_SESSION TTL (14 days)
}

// IsExpired returns true if a Guest result has passed its 14-day TTL.
func (r *TestResult) IsExpired() bool {
	if r.ExpiresAt == nil {
		return false
	}
	return time.Now().After(*r.ExpiresAt)
}
