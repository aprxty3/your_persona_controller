package handler

import (
	"net/http"

	"github.com/aprxty3/your_persona_controller.git/pkg/httpresponse"
	"github.com/labstack/echo/v4"
	goredis "github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// HealthHandler checks the synchronous dependencies nearly every endpoint relies on.
type HealthHandler struct {
	db          *gorm.DB
	redisClient *goredis.Client
}

// NewHealthHandler is the constructor for Dependency Injection.
func NewHealthHandler(db *gorm.DB, redisClient *goredis.Client) *HealthHandler {
	return &HealthHandler{db: db, redisClient: redisClient}
}

// HealthCheck handles GET /healthz
// @Summary      Health check
// @Description  Checks connectivity to PostgreSQL and Redis, the two dependencies used synchronously
// @Description  by nearly every endpoint. Used by container orchestration / load balancers.
// @Tags         Operational
// @Produce      json
// @Success      200 {object} httpresponse.Response "All checked dependencies are healthy"
// @Failure      503 {object} httpresponse.Response "One or more dependencies are unreachable — see data.checks"
// @Router       /healthz [get]
func (h *HealthHandler) HealthCheck(c echo.Context) error {
	ctx := c.Request().Context()
	checks := map[string]string{}
	healthy := true

	if sqlDB, err := h.db.DB(); err != nil || sqlDB.PingContext(ctx) != nil {
		checks["database"] = "down"
		healthy = false
	} else {
		checks["database"] = "up"
	}

	if err := h.redisClient.Ping(ctx).Err(); err != nil {
		checks["redis"] = "down"
		healthy = false
	} else {
		checks["redis"] = "up"
	}

	if !healthy {
		return c.JSON(http.StatusServiceUnavailable, httpresponse.Response{
			Success: false,
			Data:    checks,
			Error:   &httpresponse.ErrorDetail{Code: "SERVICE_UNAVAILABLE", Message: "One or more dependencies are unreachable"},
		})
	}

	return httpresponse.Success(c, http.StatusOK, checks, nil)
}
