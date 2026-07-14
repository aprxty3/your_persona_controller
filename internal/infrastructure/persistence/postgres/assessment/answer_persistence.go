package assessment

import (
	"context"

	"github.com/aprxty3/your_persona_controller.git/internal/domain/testresult"
	"github.com/aprxty3/your_persona_controller.git/internal/infrastructure/persistence/postgres"
	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type AnswerRepository struct {
	db  *gorm.DB
	log logger.Logger
}

var _ testresult.AnswerRepository = (*AnswerRepository)(nil)

func NewAnswerRepository(db *gorm.DB, log logger.Logger) *AnswerRepository {
	return &AnswerRepository{
		db:  db,
		log: log.With("repository", "answer"),
	}
}

func toAnswerModel(entity *testresult.Answer) postgres.AnswerModel {
	return postgres.AnswerModel{
		ID:           entity.ID,
		TestResultID: entity.TestResultID,
		QuestionID:   entity.QuestionID,
		Value:        entity.Value,
		CreatedAt:    entity.CreatedAt,
		UpdatedAt:    entity.UpdatedAt,
	}
}

func toAnswerEntity(model *postgres.AnswerModel) testresult.Answer {
	return testresult.Answer{
		ID:           model.ID,
		TestResultID: model.TestResultID,
		QuestionID:   model.QuestionID,
		Value:        model.Value,
		CreatedAt:    model.CreatedAt,
		UpdatedAt:    model.UpdatedAt,
	}
}

func (r *AnswerRepository) UpsertAnswers(ctx context.Context, testResultID string, answers []testresult.Answer) error {
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

	return postgres.LogQueryError(r.log, "UpsertAnswers", err)
}

func (r *AnswerRepository) FindByTestResultID(ctx context.Context, testResultID string) ([]testresult.Answer, error) {
	var models []postgres.AnswerModel
	err := r.db.WithContext(ctx).
		Where("test_result_id = ?", testResultID).
		Order("created_at asc").
		Find(&models).Error

	if err := postgres.LogQueryError(r.log, "FindByTestResultID", err); err != nil {
		return nil, err
	}

	entities := make([]testresult.Answer, len(models))
	for i, m := range models {
		entities[i] = toAnswerEntity(&m)
	}
	return entities, nil
}
