package question

import (
	"context"
)

// QuestionSection represents the assessment section a question belongs to.
type QuestionSection string

const (
	SectionA QuestionSection = "A" // SJT — Situational Judgment Test (FR-B1)
	SectionB QuestionSection = "B" // Likert scale — personality/GRIT traits (FR-B2)
	SectionC QuestionSection = "C" // Essay mini-prompts — qualitative analysis (FR-B3)
)

// QuestionType represents the format of the question.
type QuestionType string

const (
	TypeMultipleChoice QuestionType = "mc"           // SJT: choices A–E
	TypeLikert         QuestionType = "likert"        // Scale 1–5
	TypeEssayPrompt    QuestionType = "essay_prompt"  // Free-text essay mini-prompt
)

// Question is the locale-agnostic definition of an assessment question.
// Question text and answer options live in QuestionTranslation (i18n — FR-I4).
// Randomisation of display order is driven by display_order field (FR-B8).
type Question struct {
	ID               string
	Section          QuestionSection
	Type             QuestionType
	IsReverseScored  bool // Likert only — reverse the scale before scoring (FR-B2)
	IsAttentionCheck bool // Likert only — catch random clickers (FR-B2)
	DisplayOrder     int
}

// QuestionTranslation holds the locale-specific text and options for a Question.
// Composite UNIQUE constraint: (question_id, locale) — see ERD.
// Locale fallback to "en" when the requested locale is incomplete (FR-I9).
type QuestionTranslation struct {
	ID           string
	QuestionID   string
	Locale       string  // e.g. "en", "id"
	QuestionText string
	Options      *string // JSON-encoded options for mc/likert; nil for essay_prompt
}

// Repository defines the contract for Question and QuestionTranslation persistence.
type Repository interface {
	// FindAllWithTranslation returns all questions with their translation for the
	// given locale, falling back to "en" for any question without a translation
	// in the requested locale (FR-I9).
	FindAllWithTranslation(ctx context.Context, locale string) ([]Question, map[string]QuestionTranslation, error)

	// FindByID returns a single question by ID (locale-agnostic metadata only).
	FindByID(ctx context.Context, id string) (*Question, error)
}