package dashboard

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aprxty3/your_persona_controller.git/internal/application/user_dashboard/mocks"
	"github.com/aprxty3/your_persona_controller.git/internal/domain/content"
	"github.com/aprxty3/your_persona_controller.git/internal/domain/testresult"
	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
	"github.com/stretchr/testify/mock"
)

func testLogger() logger.Logger { return logger.NewLogger("test") }

func floatPtr(v float64) *float64 { return &v }

func newResult(id string, gritScore int, when time.Time) testresult.TestResult {
	return testresult.TestResult{ID: id, GritScore: gritScore, CreatedAt: when}
}

// newTestDashboardUseCase stubs CountMonthlyUsage (irrelevant to micro-insight
// tests) and wires the given history/templates.
func newTestDashboardUseCase(t *testing.T, history []testresult.TestResult, templates []content.InsightTemplate, templatesErr error) (*UseCase, *mocks.MockInsightTemplateRepository) {
	t.Helper()
	trRepo := mocks.NewMockTestResultRepository(t)
	trRepo.EXPECT().CountMonthlyUsage(mock.Anything, mock.Anything).Return(int64(0), nil).Once()
	trRepo.EXPECT().FindHistoryByUser(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(history, int64(len(history)), nil).Once()

	itRepo := mocks.NewMockInsightTemplateRepository(t)
	itRepo.EXPECT().FindMatchingTemplates(mock.Anything, mock.Anything, mock.Anything).Return(templates, templatesErr).Once()

	return NewDashboardUseCase(trRepo, itRepo, testLogger()), itRepo
}

func TestGetDashboard_MicroInsight_IncreaseTriggers(t *testing.T) {
	now := time.Now()
	history := []testresult.TestResult{newResult("r2", 76, now), newResult("r1", 70, now.Add(-24*time.Hour))}
	templates := []content.InsightTemplate{
		{InsightKey: "grit_increase", ConditionType: content.ConditionIncrease, MinDelta: floatPtr(5), TemplateText: "GRIT kamu naik {delta} poin"},
	}
	uc, itRepo := newTestDashboardUseCase(t, history, templates, nil)

	resp, err := uc.GetDashboard(context.Background(), "user-1", "id")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.MicroInsights) != 1 || resp.MicroInsights[0] != "GRIT kamu naik 6 poin" {
		t.Fatalf("expected 1 increase insight with delta substituted, got %v", resp.MicroInsights)
	}
	itRepo.AssertCalled(t, "FindMatchingTemplates", mock.Anything, "grit", "id")
}

func TestGetDashboard_MicroInsight_IncreaseBelowMinDelta(t *testing.T) {
	now := time.Now()
	history := []testresult.TestResult{newResult("r2", 74, now), newResult("r1", 70, now.Add(-24*time.Hour))}
	templates := []content.InsightTemplate{
		{InsightKey: "grit_increase", ConditionType: content.ConditionIncrease, MinDelta: floatPtr(5), TemplateText: "GRIT kamu naik {delta} poin"},
	}
	uc, _ := newTestDashboardUseCase(t, history, templates, nil)

	resp, err := uc.GetDashboard(context.Background(), "user-1", "id")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.MicroInsights) != 0 {
		t.Fatalf("expected no insights below min_delta, got %v", resp.MicroInsights)
	}
}

func TestGetDashboard_MicroInsight_DecreaseTriggers(t *testing.T) {
	now := time.Now()
	history := []testresult.TestResult{newResult("r2", 60, now), newResult("r1", 70, now.Add(-24*time.Hour))}
	templates := []content.InsightTemplate{
		{InsightKey: "grit_decrease", ConditionType: content.ConditionDecrease, MinDelta: floatPtr(5), TemplateText: "GRIT kamu turun {delta} poin"},
	}
	uc, _ := newTestDashboardUseCase(t, history, templates, nil)

	resp, err := uc.GetDashboard(context.Background(), "user-1", "id")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.MicroInsights) != 1 || resp.MicroInsights[0] != "GRIT kamu turun 10 poin" {
		t.Fatalf("expected 1 decrease insight with delta substituted, got %v", resp.MicroInsights)
	}
}

// Threshold only needs the single latest result — must fire even with just 1 result total.
func TestGetDashboard_MicroInsight_ThresholdTriggersWithSingleResult(t *testing.T) {
	history := []testresult.TestResult{newResult("r1", 85, time.Now())}
	templates := []content.InsightTemplate{
		{InsightKey: "grit_high_threshold", ConditionType: content.ConditionThreshold, ThresholdValue: floatPtr(80), TemplateText: "GRIT kamu sudah {value}, luar biasa"},
	}
	uc, _ := newTestDashboardUseCase(t, history, templates, nil)

	resp, err := uc.GetDashboard(context.Background(), "user-1", "en")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.MicroInsights) != 1 || resp.MicroInsights[0] != "GRIT kamu sudah 85, luar biasa" {
		t.Fatalf("expected threshold insight to fire off a single result, got %v", resp.MicroInsights)
	}
}

func TestGetDashboard_MicroInsight_ThresholdBelowValueDoesNotTrigger(t *testing.T) {
	history := []testresult.TestResult{newResult("r1", 79, time.Now())}
	templates := []content.InsightTemplate{
		{InsightKey: "grit_high_threshold", ConditionType: content.ConditionThreshold, ThresholdValue: floatPtr(80), TemplateText: "GRIT kamu sudah {value}"},
	}
	uc, _ := newTestDashboardUseCase(t, history, templates, nil)

	resp, err := uc.GetDashboard(context.Background(), "user-1", "en")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.MicroInsights) != 0 {
		t.Fatalf("expected no insight below threshold, got %v", resp.MicroInsights)
	}
}

// Fewer than 2 results: delta-based conditions (increase/decrease) must never fire.
func TestGetDashboard_MicroInsight_FewerThanTwoResults_NoDeltaInsight(t *testing.T) {
	history := []testresult.TestResult{newResult("r1", 90, time.Now())}
	templates := []content.InsightTemplate{
		{InsightKey: "grit_increase", ConditionType: content.ConditionIncrease, MinDelta: floatPtr(5), TemplateText: "naik {delta}"},
	}
	uc, _ := newTestDashboardUseCase(t, history, templates, nil)

	resp, err := uc.GetDashboard(context.Background(), "user-1", "en")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.MicroInsights) != 0 {
		t.Fatalf("expected no delta insight with only 1 result, got %v", resp.MicroInsights)
	}
}

func TestGetDashboard_MicroInsight_NoResults_EmptyArray(t *testing.T) {
	trRepo := mocks.NewMockTestResultRepository(t)
	trRepo.EXPECT().CountMonthlyUsage(mock.Anything, mock.Anything).Return(int64(0), nil).Once()
	trRepo.EXPECT().FindHistoryByUser(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, int64(0), nil).Once()
	itRepo := mocks.NewMockInsightTemplateRepository(t) // no EXPECT(): with 0 results, template lookup is skipped entirely
	uc := NewDashboardUseCase(trRepo, itRepo, testLogger())

	resp, err := uc.GetDashboard(context.Background(), "user-1", "en")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.MicroInsights == nil {
		t.Fatalf("expected non-nil empty slice (JSON []), got nil")
	}
	if len(resp.MicroInsights) != 0 {
		t.Fatalf("expected empty insights, got %v", resp.MicroInsights)
	}
}

func TestGetDashboard_MicroInsight_ZeroScoreGuardSkipsDelta(t *testing.T) {
	now := time.Now()
	history := []testresult.TestResult{newResult("r2", 74, now), newResult("r1", 0, now.Add(-24*time.Hour))} // pre-scoring row
	templates := []content.InsightTemplate{
		{InsightKey: "grit_increase", ConditionType: content.ConditionIncrease, MinDelta: floatPtr(5), TemplateText: "naik {delta}"},
		{InsightKey: "grit_decrease", ConditionType: content.ConditionDecrease, MinDelta: floatPtr(5), TemplateText: "turun {delta}"},
	}
	uc, _ := newTestDashboardUseCase(t, history, templates, nil)

	resp, err := uc.GetDashboard(context.Background(), "user-1", "en")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.MicroInsights) != 0 {
		t.Fatalf("expected zero-score guard to suppress delta insights, got %v", resp.MicroInsights)
	}
}

// A latest GritScore of 0 must not spuriously satisfy a threshold check either.
func TestGetDashboard_MicroInsight_ZeroScoreGuardSkipsThreshold(t *testing.T) {
	history := []testresult.TestResult{newResult("r1", 0, time.Now())}
	templates := []content.InsightTemplate{
		{InsightKey: "grit_high_threshold", ConditionType: content.ConditionThreshold, ThresholdValue: floatPtr(0), TemplateText: "x"},
	}
	uc, _ := newTestDashboardUseCase(t, history, templates, nil)

	resp, err := uc.GetDashboard(context.Background(), "user-1", "en")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.MicroInsights) != 0 {
		t.Fatalf("expected zero-score guard to suppress threshold insight, got %v", resp.MicroInsights)
	}
}

func TestGetDashboard_MicroInsight_TemplateLookupError(t *testing.T) {
	history := []testresult.TestResult{newResult("r1", 90, time.Now())}
	uc, _ := newTestDashboardUseCase(t, history, nil, errors.New("db down"))

	if _, err := uc.GetDashboard(context.Background(), "user-1", "en"); err == nil {
		t.Fatal("expected error to propagate when template lookup fails")
	}
}

func TestGetDashboard_CountUsageError_Propagates(t *testing.T) {
	trRepo := mocks.NewMockTestResultRepository(t)
	trRepo.EXPECT().CountMonthlyUsage(mock.Anything, "user-1").Return(int64(0), errors.New("db down")).Once()
	itRepo := mocks.NewMockInsightTemplateRepository(t) // no EXPECT(): never reached
	uc := NewDashboardUseCase(trRepo, itRepo, testLogger())

	if _, err := uc.GetDashboard(context.Background(), "user-1", "en"); err == nil {
		t.Fatal("expected the count-usage error to propagate")
	}
}
