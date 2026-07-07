package answer

import (
	"context"
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

// Repository defines the contract for Answer data persistence.
type Repository interface {
	// UpsertAnswers inserts or updates answers for a given test result.
	// Must use ON CONFLICT (test_result_id, question_id) DO UPDATE to support
	// the back-button revision flow (FR-B10).
	UpsertAnswers(ctx context.Context, testResultID string, answers []Answer) error

	// FindByTestResultID retrieves all answers for a given test result.
	FindByTestResultID(ctx context.Context, testResultID string) ([]Answer, error)
}
