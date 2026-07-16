package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aprxty3/your_persona_controller.git/internal/application/auth"
	authmocks "github.com/aprxty3/your_persona_controller.git/internal/application/auth/mocks"
	"github.com/aprxty3/your_persona_controller.git/internal/domain/account"
	accountmocks "github.com/aprxty3/your_persona_controller.git/internal/domain/account/mocks"
	jwtservice "github.com/aprxty3/your_persona_controller.git/internal/infrastructure/jwt"
	"github.com/aprxty3/your_persona_controller.git/internal/interfaces/http/middleware"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/mock"
	"golang.org/x/crypto/bcrypt"
)

func newAuthCtx(method, path, body string) (echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	return e.NewContext(req, rec), rec
}

func newTestAuthHandler(
	createGuestSessionUC *auth.CreateGuestSessionUseCase,
	registerUC *auth.RegisterUseCase,
	accountUC *auth.AccountUseCase,
	sessionUC *auth.SessionUseCase,
	turnstile auth.TurnstileVerifier,
) *AuthHandler {
	return NewAuthHandler(createGuestSessionUC, registerUC, accountUC, sessionUC, turnstile, false, testLog())
}

// --- CreateGuestSession ---

func TestCreateGuestSession_Success_201SetsCookie(t *testing.T) {
	guestRepo := accountmocks.NewMockGuestSessionRepository(t)
	guestRepo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil).Once()

	uc := auth.NewCreateGuestSessionUseCase(guestRepo, testLog())
	h := newTestAuthHandler(uc, nil, nil, nil, nil)
	c, rec := newAuthCtx(http.MethodPost, "/v1/guest-session", `{"display_name":"Alice","age":20,"status":"student","locale":"en"}`)

	if err := h.CreateGuestSession(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	found := false
	for _, ck := range rec.Result().Cookies() {
		if ck.Name == "session_id" {
			found = true
		}
	}
	if !found {
		t.Error("expected a session_id cookie to be set")
	}
}

func TestCreateGuestSession_ValidationError_400(t *testing.T) {
	uc := auth.NewCreateGuestSessionUseCase(accountmocks.NewMockGuestSessionRepository(t), testLog())
	h := newTestAuthHandler(uc, nil, nil, nil, nil)
	c, rec := newAuthCtx(http.MethodPost, "/v1/guest-session", `{"display_name":"","age":20,"status":"student","locale":"en"}`)

	if err := h.CreateGuestSession(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	body := decodeResponse(t, rec)
	if body.Error == nil || body.Error.Code != "VALIDATION_ERROR" {
		t.Errorf("expected VALIDATION_ERROR, got %+v", body.Error)
	}
}

// --- Register / Login / ForgotPassword: Turnstile gate ---

func TestRegister_MissingTurnstileToken_400_NoVerifierCall(t *testing.T) {
	// No mock.On() registered on the verifier — a call would panic, proving
	// verifyTurnstile short-circuits on an empty token before ever invoking it.
	turnstile := authmocks.NewMockTurnstileVerifier(t)
	h := newTestAuthHandler(nil, nil, nil, nil, turnstile)
	c, rec := newAuthCtx(http.MethodPost, "/v1/auth/register", `{"email":"a@example.com","password":"longenoughpassword","preferred_locale":"en"}`)

	// verifyTurnstile rejects and returns errResponseWritten, not nil — the
	// point of this test is that it never reaches h.registerUseCase (nil above).
	_ = h.Register(c)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	body := decodeResponse(t, rec)
	if body.Error == nil || body.Error.Code != "VALIDATION_ERROR" {
		t.Errorf("expected VALIDATION_ERROR, got %+v", body.Error)
	}
}

func TestLogin_MissingTurnstileToken_400(t *testing.T) {
	turnstile := authmocks.NewMockTurnstileVerifier(t)
	h := newTestAuthHandler(nil, nil, nil, nil, turnstile)
	c, rec := newAuthCtx(http.MethodPost, "/v1/auth/login", `{"email":"a@example.com","password":"pw"}`)

	_ = h.Login(c)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestForgotPassword_MissingTurnstileToken_400(t *testing.T) {
	turnstile := authmocks.NewMockTurnstileVerifier(t)
	h := newTestAuthHandler(nil, nil, nil, nil, turnstile)
	c, rec := newAuthCtx(http.MethodPost, "/v1/auth/forgot-password", `{"email":"a@example.com"}`)

	_ = h.ForgotPassword(c)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

// Explicit Cloudflare rejection must map to TURNSTILE_VERIFICATION_FAILED, not a generic 500/400.
func TestLogin_TurnstileExplicitlyRejects_400(t *testing.T) {
	turnstile := authmocks.NewMockTurnstileVerifier(t)
	turnstile.EXPECT().Verify(mock.Anything, "bad-token", mock.Anything).Return(false, nil).Once()

	h := newTestAuthHandler(nil, nil, nil, nil, turnstile)
	c, rec := newAuthCtx(http.MethodPost, "/v1/auth/login", `{"email":"a@example.com","password":"pw","cf_turnstile_response":"bad-token"}`)

	_ = h.Login(c)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	body := decodeResponse(t, rec)
	if body.Error == nil || body.Error.Code != "TURNSTILE_VERIFICATION_FAILED" {
		t.Errorf("expected TURNSTILE_VERIFICATION_FAILED, got %+v", body.Error)
	}
}

// A Turnstile-side error must fail OPEN — login proceeds to the next check.
func TestLogin_TurnstileError_FailsOpenAndContinues(t *testing.T) {
	turnstile := authmocks.NewMockTurnstileVerifier(t)
	turnstile.EXPECT().Verify(mock.Anything, "any-token", mock.Anything).Return(true, assertErrHandler).Once()
	userRepo := accountmocks.NewMockUserRepository(t)
	userRepo.EXPECT().FindByEmail(mock.Anything, "a@example.com").Return(nil, nil).Once()
	ipLimiter := authmocks.NewMockIPRateLimiter(t)
	ipLimiter.EXPECT().Allow(mock.Anything, mock.Anything, mock.Anything).Return(true, 0, nil).Once()

	sessionUC := auth.NewSessionUseCase(nil, userRepo, accountmocks.NewMockVerificationTokenRepository(t), nil, jwtservice.NewJWTService("secret"), authmocks.NewMockSessionTokenStore(t), ipLimiter, testLog())
	h := newTestAuthHandler(nil, nil, nil, sessionUC, turnstile)
	c, rec := newAuthCtx(http.MethodPost, "/v1/auth/login", `{"email":"a@example.com","password":"pw","cf_turnstile_response":"any-token"}`)

	if err := h.Login(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Reaches Login use case (INVALID_CREDENTIALS from user-not-found), proving
	// the turnstile error did not block the request.
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 (fell through to login logic), got %d: %s", rec.Code, rec.Body.String())
	}
}

// --- VerifyEmailOTP ---

func TestVerifyEmailOTP_MissingFields_400(t *testing.T) {
	h := newTestAuthHandler(nil, nil, nil, auth.NewSessionUseCase(nil, nil, nil, nil, jwtservice.NewJWTService("secret"), nil, nil, testLog()), nil)
	c, rec := newAuthCtx(http.MethodPost, "/v1/auth/verify-email-otp", `{"email":"","otp":""}`)

	if err := h.VerifyEmailOTP(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestVerifyEmailOTP_UserNotFound_InvalidOTP400(t *testing.T) {
	userRepo := accountmocks.NewMockUserRepository(t)
	userRepo.EXPECT().FindByEmail(mock.Anything, "a@example.com").Return(nil, nil).Once()

	sessionUC := auth.NewSessionUseCase(nil, userRepo, accountmocks.NewMockVerificationTokenRepository(t), nil, jwtservice.NewJWTService("secret"), nil, nil, testLog())
	h := newTestAuthHandler(nil, nil, nil, sessionUC, nil)
	c, rec := newAuthCtx(http.MethodPost, "/v1/auth/verify-email-otp", `{"email":"a@example.com","otp":"123456"}`)

	_ = h.VerifyEmailOTP(c)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	body := decodeResponse(t, rec)
	if body.Error == nil || body.Error.Code != "INVALID_OTP" {
		t.Errorf("expected INVALID_OTP, got %+v", body.Error)
	}
}

func TestVerifyEmailOTP_Success_200(t *testing.T) {
	userRepo := accountmocks.NewMockUserRepository(t)
	userRepo.EXPECT().FindByEmail(mock.Anything, "a@example.com").Return(&account.User{ID: "user-1"}, nil).Once()
	userRepo.EXPECT().Update(mock.Anything, mock.Anything).Return(nil).Once()
	tokenRepo := accountmocks.NewMockVerificationTokenRepository(t)
	tokenRepo.EXPECT().FindActiveByUserAndType(mock.Anything, "user-1", account.TokenTypeEmailVerification).
		Return(&account.VerificationToken{ID: "tok-1", Token: "123456", ExpiresAt: time.Now().Add(time.Hour)}, nil).Once()
	tokenRepo.EXPECT().MarkUsed(mock.Anything, "tok-1").Return(nil).Once()

	sessionUC := auth.NewSessionUseCase(nil, userRepo, tokenRepo, nil, jwtservice.NewJWTService("secret"), nil, nil, testLog())
	h := newTestAuthHandler(nil, nil, nil, sessionUC, nil)
	c, rec := newAuthCtx(http.MethodPost, "/v1/auth/verify-email-otp", `{"email":"a@example.com","otp":"123456"}`)

	if err := h.VerifyEmailOTP(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "access_token") {
		t.Errorf("expected access_token in response, got: %s", rec.Body.String())
	}
}

// --- ResendEmailOTP ---

func TestResendEmailOTP_MissingEmail_400(t *testing.T) {
	h := newTestAuthHandler(nil, nil, auth.NewAccountUseCase(nil, nil, nil, nil, testLog()), nil, nil)
	c, rec := newAuthCtx(http.MethodPost, "/v1/auth/resend-email-otp", `{"email":""}`)

	if err := h.ResendEmailOTP(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestResendEmailOTP_RateLimited_429(t *testing.T) {
	rateLimiter := authmocks.NewMockOTPRateLimiter(t)
	rateLimiter.EXPECT().CheckAndConsume(mock.Anything, mock.Anything, "a@example.com").Return(30, nil).Once()

	accountUC := auth.NewAccountUseCase(accountmocks.NewMockUserRepository(t), accountmocks.NewMockVerificationTokenRepository(t), nil, rateLimiter, testLog())
	h := newTestAuthHandler(nil, nil, accountUC, nil, nil)
	c, rec := newAuthCtx(http.MethodPost, "/v1/auth/resend-email-otp", `{"email":"a@example.com"}`)

	if err := h.ResendEmailOTP(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d: %s", rec.Code, rec.Body.String())
	}
	body := decodeResponse(t, rec)
	if body.Error == nil || body.Error.Code != "RATE_LIMITED" {
		t.Errorf("expected RATE_LIMITED, got %+v", body.Error)
	}
	if body.Meta == nil {
		t.Error("expected meta.retry_after_seconds to be present")
	}
}

// Anti-enumeration: unregistered email still returns 200 with the generic message.
func TestResendEmailOTP_UnregisteredEmail_StillReturns200(t *testing.T) {
	rateLimiter := authmocks.NewMockOTPRateLimiter(t)
	rateLimiter.EXPECT().CheckAndConsume(mock.Anything, mock.Anything, "nobody@example.com").Return(0, nil).Once()
	userRepo := accountmocks.NewMockUserRepository(t)
	userRepo.EXPECT().FindByEmail(mock.Anything, "nobody@example.com").Return(nil, nil).Once()

	accountUC := auth.NewAccountUseCase(userRepo, accountmocks.NewMockVerificationTokenRepository(t), nil, rateLimiter, testLog())
	h := newTestAuthHandler(nil, nil, accountUC, nil, nil)
	c, rec := newAuthCtx(http.MethodPost, "/v1/auth/resend-email-otp", `{"email":"nobody@example.com"}`)

	if err := h.ResendEmailOTP(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 (anti-enumeration), got %d: %s", rec.Code, rec.Body.String())
	}
}

// --- Login ---

func TestLogin_MissingFields_400(t *testing.T) {
	turnstile := authmocks.NewMockTurnstileVerifier(t)
	turnstile.EXPECT().Verify(mock.Anything, "tok", mock.Anything).Return(true, nil).Once()
	sessionUC := auth.NewSessionUseCase(nil, nil, nil, nil, jwtservice.NewJWTService("secret"), nil, nil, testLog())
	h := newTestAuthHandler(nil, nil, nil, sessionUC, turnstile)
	c, rec := newAuthCtx(http.MethodPost, "/v1/auth/login", `{"email":"","password":"","cf_turnstile_response":"tok"}`)

	_ = h.Login(c)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestLogin_InvalidCredentials_401(t *testing.T) {
	turnstile := authmocks.NewMockTurnstileVerifier(t)
	turnstile.EXPECT().Verify(mock.Anything, "tok", mock.Anything).Return(true, nil).Once()
	ipLimiter := authmocks.NewMockIPRateLimiter(t)
	ipLimiter.EXPECT().Allow(mock.Anything, mock.Anything, mock.Anything).Return(true, 0, nil).Once()
	userRepo := accountmocks.NewMockUserRepository(t)
	userRepo.EXPECT().FindByEmail(mock.Anything, "a@example.com").Return(nil, nil).Once()

	sessionUC := auth.NewSessionUseCase(nil, userRepo, nil, nil, jwtservice.NewJWTService("secret"), nil, ipLimiter, testLog())
	h := newTestAuthHandler(nil, nil, nil, sessionUC, turnstile)
	c, rec := newAuthCtx(http.MethodPost, "/v1/auth/login", `{"email":"a@example.com","password":"wrongpassword","cf_turnstile_response":"tok"}`)

	if err := h.Login(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", rec.Code, rec.Body.String())
	}
	body := decodeResponse(t, rec)
	if body.Error == nil || body.Error.Code != "INVALID_CREDENTIALS" {
		t.Errorf("expected INVALID_CREDENTIALS, got %+v", body.Error)
	}
}

func TestLogin_AccountLocked_423(t *testing.T) {
	turnstile := authmocks.NewMockTurnstileVerifier(t)
	turnstile.EXPECT().Verify(mock.Anything, "tok", mock.Anything).Return(true, nil).Once()
	ipLimiter := authmocks.NewMockIPRateLimiter(t)
	ipLimiter.EXPECT().Allow(mock.Anything, mock.Anything, mock.Anything).Return(true, 0, nil).Once()
	lockedUntil := time.Now().Add(time.Hour)
	userRepo := accountmocks.NewMockUserRepository(t)
	userRepo.EXPECT().FindByEmail(mock.Anything, "a@example.com").Return(&account.User{ID: "user-1", LockedUntil: &lockedUntil}, nil).Once()

	sessionUC := auth.NewSessionUseCase(nil, userRepo, nil, nil, jwtservice.NewJWTService("secret"), nil, ipLimiter, testLog())
	h := newTestAuthHandler(nil, nil, nil, sessionUC, turnstile)
	c, rec := newAuthCtx(http.MethodPost, "/v1/auth/login", `{"email":"a@example.com","password":"pw","cf_turnstile_response":"tok"}`)

	if err := h.Login(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusLocked {
		t.Fatalf("expected 423, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestLogin_EmailNotVerified_403(t *testing.T) {
	turnstile := authmocks.NewMockTurnstileVerifier(t)
	turnstile.EXPECT().Verify(mock.Anything, "tok", mock.Anything).Return(true, nil).Once()
	ipLimiter := authmocks.NewMockIPRateLimiter(t)
	ipLimiter.EXPECT().Allow(mock.Anything, mock.Anything, mock.Anything).Return(true, 0, nil).Once()
	userRepo := accountmocks.NewMockUserRepository(t)
	userRepo.EXPECT().FindByEmail(mock.Anything, "a@example.com").Return(&account.User{ID: "user-1", EmailVerifiedAt: nil}, nil).Once()

	sessionUC := auth.NewSessionUseCase(nil, userRepo, nil, nil, jwtservice.NewJWTService("secret"), nil, ipLimiter, testLog())
	h := newTestAuthHandler(nil, nil, nil, sessionUC, turnstile)
	c, rec := newAuthCtx(http.MethodPost, "/v1/auth/login", `{"email":"a@example.com","password":"pw","cf_turnstile_response":"tok"}`)

	if err := h.Login(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestLogin_Success_200(t *testing.T) {
	turnstile := authmocks.NewMockTurnstileVerifier(t)
	turnstile.EXPECT().Verify(mock.Anything, "tok", mock.Anything).Return(true, nil).Once()
	ipLimiter := authmocks.NewMockIPRateLimiter(t)
	ipLimiter.EXPECT().Allow(mock.Anything, mock.Anything, mock.Anything).Return(true, 0, nil).Once()
	verifiedAt := time.Now().Add(-time.Hour)
	hash, err := bcrypt.GenerateFromPassword([]byte("correct-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	userRepo := accountmocks.NewMockUserRepository(t)
	userRepo.EXPECT().FindByEmail(mock.Anything, "a@example.com").
		Return(&account.User{ID: "user-1", PasswordHash: string(hash), EmailVerifiedAt: &verifiedAt}, nil).Once()

	sessionUC := auth.NewSessionUseCase(nil, userRepo, nil, nil, jwtservice.NewJWTService("secret"), nil, ipLimiter, testLog())
	h := newTestAuthHandler(nil, nil, nil, sessionUC, turnstile)
	c, rec := newAuthCtx(http.MethodPost, "/v1/auth/login", `{"email":"a@example.com","password":"correct-password","cf_turnstile_response":"tok"}`)

	if err := h.Login(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "access_token") {
		t.Errorf("expected access_token in response, got: %s", rec.Body.String())
	}
}

func TestLogin_RateLimited_429(t *testing.T) {
	turnstile := authmocks.NewMockTurnstileVerifier(t)
	turnstile.EXPECT().Verify(mock.Anything, "tok", mock.Anything).Return(true, nil).Once()
	ipLimiter := authmocks.NewMockIPRateLimiter(t)
	ipLimiter.EXPECT().Allow(mock.Anything, mock.Anything, mock.Anything).Return(false, 60, nil).Once()

	sessionUC := auth.NewSessionUseCase(nil, nil, nil, nil, jwtservice.NewJWTService("secret"), nil, ipLimiter, testLog())
	h := newTestAuthHandler(nil, nil, nil, sessionUC, turnstile)
	c, rec := newAuthCtx(http.MethodPost, "/v1/auth/login", `{"email":"a@example.com","password":"pw","cf_turnstile_response":"tok"}`)

	if err := h.Login(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d: %s", rec.Code, rec.Body.String())
	}
}

// --- ChangePassword ---

func newAuthedAuthCtx(body, userID string) (echo.Context, *httptest.ResponseRecorder) {
	c, rec := newAuthCtx(http.MethodPost, "/v1/auth/change-password", body)
	c.Set(middleware.ContextUserID, userID)
	return c, rec
}

func TestChangePassword_ConfirmationMismatch_400(t *testing.T) {
	sessionUC := auth.NewSessionUseCase(nil, nil, nil, nil, jwtservice.NewJWTService("secret"), nil, nil, testLog())
	h := newTestAuthHandler(nil, nil, nil, sessionUC, nil)
	c, rec := newAuthedAuthCtx(`{"old_password":"oldpassword1","new_password":"newpassword1","retry_new_password":"different1"}`, "user-1")

	if err := h.ChangePassword(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	body := decodeResponse(t, rec)
	if body.Error == nil || body.Error.Code != "PASSWORD_CONFIRMATION_MISMATCH" {
		t.Errorf("expected PASSWORD_CONFIRMATION_MISMATCH, got %+v", body.Error)
	}
}

func TestChangePassword_TooShort_400(t *testing.T) {
	sessionUC := auth.NewSessionUseCase(nil, nil, nil, authmocks.NewMockPasswordBreachChecker(t), jwtservice.NewJWTService("secret"), nil, nil, testLog())
	h := newTestAuthHandler(nil, nil, nil, sessionUC, nil)
	c, rec := newAuthedAuthCtx(`{"old_password":"oldpassword1","new_password":"short","retry_new_password":"short"}`, "user-1")

	if err := h.ChangePassword(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	body := decodeResponse(t, rec)
	if body.Error == nil || body.Error.Code != "PASSWORD_TOO_SHORT" {
		t.Errorf("expected PASSWORD_TOO_SHORT, got %+v", body.Error)
	}
}

func TestChangePassword_OldPasswordWrong_401(t *testing.T) {
	breachChecker := authmocks.NewMockPasswordBreachChecker(t)
	breachChecker.EXPECT().IsBreached(mock.Anything, "newpassword1").Return(false, nil).Once()
	hash, err := bcrypt.GenerateFromPassword([]byte("correct-old-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	userRepo := accountmocks.NewMockUserRepository(t)
	userRepo.EXPECT().FindByID(mock.Anything, "user-1").Return(&account.User{ID: "user-1", PasswordHash: string(hash)}, nil).Once()

	sessionUC := auth.NewSessionUseCase(nil, userRepo, nil, breachChecker, jwtservice.NewJWTService("secret"), nil, nil, testLog())
	h := newTestAuthHandler(nil, nil, nil, sessionUC, nil)
	c, rec := newAuthedAuthCtx(`{"old_password":"wrong-old-password","new_password":"newpassword1","retry_new_password":"newpassword1"}`, "user-1")

	if err := h.ChangePassword(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", rec.Code, rec.Body.String())
	}
	body := decodeResponse(t, rec)
	if body.Error == nil || body.Error.Code != "INVALID_CREDENTIALS" {
		t.Errorf("expected INVALID_CREDENTIALS, got %+v", body.Error)
	}
}

// --- RefreshToken ---

func TestRefreshToken_MissingField_400(t *testing.T) {
	sessionUC := auth.NewSessionUseCase(nil, nil, nil, nil, jwtservice.NewJWTService("secret"), nil, nil, testLog())
	h := newTestAuthHandler(nil, nil, nil, sessionUC, nil)
	c, rec := newAuthCtx(http.MethodPost, "/v1/auth/refresh", `{"refresh_token":""}`)

	if err := h.RefreshToken(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestRefreshToken_InvalidToken_401(t *testing.T) {
	sessionUC := auth.NewSessionUseCase(nil, nil, nil, nil, jwtservice.NewJWTService("secret"), nil, nil, testLog())
	h := newTestAuthHandler(nil, nil, nil, sessionUC, nil)
	c, rec := newAuthCtx(http.MethodPost, "/v1/auth/refresh", `{"refresh_token":"not-a-valid-jwt"}`)

	if err := h.RefreshToken(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", rec.Code, rec.Body.String())
	}
	body := decodeResponse(t, rec)
	if body.Error == nil || body.Error.Code != "UNAUTHORIZED" {
		t.Errorf("expected UNAUTHORIZED, got %+v", body.Error)
	}
}

func TestRefreshToken_Success_200(t *testing.T) {
	jwtSvc := jwtservice.NewJWTService("secret")
	refreshToken, err := jwtSvc.GenerateRefreshToken("user-1", 0, time.Hour)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	tokenStore := authmocks.NewMockSessionTokenStore(t)
	tokenStore.EXPECT().IsRefreshJTIDenylisted(mock.Anything, mock.Anything).Return(false, nil).Once()
	tokenStore.EXPECT().DenylistRefreshJTI(mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()
	userRepo := accountmocks.NewMockUserRepository(t)
	userRepo.EXPECT().FindByID(mock.Anything, "user-1").Return(&account.User{ID: "user-1", TokenVersion: 0}, nil).Once()

	sessionUC := auth.NewSessionUseCase(nil, userRepo, nil, nil, jwtSvc, tokenStore, nil, testLog())
	h := newTestAuthHandler(nil, nil, nil, sessionUC, nil)
	c, rec := newAuthCtx(http.MethodPost, "/v1/auth/refresh", `{"refresh_token":"`+refreshToken+`"}`)

	if err := h.RefreshToken(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestRefreshToken_TokenVersionMismatch_401(t *testing.T) {
	jwtSvc := jwtservice.NewJWTService("secret")
	refreshToken, err := jwtSvc.GenerateRefreshToken("user-1", 5, time.Hour)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	tokenStore := authmocks.NewMockSessionTokenStore(t)
	tokenStore.EXPECT().IsRefreshJTIDenylisted(mock.Anything, mock.Anything).Return(false, nil).Once()
	userRepo := accountmocks.NewMockUserRepository(t)
	userRepo.EXPECT().FindByID(mock.Anything, "user-1").Return(&account.User{ID: "user-1", TokenVersion: 1}, nil).Once()

	sessionUC := auth.NewSessionUseCase(nil, userRepo, nil, nil, jwtSvc, tokenStore, nil, testLog())
	h := newTestAuthHandler(nil, nil, nil, sessionUC, nil)
	c, rec := newAuthCtx(http.MethodPost, "/v1/auth/refresh", `{"refresh_token":"`+refreshToken+`"}`)

	if err := h.RefreshToken(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", rec.Code, rec.Body.String())
	}
	body := decodeResponse(t, rec)
	if body.Error == nil || body.Error.Code != "TOKEN_VERSION_MISMATCH" {
		t.Errorf("expected TOKEN_VERSION_MISMATCH, got %+v", body.Error)
	}
}

// --- Logout / LogoutAll ---

func TestLogout_MissingField_400(t *testing.T) {
	sessionUC := auth.NewSessionUseCase(nil, nil, nil, nil, jwtservice.NewJWTService("secret"), nil, nil, testLog())
	h := newTestAuthHandler(nil, nil, nil, sessionUC, nil)
	c, rec := newAuthedAuthCtx(`{"refresh_token":""}`, "user-1")

	if err := h.Logout(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestLogout_Success_200(t *testing.T) {
	jwtSvc := jwtservice.NewJWTService("secret")
	refreshToken, err := jwtSvc.GenerateRefreshToken("user-1", 0, time.Hour)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	tokenStore := authmocks.NewMockSessionTokenStore(t)
	tokenStore.EXPECT().DenylistRefreshJTI(mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()

	sessionUC := auth.NewSessionUseCase(nil, nil, nil, nil, jwtSvc, tokenStore, nil, testLog())
	h := newTestAuthHandler(nil, nil, nil, sessionUC, nil)
	c, rec := newAuthedAuthCtx(`{"refresh_token":"`+refreshToken+`"}`, "user-1")

	if err := h.Logout(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestLogoutAll_Success_200(t *testing.T) {
	userRepo := accountmocks.NewMockUserRepository(t)
	userRepo.EXPECT().IncrementTokenVersion(mock.Anything, "user-1").Return(nil).Once()

	sessionUC := auth.NewSessionUseCase(nil, userRepo, nil, nil, jwtservice.NewJWTService("secret"), nil, nil, testLog())
	h := newTestAuthHandler(nil, nil, nil, sessionUC, nil)
	c, rec := newAuthedAuthCtx("", "user-1")

	if err := h.LogoutAll(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

// --- VerifyResetOTP / ResetPassword ---

func TestVerifyResetOTP_Success_200(t *testing.T) {
	userRepo := accountmocks.NewMockUserRepository(t)
	userRepo.EXPECT().FindByEmail(mock.Anything, "a@example.com").Return(&account.User{ID: "user-1"}, nil).Once()
	tokenRepo := accountmocks.NewMockVerificationTokenRepository(t)
	tokenRepo.EXPECT().FindActiveByUserAndType(mock.Anything, "user-1", account.TokenTypePasswordReset).
		Return(&account.VerificationToken{ID: "tok-1", Token: "654321", ExpiresAt: time.Now().Add(time.Hour)}, nil).Once()
	tokenRepo.EXPECT().MarkUsed(mock.Anything, "tok-1").Return(nil).Once()
	tokenStore := authmocks.NewMockSessionTokenStore(t)
	tokenStore.EXPECT().StoreResetJTI(mock.Anything, mock.Anything, "user-1", mock.Anything).Return(nil).Once()

	sessionUC := auth.NewSessionUseCase(nil, userRepo, tokenRepo, nil, jwtservice.NewJWTService("secret"), tokenStore, nil, testLog())
	h := newTestAuthHandler(nil, nil, nil, sessionUC, nil)
	c, rec := newAuthCtx(http.MethodPost, "/v1/auth/verify-reset-otp", `{"email":"a@example.com","otp":"654321"}`)

	if err := h.VerifyResetOTP(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "reset_token") {
		t.Errorf("expected reset_token in response, got: %s", rec.Body.String())
	}
}

func TestResetPassword_InvalidToken_401(t *testing.T) {
	breachChecker := authmocks.NewMockPasswordBreachChecker(t)
	breachChecker.EXPECT().IsBreached(mock.Anything, "newpassword1").Return(false, nil).Once()
	sessionUC := auth.NewSessionUseCase(nil, nil, nil, breachChecker, jwtservice.NewJWTService("secret"), nil, nil, testLog())
	h := newTestAuthHandler(nil, nil, nil, sessionUC, nil)
	c, rec := newAuthCtx(http.MethodPost, "/v1/auth/reset-password", `{"reset_token":"not-a-valid-jwt","new_password":"newpassword1"}`)

	if err := h.ResetPassword(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", rec.Code, rec.Body.String())
	}
	body := decodeResponse(t, rec)
	if body.Error == nil || body.Error.Code != "UNAUTHORIZED" {
		t.Errorf("expected UNAUTHORIZED, got %+v", body.Error)
	}
}

func TestResetPassword_TooShort_400(t *testing.T) {
	sessionUC := auth.NewSessionUseCase(nil, nil, nil, authmocks.NewMockPasswordBreachChecker(t), jwtservice.NewJWTService("secret"), nil, nil, testLog())
	h := newTestAuthHandler(nil, nil, nil, sessionUC, nil)
	c, rec := newAuthCtx(http.MethodPost, "/v1/auth/reset-password", `{"reset_token":"whatever","new_password":"short"}`)

	if err := h.ResetPassword(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	body := decodeResponse(t, rec)
	if body.Error == nil || body.Error.Code != "PASSWORD_TOO_SHORT" {
		t.Errorf("expected PASSWORD_TOO_SHORT, got %+v", body.Error)
	}
}
