package http

import (
	"github.com/aprxty3/your_persona_controller.git/internal/interfaces/http/handler"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	echoSwagger "github.com/swaggo/echo-swagger"
)

// SetupRouter initializes the Echo instance, applies global middlewares,
// and registers all API routes.
func SetupRouter(assessmentHandler *handler.AssessmentHandler, authHandler *handler.AuthHandler) *echo.Echo {
	e := echo.New()

	// ---------------------------------------------------------
	// GLOBAL MIDDLEWARE
	// ---------------------------------------------------------
	e.Use(middleware.Recover())
	e.Use(middleware.Logger())

	e.Use(middleware.BodyLimit("32K"))

	v1 := e.Group("/v1")

	// Guest Session (Public onboarding)
	v1.POST("/guest-session", authHandler.CreateGuestSession)

	// Auth Group
	authGroup := v1.Group("/auth")
	authGroup.POST("/register", authHandler.Register)
	authGroup.POST("/verify-email-otp", authHandler.VerifyEmailOTP)
	authGroup.POST("/resend-email-otp", authHandler.ResendEmailOTP)
	authGroup.POST("/login", authHandler.Login)

	// Assessment Group
	assessmentGroup := v1.Group("/assessment")

	// POST /v1/assessment/submit
	// Auth is optional (handled inside the handler itself).
	assessmentGroup.POST("/submit", assessmentHandler.Submit)

	// Swagger Endpoint
	e.GET("/swagger/*", echoSwagger.WrapHandler)

	return e
}
