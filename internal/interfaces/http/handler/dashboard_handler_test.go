package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	dashboard "github.com/aprxty3/your_persona_controller.git/internal/application/user_dashboard"
	dashboardmocks "github.com/aprxty3/your_persona_controller.git/internal/application/user_dashboard/mocks"
	"github.com/aprxty3/your_persona_controller.git/internal/domain/testresult"
	"github.com/aprxty3/your_persona_controller.git/internal/interfaces/http/middleware"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/mock"
)

func newDashboardCtx(query string) (echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/v1/user-dashboard?"+query, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set(middleware.ContextUserID, "user-1")
	c.Set(middleware.ContextLocale, "en")
	return c, rec
}

func TestGetDashboard_Success_200(t *testing.T) {
	trRepo := dashboardmocks.NewMockTestResultRepository(t)
	trRepo.EXPECT().CountMonthlyUsage(mock.Anything, "user-1").Return(int64(1), nil).Once()
	trRepo.EXPECT().FindHistoryByUser(mock.Anything, "user-1", 1, mock.Anything).Return(nil, int64(0), nil).Once()
	insightRepo := dashboardmocks.NewMockInsightTemplateRepository(t)
	insightRepo.EXPECT().FindMatchingTemplates(mock.Anything, "grit", "en").Return(nil, nil).Maybe()

	uc := dashboard.NewDashboardUseCase(trRepo, insightRepo, testLog())
	h := NewDashboardHandler(uc, testLog())
	c, rec := newDashboardCtx("")

	if err := h.GetDashboard(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestGetDashboard_RepoError_500(t *testing.T) {
	trRepo := dashboardmocks.NewMockTestResultRepository(t)
	trRepo.EXPECT().CountMonthlyUsage(mock.Anything, "user-1").Return(int64(0), assertErrHandler).Once()

	uc := dashboard.NewDashboardUseCase(trRepo, dashboardmocks.NewMockInsightTemplateRepository(t), testLog())
	h := NewDashboardHandler(uc, testLog())
	c, rec := newDashboardCtx("")

	if err := h.GetDashboard(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestGetHistory_Success_200WithPagination(t *testing.T) {
	trRepo := dashboardmocks.NewMockTestResultRepository(t)
	trRepo.EXPECT().FindHistoryByUser(mock.Anything, "user-1", 2, 5).
		Return([]testresult.TestResult{{ID: "r1"}}, int64(11), nil).Once()

	uc := dashboard.NewDashboardUseCase(trRepo, dashboardmocks.NewMockInsightTemplateRepository(t), testLog())
	h := NewDashboardHandler(uc, testLog())
	c, rec := newDashboardCtx("page=2&limit=5")

	if err := h.GetHistory(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	body := decodeResponse(t, rec)
	meta, ok := body.Meta.(map[string]interface{})
	if !ok || meta["total"] != float64(11) {
		t.Errorf("expected meta.total=11, got %+v", body.Meta)
	}
}

func TestGetHistory_RepoError_500(t *testing.T) {
	trRepo := dashboardmocks.NewMockTestResultRepository(t)
	trRepo.EXPECT().FindHistoryByUser(mock.Anything, "user-1", mock.Anything, mock.Anything).
		Return(nil, int64(0), assertErrHandler).Once()

	uc := dashboard.NewDashboardUseCase(trRepo, dashboardmocks.NewMockInsightTemplateRepository(t), testLog())
	h := NewDashboardHandler(uc, testLog())
	c, rec := newDashboardCtx("")

	if err := h.GetHistory(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
	}
}
