package http

import (
	_ "github.com/aprxty3/your_persona_controller.git/docs"
	"github.com/aprxty3/your_persona_controller.git/internal/interfaces/http/handler"
	appmiddleware "github.com/aprxty3/your_persona_controller.git/internal/interfaces/http/middleware"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	echoSwagger "github.com/swaggo/echo-swagger"
)

// SetupRouter initializes the Echo instance, applies global middlewares,
func SetupRouter(
	assessmentHandler *handler.AssessmentHandler,
	authHandler *handler.AuthHandler,
	profileHandler *handler.ProfileHandler,
	authMiddleware *appmiddleware.AuthMiddleware,
) *echo.Echo {
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
	authGroup.POST("/refresh", authHandler.RefreshToken)
	authGroup.POST("/forgot-password", authHandler.ForgotPassword)
	authGroup.POST("/verify-reset-otp", authHandler.VerifyResetOTP)
	authGroup.POST("/reset-password", authHandler.ResetPassword)
	authGroup.POST("/logout", authHandler.Logout, authMiddleware.RequireAuth)
	authGroup.POST("/logout-all", authHandler.LogoutAll, authMiddleware.RequireAuth)
	authGroup.POST("/change-password", authHandler.ChangePassword, authMiddleware.RequireAuth)

	// Account Group (Member Only)
	accountGroup := v1.Group("/account")
	accountGroup.PATCH("/profile", profileHandler.UpdateProfile, authMiddleware.RequireAuth)
	accountGroup.GET("/referral-code", profileHandler.GetReferralCode, authMiddleware.RequireAuth)

	// Assessment Group
	assessmentGroup := v1.Group("/assessment")

	// POST /v1/assessment/submit
	// Auth is optional (handled inside the handler itself).
	assessmentGroup.POST("/submit", assessmentHandler.Submit)

	// Swagger Endpoint
	e.GET("/swagger/*", echoSwagger.WrapHandler)

	return e
}
