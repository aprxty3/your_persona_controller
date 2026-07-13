package assessment

import (
	"context"
	"errors"

	"github.com/aprxty3/your_persona_controller.git/internal/domain/question"
	"github.com/aprxty3/your_persona_controller.git/internal/domain/questiontranslation"
	"github.com/aprxty3/your_persona_controller.git/internal/infrastructure/persistence/postgres"
	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type QuestionRepository struct {
	db  *gorm.DB
	log logger.Logger
}

func NewQuestionRepository(db *gorm.DB, log logger.Logger) *QuestionRepository {
	return &QuestionRepository{
		db:  db,
		log: log.With("repository", "question"),
	}
}

func toQuestionEntity(model *postgres.QuestionModel) question.Question {
	return question.Question{
		ID:               model.ID,
		Section:          question.QuestionSection(model.Section),
		Type:             question.QuestionType(model.Type),
		IsReverseScored:  model.IsReverseScored,
		IsAttentionCheck: model.IsAttentionCheck,
		DisplayOrder:     model.DisplayOrder,
	}
}

func toTranslationEntity(model *postgres.QuestionTranslationModel) question.QuestionTranslation {
	return question.QuestionTranslation{
		ID:           model.ID,
		QuestionID:   model.QuestionID,
		Locale:       model.Locale,
		QuestionText: model.QuestionText,
		Options:      model.Options,
	}
}

func toQuestionTranslationModel(entity *questiontranslation.QuestionTranslation) postgres.QuestionTranslationModel {
	return postgres.QuestionTranslationModel{
		ID:           entity.ID,
		QuestionID:   entity.QuestionID,
		Locale:       entity.Locale,
		QuestionText: entity.QuestionText,
		Options:      entity.Options,
	}
}

func toQuestionTranslationEntity(model *postgres.QuestionTranslationModel) questiontranslation.QuestionTranslation {
	return questiontranslation.QuestionTranslation{
		ID:           model.ID,
		QuestionID:   model.QuestionID,
		Locale:       model.Locale,
		QuestionText: model.QuestionText,
		Options:      model.Options,
	}
}

func (r *QuestionRepository) FindByID(ctx context.Context, id string) (*question.Question, error) {
	var m postgres.QuestionModel
	err := r.db.WithContext(ctx).First(&m, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		r.log.Error("query failed", "op", "FindByID", "error", err)
		return nil, err
	}
	q := toQuestionEntity(&m)
	return &q, nil
}

func (r *QuestionRepository) FindAllWithTranslation(ctx context.Context, locale string) ([]question.Question, map[string]question.QuestionTranslation, error) {
	var questionModels []postgres.QuestionModel
	err := r.db.WithContext(ctx).
		Order("section asc, display_order asc").
		Find(&questionModels).Error
	if err != nil {
		r.log.Error("query failed", "op", "FindAllWithTranslation.questions", "error", err)
		return nil, nil, err
	}

	questions := make([]question.Question, len(questionModels))
	questionIDs := make([]string, len(questionModels))
	for i, m := range questionModels {
		questions[i] = toQuestionEntity(&m)
		questionIDs[i] = m.ID
	}

	var translations []postgres.QuestionTranslationModel
	err = r.db.WithContext(ctx).
		Where("question_id IN ? AND (locale = ? OR locale = 'en')", questionIDs, locale).
		Find(&translations).Error
	if err != nil {
		r.log.Error("query failed", "op", "FindAllWithTranslation.translations", "error", err)
		return nil, nil, err
	}

	translationMap := make(map[string]question.QuestionTranslation)
	isLocaleMapped := make(map[string]bool)

	for _, tr := range translations {
		if tr.Locale == locale {
			translationMap[tr.QuestionID] = toTranslationEntity(&tr)
			isLocaleMapped[tr.QuestionID] = true
		}
	}

	for _, tr := range translations {
		if tr.Locale == "en" && !isLocaleMapped[tr.QuestionID] {
			translationMap[tr.QuestionID] = toTranslationEntity(&tr)
		}
	}

	return questions, translationMap, nil
}

func (r *QuestionRepository) FindByQuestionAndLocale(ctx context.Context, questionID, locale string) (*questiontranslation.QuestionTranslation, error) {
	var m postgres.QuestionTranslationModel
	err := r.db.WithContext(ctx).
		First(&m, "question_id = ? AND locale = ?", questionID, locale).
		Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		r.log.Error("query failed", "op", "FindByQuestionAndLocale", "error", err)
		return nil, err
	}

	entity := toQuestionTranslationEntity(&m)
	return &entity, nil
}

func (r *QuestionRepository) UpsertTranslation(ctx context.Context, tr *questiontranslation.QuestionTranslation) error {
	m := toQuestionTranslationModel(tr)
	err := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "question_id"}, {Name: "locale"}},
			DoUpdates: clause.AssignmentColumns([]string{"question_text", "options"}),
		}).
		Create(&m).Error

	if err != nil {
		r.log.Error("query failed", "op", "UpsertTranslation", "error", err)
	}
	return err
}
