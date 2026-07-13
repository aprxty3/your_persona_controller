package testresult

import (
	"time"
)

// PromptAuditLog records every Gemini API call for security auditing.
type PromptAuditLog struct {
	ID             string
	TestResultID   string
	RawPrompt      string
	RawResponse    string
	FlaggedAnomaly bool
	CreatedAt      time.Time
	ExpiresAt      time.Time
}
