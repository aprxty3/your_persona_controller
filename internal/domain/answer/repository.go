package answer

import (
	"context"
)

// Repository defines the contract for Answer data persistence.
type Repository interface {
	// UpsertAnswers inserts or updates answers for a given test result.
	// Must use ON CONFLICT (test_result_id, question_id) DO UPDATE to support
	// the back-button revision flow (FR-B10).
	UpsertAnswers(ctx context.Context, testResultID string, answers []Answer) error

	// FindByTestResultID retrieves all answers for a given test result.
	FindByTestResultID(ctx context.Context, testResultID string) ([]Answer, error)
}
