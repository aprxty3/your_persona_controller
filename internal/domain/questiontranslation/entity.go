package questiontranslation

// QuestionTranslation holds the locale-specific text and options for a Question.
// This package provides a standalone repository interface for direct lookups,
// while the question package handles the combined FindAllWithTranslation query.
//
// Composite UNIQUE constraint: (question_id, locale) — see ERD.
// Locale fallback to "en" is handled at the query level (FR-I9).
type QuestionTranslation struct {
	ID           string
	QuestionID   string
	Locale       string // e.g. "en", "id"
	QuestionText string
	Options      *string // JSON-encoded options for mc/likert; nil for essay_prompt
}
