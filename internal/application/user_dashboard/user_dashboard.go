package dashboard

import (
	"context"
	"fmt"
	"time"

	"github.com/aprxty3/your_persona_controller.git/internal/application"
	"github.com/aprxty3/your_persona_controller.git/internal/domain/testresult"
	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
)

// TestResultRepository is the narrow slice of TestResult persistence the
// dashboard needs — scoped smaller than the full domain testresult.TestResultRepository.
type TestResultRepository interface {
	CountMonthlyUsage(ctx context.Context, userID string) (int64, error)
	FindHistoryByUser(ctx context.Context, userID string, page, limit int) (results []testresult.TestResult, total int64, err error)
}

// GritTrendPoint is one data point in the Member's recent GRIT score history .
type GritTrendPoint struct {
	ResultID  string    `json:"result_id"`
	GritScore int       `json:"grit_score"`
	CreatedAt time.Time `json:"created_at"`
}

// DashboardResponse summarizes a Member's quota and recent trend for the dashboard landing view.
type DashboardResponse struct {
	QuotaLimit     int              `json:"quota_limit"`
	QuotaUsed      int              `json:"quota_used"`
	QuotaRemaining int              `json:"quota_remaining"`
	GritTrend      []GritTrendPoint `json:"grit_trend"`
	LatestMBTIType string           `json:"latest_mbti_type,omitempty"`
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

// DashboardUseCase serves the Member dashboard landing view and test-result history
type DashboardUseCase struct {
	testResultRepo TestResultRepository
	log            logger.Logger
}

// NewDashboardUseCase constructs a DashboardUseCase.
func NewDashboardUseCase(testResultRepo TestResultRepository, log logger.Logger) *DashboardUseCase {
	return &DashboardUseCase{testResultRepo: testResultRepo, log: log.With("usecase", "dashboard")}
}

// GetDashboard computes the Member's derived remaining quota (never a stored counter) and the recent GRIT trend.
func (uc *DashboardUseCase) GetDashboard(ctx context.Context, userID string) (*DashboardResponse, error) {
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

	return &DashboardResponse{
		QuotaLimit:     application.MemberMonthlyQuota,
		QuotaUsed:      int(used),
		QuotaRemaining: remaining,
		GritTrend:      trend,
		LatestMBTIType: latestMBTI,
	}, nil
}

// GetHistory returns a page of the Member's test results, newest-first (FR-F5).
func (uc *DashboardUseCase) GetHistory(ctx context.Context, userID string, page, limit int) ([]HistoryItem, PaginationMeta, error) {
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
