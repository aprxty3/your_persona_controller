package assessment

import (
	"context"
	"fmt"

	"github.com/aprxty3/your_persona_controller.git/internal/domain/content"
	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
)

// QuestionCatalogRepository is the narrow slice of Question persistence this
// usecase needs — scoped smaller than the full domain content.QuestionRepository.
type QuestionCatalogRepository interface {
	FindAllWithTranslation(ctx context.Context, locale string) ([]content.Question, map[string]content.QuestionTranslation, error)
}

// QuestionResponse is a single question bank entry translated to the
// negotiated locale. Deliberately excludes ALL scoring metadata
// (is_reverse_scored, is_attention_check, trait, option_trait_map): this
// endpoint is public and unauthenticated, and those fields are an answer key —
// exposing which item is the attention check lets a bot always pass it, and
// exposing scoring direction lets users game their MBTI/GRIT results. The FE
// renders every question identically and never needs them.
type QuestionResponse struct {
	ID           string  `json:"id"`
	Section      string  `json:"section"`
	Type         string  `json:"type"`
	DisplayOrder int     `json:"display_order"`
	QuestionText string  `json:"question_text"`
	Options      *string `json:"options,omitempty"`
}

// QuestionCatalogUseCase serves the locale-aware question bank.
type QuestionCatalogUseCase struct {
	repo QuestionCatalogRepository
	log  logger.Logger
}

// NewQuestionCatalogUseCase constructs a QuestionCatalogUseCase.
func NewQuestionCatalogUseCase(repo QuestionCatalogRepository, log logger.Logger) *QuestionCatalogUseCase {
	return &QuestionCatalogUseCase{repo: repo, log: log.With("usecase", "question_catalog")}
}

// ListQuestions returns every question, ordered by section/display_order, with
// translations resolved for locale (falling back to "en" per pkg/locale.PickWithFallback).
func (uc *QuestionCatalogUseCase) ListQuestions(ctx context.Context, locale string) ([]QuestionResponse, error) {
	questions, translations, err := uc.repo.FindAllWithTranslation(ctx, locale)
	if err != nil {
		uc.log.Error("list questions failed", "locale", locale, "error", err)
		return nil, fmt.Errorf("list_questions: %w", err)
	}

	resp := make([]QuestionResponse, 0, len(questions))
	for _, q := range questions {
		tr := translations[q.ID]
		resp = append(resp, QuestionResponse{
			ID:           q.ID,
			Section:      string(q.Section),
			Type:         string(q.Type),
			DisplayOrder: q.DisplayOrder,
			QuestionText: tr.QuestionText,
			Options:      tr.Options,
		})
	}
	return resp, nil
}
