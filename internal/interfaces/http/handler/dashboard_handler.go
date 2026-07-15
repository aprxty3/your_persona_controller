package handler

import (
	"net/http"
	"strconv"

	dashboard "github.com/aprxty3/your_persona_controller.git/internal/application/user_dashboard"
	"github.com/aprxty3/your_persona_controller.git/internal/interfaces/http/middleware"
	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
	"github.com/labstack/echo/v4"
)

// DashboardHandler handles HTTP requests for the Member dashboard.
type DashboardHandler struct {
	useCase *dashboard.DashboardUseCase
	log     logger.Logger
}

// NewDashboardHandler is the constructor for Dependency Injection.
func NewDashboardHandler(useCase *dashboard.DashboardUseCase, log logger.Logger) *DashboardHandler {
	return &DashboardHandler{useCase: useCase, log: log.With("handler", "dashboard")}
}

// GetDashboard handles GET /v1/user-dashboard
// @Summary      Get user dashboard summary
// @Description  Member's personal dashboard: remaining monthly quota (derived on-the-fly), recent GRIT trend,
// @Description  and rule-based micro_insights (no Gemini call — locale-aware INSIGHT_TEMPLATE lookups only).
// @Description  This is the USER (Member) dashboard — not an admin dashboard; this API has no admin concept.
// @Tags         User Dashboard
// @Produce      json
// @Security     BearerAuth
// @Success      200 {object} httpresponse.Response{data=dashboard.DashboardResponse} "Dashboard summary"
// @Failure      401 {object} httpresponse.Response "UNAUTHORIZED | TOKEN_VERSION_MISMATCH"
// @Failure      500 {object} httpresponse.Response "INTERNAL_ERROR — unexpected server error"
// @Router       /v1/user-dashboard [get]
func (h *DashboardHandler) GetDashboard(c echo.Context) error {
	resp, err := h.useCase.GetDashboard(c.Request().Context(), middleware.UserIDFromContext(c), middleware.LocaleFromContext(c))
	if err != nil {
		h.log.Error("get dashboard failed", "error", err)
		return httpcallError(c, err)
	}
	return httpcallSuccess(c, http.StatusOK, resp, nil)
}

// GetHistory handles GET /v1/user-dashboard/history
// @Summary      Get paginated test-result history
// @Description  Ordered newest-first.
// @Tags         User Dashboard
// @Produce      json
// @Security     BearerAuth
// @Param        page  query int false "Page number, 1-indexed (default 1)"
// @Param        limit query int false "Results per page, max 50 (default 10)"
// @Success      200 {object} httpresponse.Response{data=[]dashboard.HistoryItem,meta=dashboard.PaginationMeta} "History page"
// @Failure      401 {object} httpresponse.Response "UNAUTHORIZED | TOKEN_VERSION_MISMATCH"
// @Failure      500 {object} httpresponse.Response "INTERNAL_ERROR — unexpected server error"
// @Router       /v1/user-dashboard/history [get]
func (h *DashboardHandler) GetHistory(c echo.Context) error {
	page, _ := strconv.Atoi(c.QueryParam("page"))
	limit, _ := strconv.Atoi(c.QueryParam("limit"))

	items, meta, err := h.useCase.GetHistory(c.Request().Context(), middleware.UserIDFromContext(c), page, limit)
	if err != nil {
		h.log.Error("get history failed", "error", err)
		return httpcallError(c, err)
	}
	return httpcallSuccess(c, http.StatusOK, items, meta)
}
