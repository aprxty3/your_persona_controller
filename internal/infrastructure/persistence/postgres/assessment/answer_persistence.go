package assessment

import (
	"context"
	"time"

	"github.com/aprxty3/your_persona_controller.git/internal/domain/answer"
	"github.com/aprxty3/your_persona_controller.git/internal/domain/insighttemplate"
	"github.com/aprxty3/your_persona_controller.git/internal/domain/promptauditlog"
	"github.com/aprxty3/your_persona_controller.git/internal/infrastructure/persistence/postgres"
	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type AnswerRepository struct {
	db  *gorm.DB
	log logger.Logger
}

func NewAnswerRepository(db *gorm.DB, log logger.Logger) *AnswerRepository {
	return &AnswerRepository{
		db:  db,
		log: log.With("repository", "answer"),
	}
}

func toAnswerModel(entity *answer.Answer) postgres.AnswerModel {
	return postgres.AnswerModel{
		ID:           entity.ID,
		TestResultID: entity.TestResultID,
		QuestionID:   entity.QuestionID,
		Value:        entity.Value,
		CreatedAt:    entity.CreatedAt,
		UpdatedAt:    entity.UpdatedAt,
	}
}

func toAnswerEntity(model *postgres.AnswerModel) answer.Answer {
	return answer.Answer{
		ID:           model.ID,
		TestResultID: model.TestResultID,
		QuestionID:   model.QuestionID,
		Value:        model.Value,
		CreatedAt:    model.CreatedAt,
		UpdatedAt:    model.UpdatedAt,
	}
}

func toInsightTemplateEntity(model *postgres.InsightTemplateModel) insighttemplate.InsightTemplate {
	return insighttemplate.InsightTemplate{
		ID:             model.ID,
		InsightKey:     model.InsightKey,
		Locale:         model.Locale,
		Trait:          model.Trait,
		ConditionType:  insighttemplate.ConditionType(model.ConditionType),
		MinDelta:       model.MinDelta,
		ThresholdValue: model.ThresholdValue,
		TemplateText:   model.TemplateText,
		IsActive:       model.IsActive,
	}
}

func toPromptAuditLogModel(entity *promptauditlog.PromptAuditLog) postgres.PromptAuditLogModel {
	return postgres.PromptAuditLogModel{
		ID:             entity.ID,
		TestResultID:   entity.TestResultID,
		RawPrompt:      entity.RawPrompt,
		RawResponse:    entity.RawResponse,
		FlaggedAnomaly: entity.FlaggedAnomaly,
		CreatedAt:      entity.CreatedAt,
		ExpiresAt:      entity.ExpiresAt,
	}
}

func (r *AnswerRepository) UpsertAnswers(ctx context.Context, testResultID string, answers []answer.Answer) error {
	if len(answers) == 0 {
		return nil
	}

	models := make([]postgres.AnswerModel, len(answers))
	for i, ans := range answers {
		ans.TestResultID = testResultID
		models[i] = toAnswerModel(&ans)
	}

	err := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "test_result_id"}, {Name: "question_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"value", "updated_at"}),
		}).
		Create(&models).Error

	if err != nil {
		r.log.Error("query failed", "op", "UpsertAnswers", "error", err)
		return err
	}
	return nil
}

func (r *AnswerRepository) FindByTestResultID(ctx context.Context, testResultID string) ([]answer.Answer, error) {
	var models []postgres.AnswerModel
	err := r.db.WithContext(ctx).
		Where("test_result_id = ?", testResultID).
		Order("created_at asc").
		Find(&models).Error

	if err != nil {
		r.log.Error("query failed", "op", "FindByTestResultID", "error", err)
		return nil, err
	}

	entities := make([]answer.Answer, len(models))
	for i, m := range models {
		entities[i] = toAnswerEntity(&m)
	}
	return entities, nil
}

func (r *AnswerRepository) FindMatchingTemplates(ctx context.Context, trait, locale string) ([]insighttemplate.InsightTemplate, error) {
	var models []postgres.InsightTemplateModel

	err := r.db.WithContext(ctx).
		Where("trait = ? AND is_active = true AND (locale = ? OR locale = 'en')", trait, locale).
		Find(&models).Error

	if err != nil {
		r.log.Error("query failed", "op", "FindMatchingTemplates", "error", err)
		return nil, err
	}

	var matched []postgres.InsightTemplateModel
	isKeyMapped := make(map[string]bool)

	for _, m := range models {
		if m.Locale == locale {
			matched = append(matched, m)
			isKeyMapped[m.InsightKey] = true
		}
	}

	for _, m := range models {
		if m.Locale == "en" && !isKeyMapped[m.InsightKey] {
			matched = append(matched, m)
		}
	}

	entities := make([]insighttemplate.InsightTemplate, len(matched))
	for i, m := range matched {
		entities[i] = toInsightTemplateEntity(&m)
	}

	return entities, nil
}

func (r *AnswerRepository) Create(ctx context.Context, log *promptauditlog.PromptAuditLog) error {
	m := toPromptAuditLogModel(log)
	if err := r.db.WithContext(ctx).Create(&m).Error; err != nil {
		r.log.Error("query failed", "op", "Create", "error", err)
		return err
	}
	return nil
}

func (r *AnswerRepository) DeleteByTestResultID(ctx context.Context, testResultID string) error {
	err := r.db.WithContext(ctx).
		Where("test_result_id = ?", testResultID).
		Delete(&postgres.PromptAuditLogModel{}).Error

	if err != nil {
		r.log.Error("query failed", "op", "DeleteByTestResultID", "error", err)
	}
	return err
}

func (r *AnswerRepository) DeleteExpired(ctx context.Context) error {
	err := r.db.WithContext(ctx).
		Where("expires_at < ?", time.Now()).
		Delete(&postgres.PromptAuditLogModel{}).Error

	if err != nil {
		r.log.Error("query failed", "op", "DeleteExpired", "error", err)
	}
	return err
}
