package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aprxty3/your_persona_controller.git/internal/domain/account"
	"github.com/aprxty3/your_persona_controller.git/internal/domain/account/mocks"
	jwtservice "github.com/aprxty3/your_persona_controller.git/internal/infrastructure/jwt"
	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/mock"
)

func testLog() logger.Logger { return logger.NewLogger("test") }

func newEchoCtx(authHeader string) (echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if authHeader != "" {
		req.Header.Set(echo.HeaderAuthorization, authHeader)
	}
	rec := httptest.NewRecorder()
	return e.NewContext(req, rec), rec
}

func TestBearerToken_ExtractsFromHeader(t *testing.T) {
	c, _ := newEchoCtx("Bearer abc123")
	if got := BearerToken(c); got != "abc123" {
		t.Errorf("expected abc123, got %q", got)
	}
}

func TestBearerToken_MissingHeader_ReturnsEmpty(t *testing.T) {
	c, _ := newEchoCtx("")
	if got := BearerToken(c); got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestRequireAuth_MissingHeader_401(t *testing.T) {
	m := NewAuthMiddleware(jwtservice.NewJWTService("secret"), mocks.NewMockUserRepository(t), testLog())
	c, rec := newEchoCtx("")

	handler := m.RequireAuth(func(c echo.Context) error { return c.NoContent(http.StatusOK) })
	if err := handler(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestRequireAuth_InvalidToken_401(t *testing.T) {
	m := NewAuthMiddleware(jwtservice.NewJWTService("secret"), mocks.NewMockUserRepository(t), testLog())
	c, rec := newEchoCtx("Bearer not-a-valid-jwt")

	handler := m.RequireAuth(func(c echo.Context) error { return c.NoContent(http.StatusOK) })
	if err := handler(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestRequireAuth_ValidToken_UserGone_401(t *testing.T) {
	jwtSvc := jwtservice.NewJWTService("secret")
	tokenStr, err := jwtSvc.GenerateAccessToken("user-1", 0, time.Hour)
	if err != nil {
		t.Fatalf("unexpected error generating token: %v", err)
	}

	userRepo := mocks.NewMockUserRepository(t)
	userRepo.EXPECT().FindByID(mock.Anything, "user-1").Return(nil, nil).Once()

	m := NewAuthMiddleware(jwtSvc, userRepo, testLog())
	c, rec := newEchoCtx("Bearer " + tokenStr)

	handler := m.RequireAuth(func(c echo.Context) error { return c.NoContent(http.StatusOK) })
	if err := handler(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestRequireAuth_TokenVersionMismatch_401(t *testing.T) {
	jwtSvc := jwtservice.NewJWTService("secret")
	tokenStr, err := jwtSvc.GenerateAccessToken("user-1", 1, time.Hour)
	if err != nil {
		t.Fatalf("unexpected error generating token: %v", err)
	}

	userRepo := mocks.NewMockUserRepository(t)
	userRepo.EXPECT().FindByID(mock.Anything, "user-1").Return(&account.User{ID: "user-1", TokenVersion: 2}, nil).Once()

	m := NewAuthMiddleware(jwtSvc, userRepo, testLog())
	c, rec := newEchoCtx("Bearer " + tokenStr)

	handler := m.RequireAuth(func(c echo.Context) error { return c.NoContent(http.StatusOK) })
	if err := handler(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 on token_version mismatch, got %d", rec.Code)
	}
}

func TestRequireAuth_ValidToken_SetsUserIDAndCallsNext(t *testing.T) {
	jwtSvc := jwtservice.NewJWTService("secret")
	tokenStr, err := jwtSvc.GenerateAccessToken("user-1", 0, time.Hour)
	if err != nil {
		t.Fatalf("unexpected error generating token: %v", err)
	}

	userRepo := mocks.NewMockUserRepository(t)
	userRepo.EXPECT().FindByID(mock.Anything, "user-1").Return(&account.User{ID: "user-1", TokenVersion: 0}, nil).Once()

	m := NewAuthMiddleware(jwtSvc, userRepo, testLog())
	c, rec := newEchoCtx("Bearer " + tokenStr)

	var gotUserID string
	handler := m.RequireAuth(func(c echo.Context) error {
		gotUserID = UserIDFromContext(c)
		return c.NoContent(http.StatusOK)
	})
	if err := handler(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if gotUserID != "user-1" {
		t.Errorf("expected context user id user-1, got %q", gotUserID)
	}
}

// A DB lookup failure (infrastructure error, distinct from "not authenticated")
// must surface as 500, not 401.
func TestRequireAuth_RepoLookupError_500(t *testing.T) {
	jwtSvc := jwtservice.NewJWTService("secret")
	tokenStr, err := jwtSvc.GenerateAccessToken("user-1", 0, time.Hour)
	if err != nil {
		t.Fatalf("unexpected error generating token: %v", err)
	}

	userRepo := mocks.NewMockUserRepository(t)
	userRepo.EXPECT().FindByID(mock.Anything, "user-1").Return(nil, assertErr).Once()

	m := NewAuthMiddleware(jwtSvc, userRepo, testLog())
	c, rec := newEchoCtx("Bearer " + tokenStr)

	handler := m.RequireAuth(func(c echo.Context) error { return c.NoContent(http.StatusOK) })
	if err := handler(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 on repo lookup failure, got %d", rec.Code)
	}
}

var assertErr = &lookupError{}

type lookupError struct{}

func (e *lookupError) Error() string { return "db unavailable" }

// --- OptionalAuth ---

func TestOptionalAuth_NoHeader_ProceedsAsGuest(t *testing.T) {
	m := NewAuthMiddleware(jwtservice.NewJWTService("secret"), mocks.NewMockUserRepository(t), testLog())
	c, rec := newEchoCtx("")

	var called bool
	handler := m.OptionalAuth(func(c echo.Context) error {
		called = true
		if UserIDFromContext(c) != "" {
			t.Error("expected no user id set for a guest request")
		}
		return c.NoContent(http.StatusOK)
	})
	if err := handler(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called || rec.Code != http.StatusOK {
		t.Errorf("expected next() to be called with 200, got called=%v code=%d", called, rec.Code)
	}
}

func TestOptionalAuth_InvalidToken_ProceedsAsGuestWithoutError(t *testing.T) {
	m := NewAuthMiddleware(jwtservice.NewJWTService("secret"), mocks.NewMockUserRepository(t), testLog())
	c, rec := newEchoCtx("Bearer garbage-token")

	handler := m.OptionalAuth(func(c echo.Context) error {
		if UserIDFromContext(c) != "" {
			t.Error("expected no user id set for an invalid token")
		}
		return c.NoContent(http.StatusOK)
	})
	if err := handler(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 (never rejects), got %d", rec.Code)
	}
}

func TestOptionalAuth_ValidToken_SetsUserID(t *testing.T) {
	jwtSvc := jwtservice.NewJWTService("secret")
	tokenStr, err := jwtSvc.GenerateAccessToken("user-1", 0, time.Hour)
	if err != nil {
		t.Fatalf("unexpected error generating token: %v", err)
	}

	userRepo := mocks.NewMockUserRepository(t)
	userRepo.EXPECT().FindByID(mock.Anything, "user-1").Return(&account.User{ID: "user-1", TokenVersion: 0}, nil).Once()

	m := NewAuthMiddleware(jwtSvc, userRepo, testLog())
	c, rec := newEchoCtx("Bearer " + tokenStr)

	handler := m.OptionalAuth(func(c echo.Context) error {
		if UserIDFromContext(c) != "user-1" {
			t.Errorf("expected user id user-1, got %q", UserIDFromContext(c))
		}
		return c.NoContent(http.StatusOK)
	})
	if err := handler(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

// A repo lookup error under OptionalAuth must still proceed as guest (never
// reject) — this is the whole point of "optional".
func TestOptionalAuth_RepoLookupError_StillProceedsAsGuest(t *testing.T) {
	jwtSvc := jwtservice.NewJWTService("secret")
	tokenStr, err := jwtSvc.GenerateAccessToken("user-1", 0, time.Hour)
	if err != nil {
		t.Fatalf("unexpected error generating token: %v", err)
	}

	userRepo := mocks.NewMockUserRepository(t)
	userRepo.EXPECT().FindByID(mock.Anything, "user-1").Return(nil, assertErr).Once()

	m := NewAuthMiddleware(jwtSvc, userRepo, testLog())
	c, rec := newEchoCtx("Bearer " + tokenStr)

	handler := m.OptionalAuth(func(c echo.Context) error { return c.NoContent(http.StatusOK) })
	if err := handler(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 (proceeds as guest despite lookup error), got %d", rec.Code)
	}
}
