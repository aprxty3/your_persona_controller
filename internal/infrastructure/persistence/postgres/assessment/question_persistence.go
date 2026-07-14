package assessment

import (
	"context"

	"github.com/aprxty3/your_persona_controller.git/internal/domain/content"
	"github.com/aprxty3/your_persona_controller.git/internal/infrastructure/persistence/postgres"
	"github.com/aprxty3/your_persona_controller.git/pkg/locale"
	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type QuestionRepository struct {
	db  *gorm.DB
	log logger.Logger
}

var (
	_ content.QuestionRepository            = (*QuestionRepository)(nil)
	_ content.QuestionTranslationRepository = (*QuestionRepository)(nil)
)

func NewQuestionRepository(db *gorm.DB, log logger.Logger) *QuestionRepository {
	return &QuestionRepository{
		db:  db,
		log: log.With("repository", "question"),
	}
}

func toQuestionEntity(model *postgres.QuestionModel) content.Question {
	return content.Question{
		ID:               model.ID,
		Section:          content.QuestionSection(model.Section),
		Type:             content.QuestionType(model.Type),
		IsReverseScored:  model.IsReverseScored,
		IsAttentionCheck: model.IsAttentionCheck,
		DisplayOrder:     model.DisplayOrder,
	}
}

func toQuestionTranslationEntity(model *postgres.QuestionTranslationModel) content.QuestionTranslation {
	return content.QuestionTranslation{
		ID:           model.ID,
		QuestionID:   model.QuestionID,
		Locale:       model.Locale,
		QuestionText: model.QuestionText,
		Options:      model.Options,
	}
}

func toQuestionTranslationModel(entity *content.QuestionTranslation) postgres.QuestionTranslationModel {
	return postgres.QuestionTranslationModel{
		ID:           entity.ID,
		QuestionID:   entity.QuestionID,
		Locale:       entity.Locale,
		QuestionText: entity.QuestionText,
		Options:      entity.Options,
	}
}

func (r *QuestionRepository) FindByID(ctx context.Context, id string) (*content.Question, error) {
	var m postgres.QuestionModel
	err := r.db.WithContext(ctx).First(&m, "id = ?", id).Error
	if postgres.IsNotFound(err) {
		return nil, nil
	}
	if err := postgres.LogQueryError(r.log, "FindByID", err); err != nil {
		return nil, err
	}
	q := toQuestionEntity(&m)
	return &q, nil
}

// FindByIDs returns questions matching any of ids in a single query.
func (r *QuestionRepository) FindByIDs(ctx context.Context, ids []string) ([]content.Question, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	var models []postgres.QuestionModel
	err := r.db.WithContext(ctx).Where("id IN ?", ids).Find(&models).Error
	if err := postgres.LogQueryError(r.log, "FindByIDs", err); err != nil {
		return nil, err
	}

	questions := make([]content.Question, len(models))
	for i, m := range models {
		questions[i] = toQuestionEntity(&m)
	}
	return questions, nil
}

func (r *QuestionRepository) FindAllWithTranslation(ctx context.Context, loc string) ([]content.Question, map[string]content.QuestionTranslation, error) {
	var questionModels []postgres.QuestionModel
	err := r.db.WithContext(ctx).
		Order("section asc, display_order asc").
		Find(&questionModels).Error
	if err := postgres.LogQueryError(r.log, "FindAllWithTranslation.questions", err); err != nil {
		return nil, nil, err
	}

	questions := make([]content.Question, len(questionModels))
	questionIDs := make([]string, len(questionModels))
	for i, m := range questionModels {
		questions[i] = toQuestionEntity(&m)
		questionIDs[i] = m.ID
	}

	var translations []postgres.QuestionTranslationModel
	err = r.db.WithContext(ctx).
		Where("question_id IN ? AND (locale = ? OR locale = 'en')", questionIDs, loc).
		Find(&translations).Error
	if err := postgres.LogQueryError(r.log, "FindAllWithTranslation.translations", err); err != nil {
		return nil, nil, err
	}

	picked := locale.PickWithFallback(translations,
		func(m postgres.QuestionTranslationModel) string { return m.QuestionID },
		func(m postgres.QuestionTranslationModel) string { return m.Locale },
		loc)

	translationMap := make(map[string]content.QuestionTranslation, len(picked))
	for questionID, tr := range picked {
		translationMap[questionID] = toQuestionTranslationEntity(&tr)
	}

	return questions, translationMap, nil
}

func (r *QuestionRepository) FindByQuestionAndLocale(ctx context.Context, questionID, locale string) (*content.QuestionTranslation, error) {
	var m postgres.QuestionTranslationModel
	err := r.db.WithContext(ctx).
		First(&m, "question_id = ? AND locale = ?", questionID, locale).
		Error

	if postgres.IsNotFound(err) {
		return nil, nil
	}
	if err := postgres.LogQueryError(r.log, "FindByQuestionAndLocale", err); err != nil {
		return nil, err
	}

	entity := toQuestionTranslationEntity(&m)
	return &entity, nil
}

func (r *QuestionRepository) UpsertTranslation(ctx context.Context, tr *content.QuestionTranslation) error {
	m := toQuestionTranslationModel(tr)
	err := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "question_id"}, {Name: "locale"}},
			DoUpdates: clause.AssignmentColumns([]string{"question_text", "options"}),
		}).
		Create(&m).Error

	return postgres.LogQueryError(r.log, "UpsertTranslation", err)
}
