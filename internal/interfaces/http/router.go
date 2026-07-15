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
	resultHandler *handler.ResultHandler,
	dashboardHandler *handler.DashboardHandler,
	authHandler *handler.AuthHandler,
	accountHandler *handler.AccountHandler,
	healthHandler *handler.HealthHandler,
	authMiddleware *appmiddleware.AuthMiddleware,
	localeMiddleware *appmiddleware.LocaleMiddleware,
) *echo.Echo {
	e := echo.New()

	// ---------------------------------------------------------
	// GLOBAL MIDDLEWARE
	// ---------------------------------------------------------
	e.Use(middleware.Recover())
	e.Use(middleware.Logger())

	e.Use(middleware.BodyLimit("32K"))
	e.Use(localeMiddleware.Negotiate)

	// Operational
	e.GET("/healthz", healthHandler.HealthCheck)

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
	accountGroup.PATCH("/profile", accountHandler.UpdateProfile, authMiddleware.RequireAuth)
	accountGroup.GET("/referral-code", accountHandler.GetReferralCode, authMiddleware.RequireAuth)
	accountGroup.POST("/delete-request", accountHandler.RequestDeletion, authMiddleware.RequireAuth)
	accountGroup.POST("/delete-request/cancel", accountHandler.CancelDeletion, authMiddleware.RequireAuth)

	// Assessment Group
	assessmentGroup := v1.Group("/assessment")

	// POST /v1/assessment/submit
	// Auth is optional — OptionalAuth resolves Member identity if a Bearer
	// token is present, but never blocks a Guest (cookie-only) submission.
	assessmentGroup.POST("/submit", assessmentHandler.Submit, authMiddleware.OptionalAuth)

	// Question bank — locale-negotiated only, no identity concept, so no auth middleware needed.
	v1.GET("/questions", resultHandler.GetQuestions)

	// Result Group — every route here serves personal assessment data, so
	// NoIndex (X-Robots-Tag: noindex, nofollow, FR-D9) is mandatory group-wide.
	resultGroup := v1.Group("/results", appmiddleware.NoIndex, authMiddleware.OptionalAuth)
	resultGroup.GET("/:id", resultHandler.GetResult)
	resultGroup.PATCH("/:id/mascot-style", resultHandler.UpdateMascotStyle)
	resultGroup.GET("/:id/pdf-status", resultHandler.GetPDFStatus)
	resultGroup.GET("/:id/pdf", resultHandler.GetPDF)

	// User Dashboard Group (Member Only) — "user-dashboard" is deliberate:
	// distinguishes this from the separate admin dashboard app (V2, sibling
	// repo `dashboard/`), so the path can never be mistaken for admin routes.
	dashboardGroup := v1.Group("/user-dashboard", authMiddleware.RequireAuth)
	dashboardGroup.GET("", dashboardHandler.GetDashboard)
	dashboardGroup.GET("/history", dashboardHandler.GetHistory)

	// Swagger Endpoint
	e.GET("/swagger/*", echoSwagger.WrapHandler)

	return e
}
