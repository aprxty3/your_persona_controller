package middleware

import (
	"net/http"
	"strings"

	"github.com/aprxty3/your_persona_controller.git/internal/domain/user"
	jwtservice "github.com/aprxty3/your_persona_controller.git/internal/infrastructure/jwt"
	"github.com/aprxty3/your_persona_controller.git/pkg/httpresponse"
	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
	"github.com/labstack/echo/v4"
)

const (
	ContextUserID = "auth_user_id"
)

func UserIDFromContext(c echo.Context) string {
	id, _ := c.Get(ContextUserID).(string)
	return id
}

// AuthMiddleware guards endpoints marked "Auth: Required" in the API spec.
type AuthMiddleware struct {
	jwtService *jwtservice.JWTService
	userRepo   user.Repository
	log        logger.Logger
}

// NewAuthMiddleware constructs the middleware with its dependencies.
func NewAuthMiddleware(jwtService *jwtservice.JWTService, userRepo user.Repository, log logger.Logger) *AuthMiddleware {
	return &AuthMiddleware{
		jwtService: jwtService,
		userRepo:   userRepo,
		log:        log.With("middleware", "auth"),
	}
}

// RequireAuth validates the Bearer access token.
func (m *AuthMiddleware) RequireAuth(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		const bearerPrefix = "Bearer "

		header := c.Request().Header.Get(echo.HeaderAuthorization)
		if !strings.HasPrefix(header, bearerPrefix) {
			return httpresponse.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "Missing or malformed Authorization header")
		}

		claims, err := m.jwtService.ParseAccessToken(strings.TrimPrefix(header, bearerPrefix))
		if err != nil {
			m.log.Warn("auth rejected", "reason", "invalid_access_token", "error", err)
			return httpresponse.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "Invalid or expired access token")
		}

		u, err := m.userRepo.FindByID(c.Request().Context(), claims.Subject)
		if err != nil {
			m.log.Error("auth lookup failed", "error", err)
			return httpresponse.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to authenticate request")
		}
		if u == nil {
			m.log.Warn("auth rejected", "reason", "user_not_found")
			return httpresponse.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "Invalid or expired access token")
		}

		if claims.TokenVersion != u.TokenVersion {
			m.log.Warn("auth rejected", "reason", "token_version_mismatch", "user_id", u.ID)
			return httpresponse.Error(c, http.StatusUnauthorized, "TOKEN_VERSION_MISMATCH", "This session has been revoked. Please log in again")
		}

		c.Set(ContextUserID, u.ID)
		return next(c)
	}
}
