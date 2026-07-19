// Package dashboard implements the Member dashboard landing view and
// paginated test-result history use cases.
package dashboard

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/aprxty3/your_persona_controller.git/internal/application"
	"github.com/aprxty3/your_persona_controller.git/internal/domain/content"
	"github.com/aprxty3/your_persona_controller.git/internal/domain/testresult"
	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
)

// TestResultRepository is the narrow slice of TestResult persistence the
// dashboard needs — scoped smaller than the full domain testresult.Repository.
type TestResultRepository interface {
	CountMonthlyUsage(ctx context.Context, userID string) (int64, error)
	FindHistoryByUser(ctx context.Context, userID string, page, limit int) (results []testresult.TestResult, total int64, err error)
}

// InsightTemplateRepository is the narrow slice of insight-template persistence
// the dashboard needs — same repository the PDF worker already reuses
// (internal/application/pdf), scoped here to a single method.
type InsightTemplateRepository interface {
	FindMatchingTemplates(ctx context.Context, trait, locale string) ([]content.InsightTemplate, error)
}

// microInsightTrait is the only trait FR-F4 evaluates on the dashboard today
// (GRIT trend). Lowercase — the template registry stores trait keys lowercase
// (see internal/application/pdf/generate_pdf.go's collectInsights).
const microInsightTrait = "grit"

// GritTrendPoint is one data point in the Member's recent GRIT score history .
type GritTrendPoint struct {
	ResultID  string    `json:"result_id"`
	GritScore int       `json:"grit_score"`
	CreatedAt time.Time `json:"created_at"`
}

// Response summarizes a Member's quota and recent trend for the dashboard landing view.
type Response struct {
	QuotaLimit     int              `json:"quota_limit"`
	QuotaUsed      int              `json:"quota_used"`
	QuotaRemaining int              `json:"quota_remaining"`
	GritTrend      []GritTrendPoint `json:"grit_trend"`
	LatestMBTIType string           `json:"latest_mbti_type,omitempty"`
	MicroInsights  []string         `json:"micro_insights"`
}

// HistoryItem is one row in the Member's paginated test-result history.
type HistoryItem struct {
	ResultID  string    `json:"result_id"`
	MBTIType  string    `json:"mbti_type"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

// PaginationMeta is a reusable pagination envelope for any paginated list endpoint.
type PaginationMeta struct {
	Page       int   `json:"page"`
	Limit      int   `json:"limit"`
	Total      int64 `json:"total"`
	TotalPages int   `json:"total_pages"`
}

// UseCase serves the Member dashboard landing view and test-result history
type UseCase struct {
	testResultRepo      TestResultRepository
	insightTemplateRepo InsightTemplateRepository
	log                 logger.Logger
}

// NewDashboardUseCase constructs a UseCase.
func NewDashboardUseCase(testResultRepo TestResultRepository, insightTemplateRepo InsightTemplateRepository, log logger.Logger) *UseCase {
	return &UseCase{
		testResultRepo:      testResultRepo,
		insightTemplateRepo: insightTemplateRepo,
		log:                 log.With("usecase", "dashboard"),
	}
}

// GetDashboard computes the Member's derived remaining quota (never a stored counter), the
// recent GRIT trend, and rule-based micro-insights  — no Gemini call anywhere in this path.
func (uc *UseCase) GetDashboard(ctx context.Context, userID, locale string) (*Response, error) {
	used, err := uc.testResultRepo.CountMonthlyUsage(ctx, userID)
	if err != nil {
		uc.log.Error("get dashboard failed", "step", "count_usage", "user_id", userID, "error", err)
		return nil, fmt.Errorf("get_dashboard: count usage: %w", err)
	}

	recent, _, err := uc.testResultRepo.FindHistoryByUser(ctx, userID, 1, application.GritTrendPoints)
	if err != nil {
		uc.log.Error("get dashboard failed", "step", "find_recent", "user_id", userID, "error", err)
		return nil, fmt.Errorf("get_dashboard: find recent: %w", err)
	}

	remaining := application.MemberMonthlyQuota - int(used)
	if remaining < 0 {
		remaining = 0
	}

	trend := make([]GritTrendPoint, len(recent))
	for i := range recent {
		src := recent[len(recent)-1-i]
		trend[i] = GritTrendPoint{ResultID: src.ID, GritScore: src.GritScore, CreatedAt: src.CreatedAt}
	}

	latestMBTI := ""
	if len(recent) > 0 {
		latestMBTI = recent[0].MBTIType
	}

	insights, err := uc.computeMicroInsights(ctx, recent, locale)
	if err != nil {
		uc.log.Error("get dashboard failed", "step", "micro_insights", "user_id", userID, "error", err)
		return nil, fmt.Errorf("get_dashboard: micro insights: %w", err)
	}

	return &Response{
		QuotaLimit:     application.MemberMonthlyQuota,
		QuotaUsed:      int(used),
		QuotaRemaining: remaining,
		GritTrend:      trend,
		LatestMBTIType: latestMBTI,
		MicroInsights:  insights,
	}, nil
}

// computeMicroInsights evaluates rule-based insights against the GRIT
// dimension only. recent must be newest-first (FindHistoryByUser's native
// order — the same slice GetDashboard already fetched for the trend, so this
// adds no extra query). Guards against fabricating insights from incomplete
// or old data (GritScore == 0 is the zero-value for rows scored
// before scoring engine exist, not a real score).
func (uc *UseCase) computeMicroInsights(ctx context.Context, recent []testresult.TestResult, locale string) ([]string, error) {
	insights := []string{}
	if len(recent) == 0 {
		return insights, nil
	}

	templates, err := uc.insightTemplateRepo.FindMatchingTemplates(ctx, microInsightTrait, locale)
	if err != nil {
		return nil, fmt.Errorf("find matching templates: %w", err)
	}

	latest := recent[0]

	hasDelta := false
	delta := 0
	if len(recent) >= 2 {
		previous := recent[1]
		if latest.GritScore != 0 && previous.GritScore != 0 {
			delta = latest.GritScore - previous.GritScore
			hasDelta = true
		}
	}

	for _, t := range templates {
		switch t.ConditionType {
		case content.ConditionIncrease:
			if !hasDelta || t.MinDelta == nil || float64(delta) < *t.MinDelta {
				continue
			}
			insights = append(insights, strings.ReplaceAll(t.TemplateText, "{delta}", strconv.Itoa(delta)))
		case content.ConditionDecrease:
			if !hasDelta || t.MinDelta == nil || float64(-delta) < *t.MinDelta {
				continue
			}
			insights = append(insights, strings.ReplaceAll(t.TemplateText, "{delta}", strconv.Itoa(-delta)))
		case content.ConditionThreshold, content.ConditionThresholdBelow:
			if latest.GritScore == 0 || !t.MatchesScore(float64(latest.GritScore)) {
				continue
			}
			insights = append(insights, strings.ReplaceAll(t.TemplateText, "{value}", strconv.Itoa(latest.GritScore)))
		}
	}

	return insights, nil
}

// GetHistory returns a page of the Member's test results, newest-first.
func (uc *UseCase) GetHistory(ctx context.Context, userID string, page, limit int) ([]HistoryItem, PaginationMeta, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 50 {
		limit = 10
	}

	results, total, err := uc.testResultRepo.FindHistoryByUser(ctx, userID, page, limit)
	if err != nil {
		uc.log.Error("get history failed", "user_id", userID, "page", page, "limit", limit, "error", err)
		return nil, PaginationMeta{}, fmt.Errorf("get_history: %w", err)
	}

	items := make([]HistoryItem, len(results))
	for i, r := range results {
		items[i] = HistoryItem{ResultID: r.ID, MBTIType: r.MBTIType, Status: string(r.Status), CreatedAt: r.CreatedAt}
	}

	totalPages := int((total + int64(limit) - 1) / int64(limit))
	meta := PaginationMeta{Page: page, Limit: limit, Total: total, TotalPages: totalPages}

	return items, meta, nil
}
