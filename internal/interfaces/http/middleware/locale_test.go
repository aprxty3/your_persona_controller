package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aprxty3/your_persona_controller.git/internal/domain/account"
	"github.com/aprxty3/your_persona_controller.git/internal/domain/account/mocks"
	jwtservice "github.com/aprxty3/your_persona_controller.git/internal/infrastructure/jwt"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/mock"
)

func TestLocaleFromContext_Unset_DefaultsToEN(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	c := e.NewContext(req, httptest.NewRecorder())

	if got := LocaleFromContext(c); got != "en" {
		t.Errorf("expected en, got %q", got)
	}
}

func TestLocaleFromContext_UnsupportedValue_DefaultsToEN(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	c := e.NewContext(req, httptest.NewRecorder())
	c.Set(ContextLocale, "fr")

	if got := LocaleFromContext(c); got != "en" {
		t.Errorf("expected fallback to en, got %q", got)
	}
}

func newLocaleReq(t *testing.T, queryLocale, acceptLanguage, authHeader, cookieLocale string) echo.Context {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if queryLocale != "" {
		q := req.URL.Query()
		q.Set("locale", queryLocale)
		req.URL.RawQuery = q.Encode()
	}
	if acceptLanguage != "" {
		req.Header.Set("Accept-Language", acceptLanguage)
	}
	if authHeader != "" {
		req.Header.Set(echo.HeaderAuthorization, authHeader)
	}
	if cookieLocale != "" {
		req.AddCookie(&http.Cookie{Name: "locale", Value: cookieLocale})
	}
	e := echo.New()
	return e.NewContext(req, httptest.NewRecorder())
}

func TestNegotiate_ExplicitQueryParam_TakesHighestPriority(t *testing.T) {
	m := NewLocaleMiddleware(jwtservice.NewJWTService("secret"), mocks.NewMockUserRepository(t), testLog())
	c := newLocaleReq(t, "id", "en-US", "", "en")

	if err := m.Negotiate(func(c echo.Context) error { return nil })(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := LocaleFromContext(c); got != "id" {
		t.Errorf("expected query param id to win, got %q", got)
	}
}

func TestNegotiate_UnsupportedQueryParam_FallsThroughToAcceptLanguage(t *testing.T) {
	m := NewLocaleMiddleware(jwtservice.NewJWTService("secret"), mocks.NewMockUserRepository(t), testLog())
	c := newLocaleReq(t, "fr", "id-ID", "", "")

	if err := m.Negotiate(func(c echo.Context) error { return nil })(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := LocaleFromContext(c); got != "id" {
		t.Errorf("expected fallthrough to Accept-Language id, got %q", got)
	}
}

func TestNegotiate_MemberPreference_BeatsAcceptLanguageAndCookie(t *testing.T) {
	jwtSvc := jwtservice.NewJWTService("secret")
	tokenStr, err := jwtSvc.GenerateAccessToken("user-1", 0, time.Hour)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	userRepo := mocks.NewMockUserRepository(t)
	userRepo.EXPECT().FindByID(mock.Anything, "user-1").Return(&account.User{ID: "user-1", PreferredLocale: "id"}, nil).Once()

	m := NewLocaleMiddleware(jwtSvc, userRepo, testLog())
	c := newLocaleReq(t, "", "en-US", "Bearer "+tokenStr, "en")

	if err := m.Negotiate(func(c echo.Context) error { return nil })(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := LocaleFromContext(c); got != "id" {
		t.Errorf("expected member preference id to win over Accept-Language/cookie, got %q", got)
	}
}

func TestNegotiate_AcceptLanguage_BeatsCookie(t *testing.T) {
	m := NewLocaleMiddleware(jwtservice.NewJWTService("secret"), mocks.NewMockUserRepository(t), testLog())
	c := newLocaleReq(t, "", "id-ID", "", "en")

	if err := m.Negotiate(func(c echo.Context) error { return nil })(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := LocaleFromContext(c); got != "id" {
		t.Errorf("expected Accept-Language id to win over cookie, got %q", got)
	}
}

func TestNegotiate_GuestCookie_UsedWhenNothingElseMatches(t *testing.T) {
	m := NewLocaleMiddleware(jwtservice.NewJWTService("secret"), mocks.NewMockUserRepository(t), testLog())
	c := newLocaleReq(t, "", "", "", "id")

	if err := m.Negotiate(func(c echo.Context) error { return nil })(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := LocaleFromContext(c); got != "id" {
		t.Errorf("expected guest cookie id to be used, got %q", got)
	}
}

func TestNegotiate_NothingMatches_DefaultsToEN(t *testing.T) {
	m := NewLocaleMiddleware(jwtservice.NewJWTService("secret"), mocks.NewMockUserRepository(t), testLog())
	c := newLocaleReq(t, "", "", "", "")

	if err := m.Negotiate(func(c echo.Context) error { return nil })(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := LocaleFromContext(c); got != "en" {
		t.Errorf("expected default en, got %q", got)
	}
}

// An invalid/expired Bearer token during locale negotiation must not error
// out the request — it should just fall through to the next signal.
func TestNegotiate_InvalidToken_FallsThroughGracefully(t *testing.T) {
	m := NewLocaleMiddleware(jwtservice.NewJWTService("secret"), mocks.NewMockUserRepository(t), testLog())
	c := newLocaleReq(t, "", "id-ID", "Bearer garbage", "")

	if err := m.Negotiate(func(c echo.Context) error { return nil })(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := LocaleFromContext(c); got != "id" {
		t.Errorf("expected fallthrough to Accept-Language id, got %q", got)
	}
}

func TestNegotiate_MemberWithUnsupportedPreference_FallsThrough(t *testing.T) {
	jwtSvc := jwtservice.NewJWTService("secret")
	tokenStr, err := jwtSvc.GenerateAccessToken("user-1", 0, time.Hour)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	userRepo := mocks.NewMockUserRepository(t)
	userRepo.EXPECT().FindByID(mock.Anything, "user-1").Return(&account.User{ID: "user-1", PreferredLocale: ""}, nil).Once()

	m := NewLocaleMiddleware(jwtSvc, userRepo, testLog())
	c := newLocaleReq(t, "", "id-ID", "Bearer "+tokenStr, "")

	if err := m.Negotiate(func(c echo.Context) error { return nil })(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := LocaleFromContext(c); got != "id" {
		t.Errorf("expected fallthrough to Accept-Language id when member has no supported preference, got %q", got)
	}
}
