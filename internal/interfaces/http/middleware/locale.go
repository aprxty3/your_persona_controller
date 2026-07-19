package middleware

import (
	"github.com/aprxty3/your_persona_controller.git/internal/domain/account"
	jwtservice "github.com/aprxty3/your_persona_controller.git/internal/infrastructure/jwt"
	"github.com/aprxty3/your_persona_controller.git/pkg/locale"
	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
	"github.com/labstack/echo/v4"
)

// ContextLocale is the echo.Context key the locale middleware stores the
// negotiated request locale under.
const ContextLocale = "locale"

// LocaleFromContext reads the negotiated locale set by LocaleMiddleware.
func LocaleFromContext(c echo.Context) string {
	loc, _ := c.Get(ContextLocale).(string)
	if !locale.IsSupported(loc) {
		return locale.EN
	}
	return loc
}

// LocaleMiddleware negotiates the active request locale so handlers and use cases don't each repeat the same detection logic.
type LocaleMiddleware struct {
	jwtService *jwtservice.JWTService
	userRepo   account.UserRepository
	log        logger.Logger
}

// NewLocaleMiddleware constructs the middleware with its dependencies.
func NewLocaleMiddleware(jwtService *jwtservice.JWTService, userRepo account.UserRepository, log logger.Logger) *LocaleMiddleware {
	return &LocaleMiddleware{
		jwtService: jwtService,
		userRepo:   userRepo,
		log:        log.With("middleware", "locale"),
	}
}

// Negotiate resolves the request locale and stores it in context.
func (m *LocaleMiddleware) Negotiate(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		c.Set(ContextLocale, m.resolve(c))
		return next(c)
	}
}

// resolve applies the negotiation order: explicit query param,
// then authenticated member preference, then Accept-Language, then guest cookie, then the default (en).
func (m *LocaleMiddleware) resolve(c echo.Context) string {
	if q := c.QueryParam("locale"); locale.IsSupported(q) {
		return q
	}

	if pref, ok := m.memberPreference(c); ok {
		return pref
	}

	if lang := locale.ParseAcceptLanguage(c.Request().Header.Get("Accept-Language")); lang != "" {
		return lang
	}

	if cookie, err := c.Cookie("locale"); err == nil && cookie != nil && locale.IsSupported(cookie.Value) {
		return cookie.Value
	}

	return locale.EN
}

// memberPreference best-effort parses a Bearer token (if present) and looks up the member's stored preference.
func (m *LocaleMiddleware) memberPreference(c echo.Context) (string, bool) {
	tokenStr := BearerToken(c)
	if tokenStr == "" {
		return "", false
	}

	claims, err := m.jwtService.ParseAccessToken(tokenStr)
	if err != nil {
		return "", false
	}

	u, err := m.userRepo.FindByID(c.Request().Context(), claims.Subject)
	if err != nil {
		m.log.Warn("member locale lookup failed", "error", err)
		return "", false
	}
	if u == nil || !locale.IsSupported(u.PreferredLocale) {
		return "", false
	}
	return u.PreferredLocale, true
}
