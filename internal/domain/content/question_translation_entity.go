package content

// QuestionTranslation holds the locale-specific text and options for a Question.
type QuestionTranslation struct {
	ID           string
	QuestionID   string
	Locale       string // e.g. "en", "id"
	QuestionText string
	Options      *string // JSON-encoded options for mc/likert; nil for essay_prompt
}
