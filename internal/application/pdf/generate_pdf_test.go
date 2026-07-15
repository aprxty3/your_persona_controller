package pdf

import (
	"context"
	"errors"
	"testing"

	"github.com/aprxty3/your_persona_controller.git/internal/domain/account"
	accountmocks "github.com/aprxty3/your_persona_controller.git/internal/domain/account/mocks"
	"github.com/aprxty3/your_persona_controller.git/internal/domain/content"
	contentmocks "github.com/aprxty3/your_persona_controller.git/internal/domain/content/mocks"
	"github.com/aprxty3/your_persona_controller.git/internal/domain/testresult"
	testresultmocks "github.com/aprxty3/your_persona_controller.git/internal/domain/testresult/mocks"
	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
	"github.com/stretchr/testify/mock"
)

func testLogger() logger.Logger { return logger.NewLogger("test") }

// --- Execute ---

func TestExecute_ResultNotFound_DroppedSilently(t *testing.T) {
	trRepo := testresultmocks.NewMockTestResultRepository(t)
	trRepo.EXPECT().FindByID(mock.Anything, "r1").Return(nil, nil).Once()
	uc := NewGeneratePDFUseCase(trRepo, nil, nil, nil, nil, nil, nil, nil, testLogger())

	if err := uc.Execute(context.Background(), "r1"); err != nil {
		t.Fatalf("expected a silent drop for a missing test result, got error: %v", err)
	}
}

func TestExecute_MarkProcessingFails_Propagates(t *testing.T) {
	trRepo := testresultmocks.NewMockTestResultRepository(t)
	trRepo.EXPECT().FindByID(mock.Anything, "r1").Return(&testresult.TestResult{ID: "r1"}, nil).Once()
	trRepo.EXPECT().UpdatePDFStatus(mock.Anything, "r1", (*string)(nil), testresult.PDFStatusProcessing).Return(errors.New("db down")).Once()
	uc := NewGeneratePDFUseCase(trRepo, nil, nil, nil, nil, nil, nil, nil, testLogger())

	if err := uc.Execute(context.Background(), "r1"); err == nil {
		t.Fatal("expected the mark-processing error to propagate")
	}
}

func TestExecute_Success_UploadsAndMarksCompleted(t *testing.T) {
	userID := "user-1"
	result := &testresult.TestResult{ID: "r1", UserID: &userID, MBTIType: "ENTJ", GritScore: 80}

	trRepo := testresultmocks.NewMockTestResultRepository(t)
	trRepo.EXPECT().FindByID(mock.Anything, "r1").Return(result, nil).Once()
	trRepo.EXPECT().UpdatePDFStatus(mock.Anything, "r1", (*string)(nil), testresult.PDFStatusProcessing).Return(nil).Once()
	trRepo.EXPECT().UpdatePDFStatus(mock.Anything, "r1", mock.AnythingOfType("*string"), testresult.PDFStatusCompleted).Return(nil).Once()

	answerRepo := testresultmocks.NewMockAnswerRepository(t)
	answerRepo.EXPECT().FindByTestResultID(mock.Anything, "r1").Return(nil, nil).Once()

	userRepo := accountmocks.NewMockUserRepository(t)
	userRepo.EXPECT().FindByID(mock.Anything, "user-1").Return(&account.User{ID: "user-1", DisplayName: "Budi"}, nil).Once()

	renderer := NewMockPDFRenderer(t)
	renderer.EXPECT().Render(mock.Anything, mock.MatchedBy(func(d PDFData) bool { return d.DisplayName == "Budi" && d.MBTIType == "ENTJ" })).Return([]byte("%PDF-1.4..."), nil).Once()

	uploader := NewMockPDFUploader(t)
	uploader.EXPECT().Upload(mock.Anything, "member/user-1/r1.pdf", mock.Anything, "application/pdf").Return("https://r2.example/member/user-1/r1.pdf", nil).Once()

	uc := NewGeneratePDFUseCase(trRepo, answerRepo, nil, nil, userRepo, nil, renderer, uploader, testLogger())

	if err := uc.Execute(context.Background(), "r1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExecute_RenderError_Propagates(t *testing.T) {
	result := &testresult.TestResult{ID: "r1", GuestSessionID: strPtr("guest-1")}
	trRepo := testresultmocks.NewMockTestResultRepository(t)
	trRepo.EXPECT().FindByID(mock.Anything, "r1").Return(result, nil).Once()
	trRepo.EXPECT().UpdatePDFStatus(mock.Anything, "r1", (*string)(nil), testresult.PDFStatusProcessing).Return(nil).Once()
	answerRepo := testresultmocks.NewMockAnswerRepository(t)
	answerRepo.EXPECT().FindByTestResultID(mock.Anything, "r1").Return(nil, nil).Once()
	guestRepo := accountmocks.NewMockGuestSessionRepository(t)
	guestRepo.EXPECT().FindBySessionID(mock.Anything, "guest-1").Return(&account.GuestSession{SessionID: "guest-1", DisplayName: "Tamu"}, nil).Once()
	renderer := NewMockPDFRenderer(t)
	renderer.EXPECT().Render(mock.Anything, mock.Anything).Return(nil, errors.New("render failed")).Once()

	uc := NewGeneratePDFUseCase(trRepo, answerRepo, nil, nil, nil, guestRepo, renderer, nil, testLogger())

	if err := uc.Execute(context.Background(), "r1"); err == nil {
		t.Fatal("expected the render error to propagate (left as pdf_status=processing for the worker's failure hook)")
	}
}

func strPtr(s string) *string { return &s }

// --- collectEssayQuotes ---

func TestCollectEssayQuotes_FiltersEssayOnly_Deduped(t *testing.T) {
	answerRepo := testresultmocks.NewMockAnswerRepository(t)
	answerRepo.EXPECT().FindByTestResultID(mock.Anything, "r1").Return([]testresult.Answer{
		{QuestionID: "q1", Value: "my essay text"},
		{QuestionID: "q2", Value: "4"},
		{QuestionID: "q1", Value: "duplicate question id"}, // same question, should be deduped for the FindByIDs call
	}, nil).Once()
	questionRepo := contentmocks.NewMockQuestionRepository(t)
	questionRepo.EXPECT().FindByIDs(mock.Anything, mock.MatchedBy(func(ids []string) bool { return len(ids) == 2 })).Return([]content.Question{
		{ID: "q1", Type: content.TypeEssayPrompt},
		{ID: "q2", Type: content.TypeLikert},
	}, nil).Once()

	uc := &GeneratePDFUseCase{answerRepo: answerRepo, questionRepo: questionRepo, log: testLogger()}

	quotes, err := uc.collectEssayQuotes(context.Background(), "r1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(quotes) != 2 || quotes[0] != "my essay text" || quotes[1] != "duplicate question id" {
		t.Fatalf("expected both essay answers for q1, likert q2 excluded, got %v", quotes)
	}
}

func TestCollectEssayQuotes_NoAnswers_ReturnsNil(t *testing.T) {
	answerRepo := testresultmocks.NewMockAnswerRepository(t)
	answerRepo.EXPECT().FindByTestResultID(mock.Anything, "r1").Return(nil, nil).Once()
	uc := &GeneratePDFUseCase{answerRepo: answerRepo, log: testLogger()}

	quotes, err := uc.collectEssayQuotes(context.Background(), "r1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if quotes != nil {
		t.Fatalf("expected nil quotes for no answers, got %v", quotes)
	}
}

// --- collectInsights ---

func TestCollectInsights_ThresholdMatch_Included(t *testing.T) {
	insightRepo := contentmocks.NewMockInsightTemplateRepository(t)
	insightRepo.EXPECT().FindMatchingTemplates(mock.Anything, "grit", "en").Return([]content.InsightTemplate{
		{ConditionType: content.ConditionThreshold, ThresholdValue: floatPtr(70), TemplateText: "Kekuatanmu: GRIT {value}"},
	}, nil).Once()
	uc := &GeneratePDFUseCase{insightRepo: insightRepo, log: testLogger()}

	texts, err := uc.collectInsights(context.Background(), &testresult.TestResult{Locale: "en", TraitScores: map[string]interface{}{"GRIT": 80}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(texts) != 1 || texts[0] != "Kekuatanmu: GRIT 80" {
		t.Fatalf("expected 1 threshold insight with {value} substituted, got %v", texts)
	}
}

func TestCollectInsights_BelowThreshold_Excluded(t *testing.T) {
	insightRepo := contentmocks.NewMockInsightTemplateRepository(t)
	insightRepo.EXPECT().FindMatchingTemplates(mock.Anything, "grit", "en").Return([]content.InsightTemplate{
		{ConditionType: content.ConditionThreshold, ThresholdValue: floatPtr(90), TemplateText: "x"},
	}, nil).Once()
	uc := &GeneratePDFUseCase{insightRepo: insightRepo, log: testLogger()}

	texts, err := uc.collectInsights(context.Background(), &testresult.TestResult{Locale: "en", TraitScores: map[string]interface{}{"GRIT": 80}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(texts) != 0 {
		t.Fatalf("expected no insight below threshold, got %v", texts)
	}
}

// increase/decrease need a previous result this single-result job doesn't
// have — must always be skipped here (that's the dashboard's job).
func TestCollectInsights_IncreaseDecreaseAlwaysSkipped(t *testing.T) {
	insightRepo := contentmocks.NewMockInsightTemplateRepository(t)
	insightRepo.EXPECT().FindMatchingTemplates(mock.Anything, "grit", "en").Return([]content.InsightTemplate{
		{ConditionType: content.ConditionIncrease, MinDelta: floatPtr(1), TemplateText: "naik"},
		{ConditionType: content.ConditionDecrease, MinDelta: floatPtr(1), TemplateText: "turun"},
	}, nil).Once()
	uc := &GeneratePDFUseCase{insightRepo: insightRepo, log: testLogger()}

	texts, err := uc.collectInsights(context.Background(), &testresult.TestResult{Locale: "en", TraitScores: map[string]interface{}{"GRIT": 80}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(texts) != 0 {
		t.Fatalf("expected increase/decrease templates to always be skipped in the PDF job, got %v", texts)
	}
}

func TestCollectInsights_NonNumericTraitScore_Skipped(t *testing.T) {
	uc := &GeneratePDFUseCase{log: testLogger()} // insightRepo nil: FindMatchingTemplates must never be called

	texts, err := uc.collectInsights(context.Background(), &testresult.TestResult{Locale: "en", TraitScores: map[string]interface{}{"GRIT": "not-a-number"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(texts) != 0 {
		t.Fatalf("expected non-numeric trait scores to be skipped without a lookup, got %v", texts)
	}
}

func TestCollectInsights_RepoError_Propagates(t *testing.T) {
	insightRepo := contentmocks.NewMockInsightTemplateRepository(t)
	insightRepo.EXPECT().FindMatchingTemplates(mock.Anything, "grit", "en").Return(nil, errors.New("db down")).Once()
	uc := &GeneratePDFUseCase{insightRepo: insightRepo, log: testLogger()}

	if _, err := uc.collectInsights(context.Background(), &testresult.TestResult{Locale: "en", TraitScores: map[string]interface{}{"GRIT": 80}}); err == nil {
		t.Fatal("expected the repository error to propagate")
	}
}

func floatPtr(v float64) *float64 { return &v }

// --- toNumeric ---

func TestToNumeric_Float64AndInt(t *testing.T) {
	if v, ok := toNumeric(float64(42)); !ok || v != 42 {
		t.Fatalf("expected float64(42) to convert cleanly, got %v ok=%v", v, ok)
	}
	if v, ok := toNumeric(42); !ok || v != 42 {
		t.Fatalf("expected int(42) to convert cleanly, got %v ok=%v", v, ok)
	}
	if _, ok := toNumeric("42"); ok {
		t.Fatal("expected a string value to fail conversion")
	}
}

// --- resolveDisplayName ---

func TestResolveDisplayName_Member(t *testing.T) {
	userRepo := accountmocks.NewMockUserRepository(t)
	userRepo.EXPECT().FindByID(mock.Anything, "user-1").Return(&account.User{ID: "user-1", DisplayName: "Budi"}, nil).Once()
	uc := &GeneratePDFUseCase{userRepo: userRepo, log: testLogger()}

	name, err := uc.resolveDisplayName(context.Background(), &testresult.TestResult{UserID: strPtr("user-1")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "Budi" {
		t.Fatalf("expected Budi, got %q", name)
	}
}

func TestResolveDisplayName_Guest(t *testing.T) {
	guestRepo := accountmocks.NewMockGuestSessionRepository(t)
	guestRepo.EXPECT().FindBySessionID(mock.Anything, "guest-1").Return(&account.GuestSession{SessionID: "guest-1", DisplayName: "Tamu"}, nil).Once()
	uc := &GeneratePDFUseCase{guestSessionRepo: guestRepo, log: testLogger()}

	name, err := uc.resolveDisplayName(context.Background(), &testresult.TestResult{GuestSessionID: strPtr("guest-1")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "Tamu" {
		t.Fatalf("expected Tamu, got %q", name)
	}
}

func TestResolveDisplayName_NeitherOwnerType_EmptyString(t *testing.T) {
	uc := &GeneratePDFUseCase{log: testLogger()}

	name, err := uc.resolveDisplayName(context.Background(), &testresult.TestResult{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "" {
		t.Fatalf("expected empty display name, got %q", name)
	}
}

// --- objectKey ---

func TestObjectKey_Member(t *testing.T) {
	key := objectKey(&testresult.TestResult{ID: "r1", UserID: strPtr("user-1")})
	if key != "member/user-1/r1.pdf" {
		t.Fatalf("expected member path, got %q", key)
	}
}

func TestObjectKey_Guest(t *testing.T) {
	key := objectKey(&testresult.TestResult{ID: "r1", GuestSessionID: strPtr("guest-1")})
	if key != "guest/guest-1/r1.pdf" {
		t.Fatalf("expected guest path, got %q", key)
	}
}
