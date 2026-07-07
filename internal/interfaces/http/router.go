package http

import (
	"github.com/aprxty3/your_persona_controller.git/internal/interfaces/http/handler"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

// SetupRouter initializes the Echo instance, applies global middlewares,
// and registers all API routes.
func SetupRouter(assessmentHandler *handler.AssessmentHandler) *echo.Echo {
	e := echo.New()

	// ---------------------------------------------------------
	// GLOBAL MIDDLEWARE
	// ---------------------------------------------------------
	e.Use(middleware.Recover())
	e.Use(middleware.Logger())

	// Security: Prevent Denial of Wallet & OOM by limiting body size to 32KB.
	e.Use(middleware.BodyLimit("32K"))

	// ---------------------------------------------------------
	// ROUTE DEFINITIONS
	// ---------------------------------------------------------
	v1 := e.Group("/v1")

	// Assessment Group
	assessmentGroup := v1.Group("/assessment")

	// POST /v1/assessment/submit
	// Auth is optional (handled inside the handler itself).
	assessmentGroup.POST("/submit", assessmentHandler.Submit)

	return e
}
