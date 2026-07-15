package dashboard

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aprxty3/your_persona_controller.git/internal/domain/content"
	"github.com/aprxty3/your_persona_controller.git/internal/domain/testresult"
	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
)

func testLogger() logger.Logger { return logger.NewLogger("test") }

type mockTestResultRepo struct {
	usage      int64
	usageErr   error
	history    []testresult.TestResult
	historyErr error
}

func (m *mockTestResultRepo) CountMonthlyUsage(ctx context.Context, userID string) (int64, error) {
	return m.usage, m.usageErr
}

func (m *mockTestResultRepo) FindHistoryByUser(ctx context.Context, userID string, page, limit int) ([]testresult.TestResult, int64, error) {
	return m.history, int64(len(m.history)), m.historyErr
}

// mockInsightTemplateRepo is a hand-written fake satisfying InsightTemplateRepository.
type mockInsightTemplateRepo struct {
	templates []content.InsightTemplate
	err       error

	gotTrait  string
	gotLocale string
}

func (m *mockInsightTemplateRepo) FindMatchingTemplates(ctx context.Context, trait, locale string) ([]content.InsightTemplate, error) {
	m.gotTrait = trait
	m.gotLocale = locale
	return m.templates, m.err
}

func floatPtr(v float64) *float64 { return &v }

func newResult(id string, gritScore int, when time.Time) testresult.TestResult {
	return testresult.TestResult{ID: id, GritScore: gritScore, CreatedAt: when}
}

func TestGetDashboard_MicroInsight_IncreaseTriggers(t *testing.T) {
	now := time.Now()
	trRepo := &mockTestResultRepo{history: []testresult.TestResult{
		newResult("r2", 76, now),
		newResult("r1", 70, now.Add(-24*time.Hour)),
	}}
	itRepo := &mockInsightTemplateRepo{templates: []content.InsightTemplate{
		{InsightKey: "grit_increase", ConditionType: content.ConditionIncrease, MinDelta: floatPtr(5), TemplateText: "GRIT kamu naik {delta} poin"},
	}}
	uc := NewDashboardUseCase(trRepo, itRepo, testLogger())

	resp, err := uc.GetDashboard(context.Background(), "user-1", "id")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.MicroInsights) != 1 || resp.MicroInsights[0] != "GRIT kamu naik 6 poin" {
		t.Fatalf("expected 1 increase insight with delta substituted, got %v", resp.MicroInsights)
	}
	if itRepo.gotTrait != "grit" || itRepo.gotLocale != "id" {
		t.Fatalf("expected lookup for trait=grit locale=id, got trait=%s locale=%s", itRepo.gotTrait, itRepo.gotLocale)
	}
}

func TestGetDashboard_MicroInsight_IncreaseBelowMinDelta(t *testing.T) {
	now := time.Now()
	trRepo := &mockTestResultRepo{history: []testresult.TestResult{
		newResult("r2", 74, now),
		newResult("r1", 70, now.Add(-24*time.Hour)),
	}}
	itRepo := &mockInsightTemplateRepo{templates: []content.InsightTemplate{
		{InsightKey: "grit_increase", ConditionType: content.ConditionIncrease, MinDelta: floatPtr(5), TemplateText: "GRIT kamu naik {delta} poin"},
	}}
	uc := NewDashboardUseCase(trRepo, itRepo, testLogger())

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
	trRepo := &mockTestResultRepo{history: []testresult.TestResult{
		newResult("r2", 60, now),
		newResult("r1", 70, now.Add(-24*time.Hour)),
	}}
	itRepo := &mockInsightTemplateRepo{templates: []content.InsightTemplate{
		{InsightKey: "grit_decrease", ConditionType: content.ConditionDecrease, MinDelta: floatPtr(5), TemplateText: "GRIT kamu turun {delta} poin"},
	}}
	uc := NewDashboardUseCase(trRepo, itRepo, testLogger())

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
	trRepo := &mockTestResultRepo{history: []testresult.TestResult{
		newResult("r1", 85, time.Now()),
	}}
	itRepo := &mockInsightTemplateRepo{templates: []content.InsightTemplate{
		{InsightKey: "grit_high_threshold", ConditionType: content.ConditionThreshold, ThresholdValue: floatPtr(80), TemplateText: "GRIT kamu sudah {value}, luar biasa"},
	}}
	uc := NewDashboardUseCase(trRepo, itRepo, testLogger())

	resp, err := uc.GetDashboard(context.Background(), "user-1", "en")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.MicroInsights) != 1 || resp.MicroInsights[0] != "GRIT kamu sudah 85, luar biasa" {
		t.Fatalf("expected threshold insight to fire off a single result, got %v", resp.MicroInsights)
	}
}

func TestGetDashboard_MicroInsight_ThresholdBelowValueDoesNotTrigger(t *testing.T) {
	trRepo := &mockTestResultRepo{history: []testresult.TestResult{
		newResult("r1", 79, time.Now()),
	}}
	itRepo := &mockInsightTemplateRepo{templates: []content.InsightTemplate{
		{InsightKey: "grit_high_threshold", ConditionType: content.ConditionThreshold, ThresholdValue: floatPtr(80), TemplateText: "GRIT kamu sudah {value}"},
	}}
	uc := NewDashboardUseCase(trRepo, itRepo, testLogger())

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
	trRepo := &mockTestResultRepo{history: []testresult.TestResult{
		newResult("r1", 90, time.Now()),
	}}
	itRepo := &mockInsightTemplateRepo{templates: []content.InsightTemplate{
		{InsightKey: "grit_increase", ConditionType: content.ConditionIncrease, MinDelta: floatPtr(5), TemplateText: "naik {delta}"},
	}}
	uc := NewDashboardUseCase(trRepo, itRepo, testLogger())

	resp, err := uc.GetDashboard(context.Background(), "user-1", "en")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.MicroInsights) != 0 {
		t.Fatalf("expected no delta insight with only 1 result, got %v", resp.MicroInsights)
	}
}

func TestGetDashboard_MicroInsight_NoResults_EmptyArray(t *testing.T) {
	trRepo := &mockTestResultRepo{history: nil}
	itRepo := &mockInsightTemplateRepo{templates: []content.InsightTemplate{
		{InsightKey: "grit_high_threshold", ConditionType: content.ConditionThreshold, ThresholdValue: floatPtr(1), TemplateText: "x"},
	}}
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
	trRepo := &mockTestResultRepo{history: []testresult.TestResult{
		newResult("r2", 74, now),
		newResult("r1", 0, now.Add(-24*time.Hour)), // pre-scoring row
	}}
	itRepo := &mockInsightTemplateRepo{templates: []content.InsightTemplate{
		{InsightKey: "grit_increase", ConditionType: content.ConditionIncrease, MinDelta: floatPtr(5), TemplateText: "naik {delta}"},
		{InsightKey: "grit_decrease", ConditionType: content.ConditionDecrease, MinDelta: floatPtr(5), TemplateText: "turun {delta}"},
	}}
	uc := NewDashboardUseCase(trRepo, itRepo, testLogger())

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
	trRepo := &mockTestResultRepo{history: []testresult.TestResult{
		newResult("r1", 0, time.Now()),
	}}
	itRepo := &mockInsightTemplateRepo{templates: []content.InsightTemplate{
		{InsightKey: "grit_high_threshold", ConditionType: content.ConditionThreshold, ThresholdValue: floatPtr(0), TemplateText: "x"},
	}}
	uc := NewDashboardUseCase(trRepo, itRepo, testLogger())

	resp, err := uc.GetDashboard(context.Background(), "user-1", "en")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.MicroInsights) != 0 {
		t.Fatalf("expected zero-score guard to suppress threshold insight, got %v", resp.MicroInsights)
	}
}

func TestGetDashboard_MicroInsight_TemplateLookupError(t *testing.T) {
	trRepo := &mockTestResultRepo{history: []testresult.TestResult{newResult("r1", 90, time.Now())}}
	itRepo := &mockInsightTemplateRepo{err: errors.New("db down")}
	uc := NewDashboardUseCase(trRepo, itRepo, testLogger())

	if _, err := uc.GetDashboard(context.Background(), "user-1", "en"); err == nil {
		t.Fatal("expected error to propagate when template lookup fails")
	}
}
