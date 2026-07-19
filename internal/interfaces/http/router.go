// Package http wires the Echo router: middleware chain (CORS, CSRF,
// logging, rate limiting), route registration, and Swagger UI.
package http

import (
	"fmt"
	"net"
	"net/http"
	"strings"

	_ "github.com/aprxty3/your_persona_controller.git/docs" // registers the generated swagger.json/yaml with echo-swagger
	"github.com/aprxty3/your_persona_controller.git/internal/interfaces/http/handler"
	appmiddleware "github.com/aprxty3/your_persona_controller.git/internal/interfaces/http/middleware"
	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	echoSwagger "github.com/swaggo/echo-swagger"
)

// csrfProtectedPaths lists the exact routes Security Rules
// mandate CSRF protection for — state-changing endpoints reachable via the
// ambient `session_id` Guest cookie (submit) or that mutate account state
// (profile, deletion request start/cancel). Bearer-token-only endpoints
// (register/login/etc.) are deliberately NOT in this list: a page cannot
// forge an Authorization header the way it can silently ride an ambient
// cookie, so CSRF isn't the relevant defense there — Turnstile + per-IP
// rate limiting cover that surface instead. Add new routes here, do not
// duplicate the CSRF middleware registration per-group.
var csrfProtectedPaths = map[string]bool{
	"/v1/assessment/submit":             true,
	"/v1/account/profile":               true,
	"/v1/account/delete-request":        true,
	"/v1/account/delete-request/cancel": true,
}

// ParseAllowedOrigins splits a comma-separated ALLOWED_ORIGINS env value
// into a whitelist for CORS. It panics on a literal "*" entry — the ticket
// this implements DILARANG (forbids) a wildcard origin, and enforcing that
// at startup (fail fast) is more robust than a code-review-only convention.
func ParseAllowedOrigins(raw string) []string {
	var origins []string
	for _, o := range strings.Split(raw, ",") {
		o = strings.TrimSpace(o)
		if o == "" {
			continue
		}
		if o == "*" {
			panic("ALLOWED_ORIGINS must not contain a wildcard \"*\" — list explicit frontend origins")
		}
		origins = append(origins, o)
	}
	return origins
}

// ParseTrustedProxies turns a comma-separated TRUSTED_PROXIES env value
// (CIDRs or bare IPs — a bare IP is treated as a /32 or /128) into an
// echo.IPExtractor for c.RealIP().
func ParseTrustedProxies(raw string) (echo.IPExtractor, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return echo.ExtractIPDirect(), nil
	}

	var opts []echo.TrustOption
	for _, entry := range strings.Split(raw, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if !strings.Contains(entry, "/") {
			if strings.Contains(entry, ":") {
				entry += "/128"
			} else {
				entry += "/32"
			}
		}
		_, ipNet, err := net.ParseCIDR(entry)
		if err != nil {
			return nil, fmt.Errorf("invalid TRUSTED_PROXIES entry %q: %w", entry, err)
		}
		opts = append(opts, echo.TrustIPRange(ipNet))
	}
	return echo.ExtractIPFromXFFHeader(opts...), nil
}

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
	allowedOrigins []string,
	isProduction bool,
	ipExtractor echo.IPExtractor,
	log logger.Logger,
) *echo.Echo {
	e := echo.New()
	e.IPExtractor = ipExtractor
	accessLog := log.With("component", "http_access")

	// ---------------------------------------------------------
	// GLOBAL MIDDLEWARE
	// ---------------------------------------------------------
	e.Use(middleware.Recover())

	e.Use(middleware.RequestIDWithConfig(middleware.RequestIDConfig{
		RequestIDHandler: func(c echo.Context, id string) {
			req := c.Request()
			c.SetRequest(req.WithContext(logger.ContextWithRequestID(req.Context(), id)))
		},
	}))

	e.Use(middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		LogRequestID: true,
		LogMethod:    true,
		LogURIPath:   true,
		LogStatus:    true,
		LogLatency:   true,
		LogRemoteIP:  true,
		LogError:     true,
		LogValuesFunc: func(_ echo.Context, v middleware.RequestLoggerValues) error {
			fields := []interface{}{
				"request_id", v.RequestID,
				"method", v.Method,
				"path", v.URIPath,
				"status", v.Status,
				"latency_ms", v.Latency.Milliseconds(),
				"remote_ip", v.RemoteIP,
			}
			if v.Error != nil {
				fields = append(fields, "error", v.Error)
			}
			switch {
			case v.Status >= 500:
				accessLog.Error("request", fields...)
			case v.Status >= 400:
				accessLog.Warn("request", fields...)
			default:
				accessLog.Info("request", fields...)
			}
			return nil
		},
	}))

	// CORS — no wildcard (see ParseAllowedOrigins), credentials allowed so
	// the browser sends session_id/csrf_token cookies cross-origin to the
	// separate front-end.
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: allowedOrigins,
		AllowMethods: []string{http.MethodGet, http.MethodPost, http.MethodPatch, http.MethodDelete, http.MethodOptions},
		AllowHeaders: []string{
			echo.HeaderOrigin, echo.HeaderContentType, echo.HeaderAccept, echo.HeaderAuthorization,
			"X-CSRF-Token", "Idempotency-Key",
		},
		AllowCredentials: true,
	}))

	e.Use(middleware.BodyLimit("32K"))

	// CSRF — double-submit cookie pattern. Registered globally (not per-group)
	// so the cookie is primed on ANY request (including plain GETs like
	// /v1/questions) before the frontend ever needs to submit a protected
	// POST/PATCH; Skipper narrows actual *enforcement* to csrfProtectedPaths.
	// Safe methods (GET/HEAD/OPTIONS/TRACE) are exempted by Echo itself.
	e.Use(middleware.CSRFWithConfig(middleware.CSRFConfig{
		TokenLookup:    "header:X-CSRF-Token",
		CookieName:     "csrf_token",
		CookieHTTPOnly: false, // frontend must read it to echo back in the header
		CookieSameSite: http.SameSiteLaxMode,
		CookieSecure:   isProduction,
		Skipper: func(c echo.Context) bool {
			switch c.Request().Method {
			case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
				return false
			default:
				return !csrfProtectedPaths[c.Path()]
			}
		},
	}))

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
	accountGroup.GET("/referral-stats", accountHandler.GetReferralStats, authMiddleware.RequireAuth)
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
