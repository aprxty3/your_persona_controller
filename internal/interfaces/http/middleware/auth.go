package middleware

import (
	"net/http"
	"strings"

	"github.com/aprxty3/your_persona_controller.git/internal/domain/account"
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
	userRepo   account.UserRepository
	log        logger.Logger
}

// NewAuthMiddleware constructs the middleware with its dependencies.
func NewAuthMiddleware(jwtService *jwtservice.JWTService, userRepo account.UserRepository, log logger.Logger) *AuthMiddleware {
	return &AuthMiddleware{
		jwtService: jwtService,
		userRepo:   userRepo,
		log:        log.With("middleware", "auth"),
	}
}

// BearerToken extracts the raw token from a "Bearer <token>" Authorization
// header, or "" if the header is absent — shared by every middleware that
// needs to inspect the caller's identity (RequireAuth, OptionalAuth, LocaleMiddleware).
func BearerToken(c echo.Context) string {
	const bearerPrefix = "Bearer "

	header := c.Request().Header.Get(echo.HeaderAuthorization)
	if header == "" {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(header, bearerPrefix))
}

// RequireAuth validates the Bearer access token.
func (m *AuthMiddleware) RequireAuth(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		tokenStr := BearerToken(c)
		if tokenStr == "" {
			return httpresponse.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "Missing or malformed Authorization header")
		}

		u, err := m.authenticate(c, tokenStr)
		if err != nil {
			return httpresponse.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to authenticate request")
		}
		if u == nil {
			return httpresponse.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "Invalid or expired access token")
		}
		if u.tokenVersionMismatch {
			return httpresponse.Error(c, http.StatusUnauthorized, "TOKEN_VERSION_MISMATCH", "This session has been revoked. Please log in again")
		}

		c.Set(ContextUserID, u.ID)
		return next(c)
	}
}

// OptionalAuth resolves the caller's identity if a valid Bearer token is
// present, but never rejects the request — endpoints with "Auth: Optional"
// (Guest-or-Member) use this instead of RequireAuth. Any failure (missing
// header, expired/invalid token, revoked session, lookup error) is treated
// identically: the request proceeds unauthenticated, and handlers fall back
// to their Guest-session logic via middleware.UserIDFromContext returning "".
func (m *AuthMiddleware) OptionalAuth(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if tokenStr := BearerToken(c); tokenStr != "" {
			if u, err := m.authenticate(c, tokenStr); err != nil {
				m.log.Warn("optional auth lookup failed, proceeding as guest", "error", err)
			} else if u != nil && !u.tokenVersionMismatch {
				c.Set(ContextUserID, u.ID)
			}
		}
		return next(c)
	}
}

type authenticatedUser struct {
	ID                   string
	tokenVersionMismatch bool
}

// authenticate parses the token and loads the member it belongs to. It
// returns (nil, nil) for any credential problem (invalid token, user gone) —
// only infrastructure failures (DB lookup error) are surfaced as an error,
// so callers can tell "not authenticated" apart from "couldn't check".
func (m *AuthMiddleware) authenticate(c echo.Context, tokenStr string) (*authenticatedUser, error) {
	claims, err := m.jwtService.ParseAccessToken(tokenStr)
	if err != nil {
		return nil, nil
	}

	u, err := m.userRepo.FindByID(c.Request().Context(), claims.Subject)
	if err != nil {
		m.log.Error("auth lookup failed", "error", err)
		return nil, err
	}
	if u == nil {
		return nil, nil
	}

	return &authenticatedUser{ID: u.ID, tokenVersionMismatch: claims.TokenVersion != u.TokenVersion}, nil
}
