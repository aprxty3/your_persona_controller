package questiontranslation

import "context"

// QuestionTranslation holds the locale-specific text and options for a Question.
// This package provides a standalone repository interface for direct lookups,
// while the question package handles the combined FindAllWithTranslation query.
//
// Composite UNIQUE constraint: (question_id, locale) — see ERD.
// Locale fallback to "en" is handled at the query level (FR-I9).
type QuestionTranslation struct {
	ID           string
	QuestionID   string
	Locale       string  // e.g. "en", "id"
	QuestionText string
	Options      *string // JSON-encoded options for mc/likert; nil for essay_prompt
}

// Repository defines the contract for QuestionTranslation data persistence.
type Repository interface {
	// FindByQuestionAndLocale retrieves the translation for a specific question in a locale.
	// Returns nil, nil when no translation exists for that locale (caller should fallback to "en").
	FindByQuestionAndLocale(ctx context.Context, questionID, locale string) (*QuestionTranslation, error)

	// UpsertTranslation inserts or updates a translation record (for seeding/admin use).
	UpsertTranslation(ctx context.Context, translation *QuestionTranslation) error
}