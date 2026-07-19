// Package testresult is the domain package for assessment test results, answers, and prompt audit logs.
package testresult

import (
	"time"
)

// Answer represents a single response to a question in an assessment.
type Answer struct {
	ID           string
	TestResultID string
	QuestionID   string
	Value        string // "A"–"E" for SJT, "1"–"5" for Likert, free text for essay
	CreatedAt    time.Time
	UpdatedAt    time.Time
}
