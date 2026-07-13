package assessment

import (
	"context"

	"github.com/aprxty3/your_persona_controller.git/internal/domain/content"
	"github.com/aprxty3/your_persona_controller.git/internal/infrastructure/persistence/postgres"
	"github.com/aprxty3/your_persona_controller.git/pkg/locale"
	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
	"gorm.io/gorm"
)

type InsightTemplateRepository struct {
	db  *gorm.DB
	log logger.Logger
}

func NewInsightTemplateRepository(db *gorm.DB, log logger.Logger) content.InsightTemplateRepository {
	return &InsightTemplateRepository{
		db:  db,
		log: log.With("repository", "insighttemplate"),
	}
}

func toInsightTemplateEntity(model *postgres.InsightTemplateModel) content.InsightTemplate {
	return content.InsightTemplate{
		ID:             model.ID,
		InsightKey:     model.InsightKey,
		Locale:         model.Locale,
		Trait:          model.Trait,
		ConditionType:  content.ConditionType(model.ConditionType),
		MinDelta:       model.MinDelta,
		ThresholdValue: model.ThresholdValue,
		TemplateText:   model.TemplateText,
		IsActive:       model.IsActive,
	}
}

func (r *InsightTemplateRepository) FindMatchingTemplates(ctx context.Context, trait, loc string) ([]content.InsightTemplate, error) {
	var models []postgres.InsightTemplateModel

	err := r.db.WithContext(ctx).
		Where("trait = ? AND is_active = true AND (locale = ? OR locale = 'en')", trait, loc).
		Find(&models).Error

	if err != nil {
		r.log.Error("query failed", "op", "FindMatchingTemplates", "error", err)
		return nil, err
	}

	picked := locale.PickWithFallback(models,
		func(m postgres.InsightTemplateModel) string { return m.InsightKey },
		func(m postgres.InsightTemplateModel) string { return m.Locale },
		loc)

	entities := make([]content.InsightTemplate, 0, len(picked))
	for _, m := range picked {
		entities = append(entities, toInsightTemplateEntity(&m))
	}
	return entities, nil
}
