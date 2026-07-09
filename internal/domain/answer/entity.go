package answer

import (
	"time"
)

// Answer represents a single response to a question in an assessment.
// The pair (TestResultID, QuestionID) has a composite UNIQUE constraint — see ERD.
// Revisions via back-button MUST use upsert (ON CONFLICT DO UPDATE), not a new insert (FR-B10).
type Answer struct {
	ID           string
	TestResultID string
	QuestionID   string
	Value        string // "A"–"E" for SJT, "1"–"5" for Likert, free text for essay
	CreatedAt    time.Time
	UpdatedAt    time.Time
}
