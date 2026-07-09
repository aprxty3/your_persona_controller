package question

import (
	"context"
)

// Repository defines the contract for Question and QuestionTranslation persistence.
type Repository interface {
	// FindAllWithTranslation returns all questions with their translation for the
	// given locale, falling back to "en" for any question without a translation
	// in the requested locale (FR-I9).
	FindAllWithTranslation(ctx context.Context, locale string) ([]Question, map[string]QuestionTranslation, error)

	// FindByID returns a single question by ID (locale-agnostic metadata only).
	FindByID(ctx context.Context, id string) (*Question, error)
}
