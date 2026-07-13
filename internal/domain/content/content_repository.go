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

type QuestionTranslationRepository interface {
	FindByQuestionAndLocale(ctx context.Context, questionID, locale string) (*QuestionTranslation, error)
	UpsertTranslation(ctx context.Context, translation *QuestionTranslation) error
}

type InsightTemplateRepository interface {
	FindMatchingTemplates(ctx context.Context, trait, locale string) ([]InsightTemplate, error)
}
