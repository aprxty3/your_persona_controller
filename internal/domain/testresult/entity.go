package testresult

import (
	"context"
	"time"
)

// ResultStatus represents the processing state of a test result.
type ResultStatus string

const (
	// StatusProcessing is set when the Gemini call is in flight.
	StatusProcessing ResultStatus = "processing"

	// StatusCompleted means Gemini returned a valid response and the result is ready.
	// This status COUNTS toward monthly quota (FR-F2, Section 9.1).
	StatusCompleted ResultStatus = "completed"

	// StatusFallbackStatic means Gemini failed and a static result was used (FR-C2).
	// This status ALSO COUNTS toward monthly quota — user still consumed a slot.
	StatusFallbackStatic ResultStatus = "fallback_static"
)

// PDFStatus represents the async PDF generation lifecycle.
type PDFStatus string

const (
	PDFStatusPending    PDFStatus = "pending"
	PDFStatusProcessing PDFStatus = "processing"
	PDFStatusCompleted  PDFStatus = "completed"
	// PDFStatusFailed signals FE to STOP polling immediately (FR-E5).
	PDFStatusFailed PDFStatus = "failed"
)

// TestResult represents the outcome of a psychological assessment.
//
// OWNERSHIP INVARIANT (per ERD & TECHNICAL_DOCUMENTATION.md):
// Exactly ONE of UserID / GuestSessionID must be non-null (XOR).
// On Guest→Member claim: set UserID, set GuestSessionID = nil.
// Trace of origin remains in GUEST_SESSION.claimed_by_user_id.
//
// QUOTA RULE (per PRD Section 9.1):
// Monthly quota counts rows WHERE status IN ('completed', 'fallback_static')
// AND created_at >= start of current month in Asia/Jakarta timezone.
type TestResult struct {
	ID               string
	UserID           *string      // nil if Guest
	GuestSessionID   *string      // nil if Member
	ShareToken       string       // UUIDv4 / nanoid for public read-only link (FR-D8)
	Locale           string       // snapshot at creation time (FR-I7)
	MascotStyle      string       // "style_a" | "style_b" — purely visual (FR-D11)
	MBTIType         string       // e.g. "INTJ"
	GritScore        int
	TraitScores      map[string]interface{} // E/I, S/N, T/F, J/P percentages
	AISummaryText    *string      // nil if fallback_static
	Status           ResultStatus
	WellbeingFlag    bool         // true if crisis language detected (FR-B11)
	PDFUrl           *string      // R2/MinIO object key — nil until worker completes
	PDFStatus        PDFStatus
	PromptTokens     *int         // nil if fallback
	CompletionTokens *int         // nil if fallback
	TotalTokens      *int         // nil if fallback — used for cost tracking (FR-C4)
	CreatedAt        time.Time
	ExpiresAt        *time.Time   // Guest only — mirrors GUEST_SESSION TTL (14 days)
}

// IsExpired returns true if a Guest result has passed its 14-day TTL.
// Endpoints MUST treat expired results as 404, even if the row still exists (PRD Section 9.6).
func (r *TestResult) IsExpired() bool {
	if r.ExpiresAt == nil {
		return false
	}
	return time.Now().After(*r.ExpiresAt)
}

// Repository defines the contract for TestResult data persistence.
type Repository interface {
	// Create inserts a new test result.
	Create(ctx context.Context, result *TestResult) error

	// FindByID retrieves a test result by its UUID.
	FindByID(ctx context.Context, id string) (*TestResult, error)

	// FindByShareToken retrieves a test result by its public share token (FR-D8).
	FindByShareToken(ctx context.Context, shareToken string) (*TestResult, error)

	// Update saves all mutable fields of the test result.
	Update(ctx context.Context, result *TestResult) error

	// CountMonthlyUsage counts completed/fallback_static results for the given user
	// in the current month, calculated in Asia/Jakarta timezone (PRD Section 9.1).
	// userID accepts both a USER.id or a GUEST_SESSION.session_id depending on caller.
	CountMonthlyUsage(ctx context.Context, userID string) (int64, error)

	// FindExpiredGuestResults returns results owned by a guest session that have
	// passed their expires_at. Used by the daily Guest TTL purge job (FR-G5, PRD 9.6).
	FindExpiredGuestResults(ctx context.Context) ([]TestResult, error)

	// UpdatePDFStatus updates pdf_url and pdf_status after async generation.
	UpdatePDFStatus(ctx context.Context, id string, pdfURL *string, status PDFStatus) error
}
