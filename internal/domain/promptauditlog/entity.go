package promptauditlog

import (
	"time"
)

// PromptAuditLog records every Gemini API call for security auditing.
// Retention: 30 days (TTL via expires_at). Deleted earlier by the anonymization
// worker when a user requests data deletion (PRD Section 9.3, FR-C6).
//
// Retry logic (FR-C2): a single TEST_RESULT may have up to 2 audit entries if
// Gemini fails on the first attempt and a retry is made. This is intentional
// and documented in the ERD.
//
// SECURITY: raw_prompt includes the full system_instruction + user essay content.
// This table MUST NOT be exposed via any public-facing API endpoint.
type PromptAuditLog struct {
	ID             string
	TestResultID   string
	RawPrompt      string // full prompt sent to Gemini (system_instruction + user content)
	RawResponse    string // raw response before any processing or validation
	FlaggedAnomaly bool   // true if FR-C5 output validation detected anomaly/refusal pattern
	CreatedAt      time.Time
	ExpiresAt      time.Time // created_at + 30 days
}
