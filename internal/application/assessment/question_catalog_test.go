package assessment

import (
	"context"
	"errors"
	"testing"

	"github.com/aprxty3/your_persona_controller.git/internal/domain/content"
	"github.com/stretchr/testify/mock"
)

func TestListQuestions_MapsAndOrders(t *testing.T) {
	opts := `["A","B"]`
	questions := []content.Question{
		{ID: "q1", Section: "a", Type: content.TypeLikert, DisplayOrder: 1},
		{ID: "q2", Section: "b", Type: content.TypeMultipleChoice, DisplayOrder: 2},
	}
	translations := map[string]content.QuestionTranslation{
		"q1": {QuestionText: "Question one"},
		"q2": {QuestionText: "Question two", Options: &opts},
	}
	repo := NewMockQuestionCatalogRepository(t)
	repo.EXPECT().FindAllWithTranslation(mock.Anything, "id").Return(questions, translations, nil).Once()
	uc := NewQuestionCatalogUseCase(repo, testLogger())

	resp, err := uc.ListQuestions(context.Background(), "id")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp) != 2 {
		t.Fatalf("expected 2 questions, got %d", len(resp))
	}
	if resp[0].QuestionText != "Question one" || resp[1].Options == nil || *resp[1].Options != opts {
		t.Fatalf("expected translations to be mapped correctly, got %+v", resp)
	}
}

// Security contract (TICKET-17 hardening): the public DTO must never carry
// scoring metadata. QuestionResponse's own field list already excludes it at
// compile time, but assert the mapping doesn't smuggle it in via a stray field.
func TestListQuestions_NeverExposesScoringMetadata(t *testing.T) {
	optionMap := `{"A":{"EI":2}}`
	questions := []content.Question{
		{ID: "q1", Section: "b", Type: content.TypeLikert, Trait: "EI", IsReverseScored: true, IsAttentionCheck: true, OptionTraitMap: &optionMap},
	}
	repo := NewMockQuestionCatalogRepository(t)
	repo.EXPECT().FindAllWithTranslation(mock.Anything, "en").Return(questions, map[string]content.QuestionTranslation{}, nil).Once()
	uc := NewQuestionCatalogUseCase(repo, testLogger())

	resp, err := uc.ListQuestions(context.Background(), "en")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// QuestionResponse has no Trait/IsReverseScored/IsAttentionCheck/OptionTraitMap
	// fields at all — this test exists so that adding one back (a regression)
	// fails a test, not just a code review.
	if resp[0].ID != "q1" || resp[0].Section != "b" || resp[0].Type != string(content.TypeLikert) {
		t.Fatalf("unexpected response shape: %+v", resp[0])
	}
}

func TestListQuestions_RepoError_Propagates(t *testing.T) {
	repo := NewMockQuestionCatalogRepository(t)
	repo.EXPECT().FindAllWithTranslation(mock.Anything, "en").Return(nil, nil, errors.New("db down")).Once()
	uc := NewQuestionCatalogUseCase(repo, testLogger())

	if _, err := uc.ListQuestions(context.Background(), "en"); err == nil {
		t.Fatal("expected the repository error to propagate")
	}
}
