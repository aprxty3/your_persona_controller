package questiontranslation

import "context"

// Repository defines the contract for QuestionTranslation data persistence.
type Repository interface {
	// FindByQuestionAndLocale retrieves the translation for a specific question in a locale.
	// Returns nil, nil when no translation exists for that locale (caller should fallback to "en").
	FindByQuestionAndLocale(ctx context.Context, questionID, locale string) (*QuestionTranslation, error)

	// UpsertTranslation inserts or updates a translation record (for seeding/admin use).
	UpsertTranslation(ctx context.Context, translation *QuestionTranslation) error
}
