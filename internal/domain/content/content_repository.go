// Package content is the domain package for the assessment question bank:
// questions, translations, and insight templates.
package content

import (
	"context"
)

// QuestionRepository defines the contract for Question persistence.
type QuestionRepository interface {
	FindAllWithTranslation(ctx context.Context, locale string) ([]Question, map[string]QuestionTranslation, error)
	FindByID(ctx context.Context, id string) (*Question, error)
	FindByIDs(ctx context.Context, ids []string) ([]Question, error)
}

// QuestionTranslationRepository defines the contract for QuestionTranslation persistence.
type QuestionTranslationRepository interface {
	FindByQuestionAndLocale(ctx context.Context, questionID, locale string) (*QuestionTranslation, error)
	UpsertTranslation(ctx context.Context, translation *QuestionTranslation) error
}

// InsightTemplateRepository defines the contract for InsightTemplate persistence.
type InsightTemplateRepository interface {
	FindMatchingTemplates(ctx context.Context, trait, locale string) ([]InsightTemplate, error)
}
