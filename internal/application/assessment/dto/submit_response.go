// Package dto holds Data Transfer Objects for the assessment application
// layer that cross package boundaries — e.g. cached by the Redis idempotency
// service, returned by IdempotencyService.Check/Save. Kept separate from
// package assessment itself so that infrastructure implementations of
// assessment's interfaces (and their generated test mocks) reference this
// leaf package instead of importing assessment back, which would otherwise
// force assessment's own internal test files into an import cycle the
// moment they need a mock of IdempotencyService.
package dto

// SubmitResponse is the result of a completed (or fallback_static) assessment submission.
type SubmitResponse struct {
	ResultID      string
	MBTIType      string
	GritScore     int
	AISummaryText string
	WellbeingFlag bool
	Status        string
}
