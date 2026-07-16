package handler

import (
	"errors"
	"net/http"

	"github.com/aprxty3/your_persona_controller.git/internal/application"
	"github.com/aprxty3/your_persona_controller.git/internal/application/auth"
	"github.com/aprxty3/your_persona_controller.git/internal/interfaces/http/dto"
	"github.com/aprxty3/your_persona_controller.git/internal/interfaces/http/middleware"
	"github.com/aprxty3/your_persona_controller.git/pkg/httpresponse"
	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
	"github.com/labstack/echo/v4"
)

type AuthHandler struct {
	createGuestSessionUseCase *auth.CreateGuestSessionUseCase
	registerUseCase           *auth.RegisterUseCase
	accountUseCase            *auth.AccountUseCase
	sessionUseCase            *auth.SessionUseCase
	turnstileVerifier         auth.TurnstileVerifier
	isProduction              bool
	log                       logger.Logger
}

// NewAuthHandler is the constructor for Dependency Injection.
func NewAuthHandler(
	createGuestSessionUseCase *auth.CreateGuestSessionUseCase,
	registerUseCase *auth.RegisterUseCase,
	accountUseCase *auth.AccountUseCase,
	sessionUseCase *auth.SessionUseCase,
	turnstileVerifier auth.TurnstileVerifier,
	isProduction bool,
	log logger.Logger,
) *AuthHandler {
	return &AuthHandler{
		createGuestSessionUseCase: createGuestSessionUseCase,
		registerUseCase:           registerUseCase,
		accountUseCase:            accountUseCase,
		sessionUseCase:            sessionUseCase,
		turnstileVerifier:         turnstileVerifier,
		isProduction:              isProduction,
		log:                       log.With("handler", "auth"),
	}
}

// verifyTurnstile gates Register/Login/ForgotPassword against Cloudflare
// Turnstile — shared by all three so the bot-check logic exists in exactly
// one place. Returns nil when verification passes (including the fail-open
// path on a Cloudflare-side error) and an already-written HTTP response
// error otherwise (VALIDATION_ERROR for a missing token, TURNSTILE_VERIFICATION_FAILED
// for an explicit Cloudflare rejection).
func (h *AuthHandler) verifyTurnstile(c echo.Context, token string) error {
	if token == "" {
		httpresponse.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "cf_turnstile_response is required")
		return errResponseWritten
	}

	ok, err := h.turnstileVerifier.Verify(c.Request().Context(), token, c.RealIP())
	if err != nil {
		h.log.Warn("turnstile verify error, failing open", "error", err)
		return nil
	}
	if !ok {
		httpcallErrorCustom(c, http.StatusBadRequest, "TURNSTILE_VERIFICATION_FAILED", "Bot verification failed. Please retry the challenge and try again")
		return errResponseWritten
	}
	return nil
}

// rateLimitedResponse is the shared 429 shape for every rate-limited auth
// flow (register, login, resend-otp, forgot-password) — always RATE_LIMITED
// with a retry_after_seconds meta field; only the message text differs.
func rateLimitedResponse(c echo.Context, retryAfterSeconds int, message string) error {
	return c.JSON(http.StatusTooManyRequests, httpresponse.Response{
		Success: false,
		Error:   &httpresponse.ErrorDetail{Code: "RATE_LIMITED", Message: message},
		Meta:    map[string]interface{}{"retry_after_seconds": retryAfterSeconds},
	})
}

// otpVerifyError maps the verification-error triple shared by VerifyEmailOTP
// and VerifyResetOTP (wrong code / expired / max attempts) to its HTTP
// response, embedding attempts_remaining and a flow-specific "request a new
// OTP via ..." hint. Returns nil if err isn't one of the three — the caller
// falls through to its own default (log + httpcallError) in that case.
func otpVerifyError(c echo.Context, err error, attemptsRemaining int, resendPath string) error {
	meta := map[string]interface{}{"attempts_remaining": attemptsRemaining}
	switch {
	case errors.Is(err, application.ErrInvalidOTP):
		c.JSON(http.StatusBadRequest, httpresponse.Response{
			Success: false,
			Error:   &httpresponse.ErrorDetail{Code: "INVALID_OTP", Message: "The OTP code is incorrect"},
			Meta:    meta,
		})
		return errResponseWritten
	case errors.Is(err, application.ErrOTPExpired):
		c.JSON(http.StatusBadRequest, httpresponse.Response{
			Success: false,
			Error:   &httpresponse.ErrorDetail{Code: "OTP_EXPIRED", Message: "The OTP code has expired. Please request a new one via " + resendPath},
			Meta:    meta,
		})
		return errResponseWritten
	case errors.Is(err, application.ErrOTPMaxAttempts):
		c.JSON(http.StatusTooManyRequests, httpresponse.Response{
			Success: false,
			Error:   &httpresponse.ErrorDetail{Code: "OTP_MAX_ATTEMPTS", Message: "Maximum verification attempts exceeded. Please request a new OTP via " + resendPath},
			Meta:    meta,
		})
		return errResponseWritten
	default:
		return nil
	}
}

// CreateGuestSession handles POST /v1/guest-session
// @Summary      Create a guest session
// @Description  Starts an anonymous onboarding session before the user registers an account.
// @Description  Sets a `session_id` HttpOnly cookie that is automatically carried through to `/auth/register`
// @Description  to link the guest session to the new account.
// @Description
// @Description  **status** — accepted values:
// @Description  - `student`    — currently studying at school or university
// @Description  - `worker`     — employed full-time or part-time
// @Description  - `freelancer` — self-employed / project-based work
// @Description  - `unemployed` — not currently working
// @Description  - `other`      — anything that does not fit the options above
// @Description
// @Description  **locale** — accepted values:
// @Description  - `en` — English (UI labels and email content will be in English)
// @Description  - `id` — Indonesian (UI labels and email content will be in Bahasa Indonesia)
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Param        request body dto.CreateGuestSessionRequestDTO true "Guest Session Onboarding Data"
// @Success      201 {object} httpresponse.Response{data=auth.CreateGuestSessionResponse} "Guest session created. session_id cookie is set."
// @Failure      400 {object} httpresponse.Response "VALIDATION_ERROR — one or more fields are missing or have an invalid value (e.g., unrecognised status or locale)"
// @Failure      500 {object} httpresponse.Response "INTERNAL_ERROR — unexpected server error"
// @Router       /v1/guest-session [post]
func (h *AuthHandler) CreateGuestSession(c echo.Context) error {
	var payload dto.CreateGuestSessionRequestDTO
	if err := bindJSON(c, h.log, "create guest session", &payload); err != nil {
		return err
	}

	ucReq := auth.CreateGuestSessionRequest{
		DisplayName: payload.DisplayName,
		Age:         payload.Age,
		Status:      payload.Status,
		Locale:      payload.Locale,
		IPAddress:   c.RealIP(),
	}

	resp, err := h.createGuestSessionUseCase.Execute(c.Request().Context(), ucReq)
	if err != nil {
		if errors.Is(err, application.ErrInvalidInput) {
			return httpresponse.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", unwrapMessage(err))
		}
		h.log.Error("create guest session failed", "error", err)
		return httpcallError(c, err)
	}

	// Set HttpOnly cookie so /auth/register can automatically claim the session.
	c.SetCookie(&http.Cookie{
		Name:     "session_id",
		Value:    resp.SessionID,
		Expires:  resp.ExpiresAt,
		Path:     "/",
		HttpOnly: true,
		Secure:   h.isProduction,
		SameSite: http.SameSiteStrictMode,
	})

	return httpresponse.Success(c, http.StatusCreated, resp, nil)
}

// Register handles POST /v1/auth/register
// @Summary      Register a new user account
// @Description  Creates a new member account. If a valid `session_id` cookie from `/v1/guest-session`
// @Description  is present, the guest session data (display name, age, status, locale) is automatically
// @Description  linked to the new account — you do NOT need to re-submit that data here.
// @Description
// @Description  After successful registration, a 6-digit OTP is sent to the provided email address.
// @Description  The account is not fully active until the OTP is verified via `/v1/auth/verify-email-otp`.
// @Description
// @Description  **preferred_locale** — accepted values:
// @Description  - `en` — English
// @Description  - `id` — Indonesian (Bahasa Indonesia)
// @Description
// @Description  **referral_code** — completely optional.
// @Description  If you do not have a referral code, omit the field, set it to `null`, or send `""` — all three are treated identically.
// @Description
// @Description  **cf_turnstile_response** — required. The token returned by the Cloudflare Turnstile
// @Description  widget after the user completes the challenge.
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Param        request body dto.RegisterRequestDTO true "Registration Data"
// @Success      201 {object} httpresponse.Response{data=auth.RegisterResponse} "Account created. OTP sent to email. Verify via /auth/verify-email-otp."
// @Failure      400 {object} httpresponse.Response "VALIDATION_ERROR | EMAIL_ALREADY_REGISTERED | PASSWORD_TOO_SHORT | TURNSTILE_VERIFICATION_FAILED"
// @Failure      409 {object} httpresponse.Response "EMAIL_ALREADY_REGISTERED — email is already in use"
// @Failure      429 {object} httpresponse.Response "RATE_LIMITED — too many registration attempts from this IP. Check meta.retry_after_seconds."
// @Failure      500 {object} httpresponse.Response "INTERNAL_ERROR — unexpected server error"
// @Router       /v1/auth/register [post]
func (h *AuthHandler) Register(c echo.Context) error {
	var payload dto.RegisterRequestDTO
	if err := bindJSON(c, h.log, "register", &payload); err != nil {
		return err
	}
	if err := h.verifyTurnstile(c, payload.CFTurnstileResponse); err != nil {
		return err
	}

	// Read guest session ID from cookie if present.
	var guestSessionID *string
	if cookie, err := c.Cookie("session_id"); err == nil && cookie != nil {
		val := cookie.Value
		guestSessionID = &val
	}

	ucReq := auth.RegisterRequest{
		Email:           payload.Email,
		Password:        payload.Password,
		PreferredLocale: payload.PreferredLocale,
		ReferralCode:    payload.ReferralCode,
		GuestSessionID:  guestSessionID,
		IPAddress:       c.RealIP(),
	}

	resp, err := h.registerUseCase.Register(c.Request().Context(), ucReq)
	if err != nil {
		switch {
		case errors.Is(err, application.ErrInvalidInput):
			return httpresponse.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", unwrapMessage(err))
		case errors.Is(err, application.ErrEmailAlreadyRegistered):
			return httpcallErrorCustom(c, http.StatusConflict, "EMAIL_ALREADY_REGISTERED", "An account with this email address already exists")
		case errors.Is(err, application.ErrPasswordTooShort):
			return httpcallErrorCustom(c, http.StatusBadRequest, "PASSWORD_TOO_SHORT", "Password must be at least 10 characters long")
		case errors.Is(err, application.ErrPasswordBreached):
			return httpcallErrorCustom(c, http.StatusBadRequest, "PASSWORD_BREACHED", "This password has appeared in known data breaches. Please choose a different password")
		case errors.Is(err, application.ErrRateLimited):
			retryAfter := 0
			if resp != nil {
				retryAfter = resp.RetryAfterSeconds
			}
			return rateLimitedResponse(c, retryAfter, "Too many registration attempts from this network. Please wait before trying again")
		default:
			h.log.Error("register failed", "error", err)
			return httpcallError(c, err)
		}
	}

	return httpcallSuccess(c, http.StatusCreated, resp, nil)
}

// VerifyEmailOTP handles POST /v1/auth/verify-email-otp
// @Summary      Verify email OTP
// @Description  Verifies the 6-digit OTP sent to the user's email address after registration.
// @Description  The account is fully active only after this step succeeds.
// @Description
// @Description  On success, a fresh `access_token` + `refresh_token` pair is returned (auto-login) —
// @Description  you already proved the password (at registration) and email ownership (this OTP),
// @Description  so a separate `/auth/login` call right after would be redundant.
// @Description
// @Description  **Attempt limits:**
// @Description  - Maximum **5 wrong attempts** per OTP code. After that, you must request a new OTP.
// @Description  - Each failed attempt returns the remaining attempt count in the `meta.attempts_remaining` field.
// @Description
// @Description  **OTP expiry:** OTP codes expire after 10 minutes. Request a new one via `/auth/resend-email-otp`.
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Param        request body dto.VerifyEmailOTPRequestDTO true "Email and OTP code"
// @Success      200 {object} httpresponse.Response{data=auth.VerifyEmailOTPResponse} "Email verified. access_token/refresh_token issued (auto-login)."
// @Failure      400 {object} httpresponse.Response "VALIDATION_ERROR | INVALID_OTP | OTP_EXPIRED"
// @Failure      429 {object} httpresponse.Response "OTP_MAX_ATTEMPTS — maximum wrong attempts exceeded. Request a new OTP."
// @Failure      500 {object} httpresponse.Response "INTERNAL_ERROR — unexpected server error"
// @Router       /v1/auth/verify-email-otp [post]
func (h *AuthHandler) VerifyEmailOTP(c echo.Context) error {
	var payload dto.VerifyEmailOTPRequestDTO
	if err := bindJSON(c, h.log, "verify email otp", &payload); err != nil {
		return err
	}

	if payload.Email == "" || payload.OTP == "" {
		h.log.Warn("verify email otp rejected", "reason", "validation_error")
		return httpresponse.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "Both email and otp fields are required")
	}

	ucReq := auth.VerifyEmailOTPRequest{
		Email: payload.Email,
		OTP:   payload.OTP,
	}

	resp, err := h.sessionUseCase.VerifyEmailOTP(c.Request().Context(), ucReq)
	if err != nil {
		attemptsRemaining := 0
		if resp != nil {
			attemptsRemaining = resp.AttemptsRemaining
		}
		if otpErr := otpVerifyError(c, err, attemptsRemaining, "/auth/resend-email-otp"); otpErr != nil {
			return otpErr
		}
		h.log.Error("verify email otp failed", "error", err)
		return httpcallError(c, err)
	}

	return httpcallSuccess(c, http.StatusOK, resp, nil)
}

// ResendEmailOTP handles POST /v1/auth/resend-email-otp
// @Summary      Resend email verification OTP
// @Description  Generates a new 6-digit OTP and sends it to the provided email address,
// @Description  invalidating any previously issued OTP for that address.
// @Description
// @Description  **Rate limiting:**
// @Description  - There is a **60-second cooldown** between resend requests.
// @Description  - Maximum **5 resend requests per 24 hours** per email address.
// @Description  - When rate limited, the response includes `meta.retry_after_seconds` indicating how long to wait.
// @Description
// @Description  For security reasons, this endpoint returns HTTP 200 regardless of whether the email
// @Description  address is registered, to prevent email enumeration attacks.
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Param        request body dto.ResendEmailOTPRequestDTO true "Email address to resend OTP to"
// @Success      200 {object} httpresponse.Response "OTP sent (or silently skipped if email is not registered)"
// @Failure      400 {object} httpresponse.Response "VALIDATION_ERROR — email field is missing or invalid"
// @Failure      429 {object} httpresponse.Response "RATE_LIMITED — cooldown active or daily limit reached. Check meta.retry_after_seconds."
// @Failure      500 {object} httpresponse.Response "INTERNAL_ERROR — unexpected server error"
// @Router       /v1/auth/resend-email-otp [post]
func (h *AuthHandler) ResendEmailOTP(c echo.Context) error {
	var payload dto.ResendEmailOTPRequestDTO
	if err := bindJSON(c, h.log, "resend email otp", &payload); err != nil {
		return err
	}

	if payload.Email == "" {
		h.log.Warn("resend email otp rejected", "reason", "validation_error")
		return httpresponse.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "email field is required")
	}

	ucReq := auth.ResendEmailOTPRequest{
		Email: payload.Email,
	}

	resp, err := h.accountUseCase.ResendEmailOTP(c.Request().Context(), ucReq)
	if err != nil {
		if errors.Is(err, application.ErrRateLimited) {
			retryAfter := 0
			if resp != nil {
				retryAfter = resp.RetryAfterSeconds
			}
			return rateLimitedResponse(c, retryAfter, "Too many OTP requests. Please wait before requesting again")
		}
		h.log.Error("resend email otp failed", "error", err)
		return httpcallError(c, err)
	}

	// Return 200 even when the email is not found to prevent email enumeration.
	return httpcallSuccess(c, http.StatusOK, map[string]string{"message": "If the email is registered and unverified, a new OTP has been sent"}, nil)
}

// Login handles POST /v1/auth/login
// @Summary      Login
// @Description  Authenticates a registered and verified member account.
// @Description  Returns a short-lived `access_token` (JWT) and a long-lived `refresh_token`.
// @Description
// @Description  **Account lockout:**
// @Description  After **10 consecutive failed attempts**, the account is temporarily locked for **15 minutes**.
// @Description  The locked response includes `meta.locked_until` (RFC 3339 timestamp) indicating when the
// @Description  lock expires. Attempting to log in during a lockout will continue returning HTTP 423.
// @Description
// @Description  The email must be verified before login is permitted.
// @Description
// @Description  **cf_turnstile_response** — required. The token returned by the Cloudflare Turnstile
// @Description  widget after the user completes the challenge.
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Param        request body dto.LoginRequestDTO true "Email and password credentials"
// @Success      200 {object} httpresponse.Response{data=auth.LoginResponse} "Login successful. Use access_token as Bearer token."
// @Failure      400 {object} httpresponse.Response "VALIDATION_ERROR | TURNSTILE_VERIFICATION_FAILED — email/password missing, or bot verification failed"
// @Failure      401 {object} httpresponse.Response "INVALID_CREDENTIALS — email or password is incorrect"
// @Failure      403 {object} httpresponse.Response "EMAIL_NOT_VERIFIED — account email is not yet verified"
// @Failure      423 {object} httpresponse.Response "ACCOUNT_LOCKED — account is temporarily locked. Check meta.locked_until."
// @Failure      429 {object} httpresponse.Response "RATE_LIMITED — too many login attempts from this IP (separate from ACCOUNT_LOCKED). Check meta.retry_after_seconds."
// @Failure      500 {object} httpresponse.Response "INTERNAL_ERROR — unexpected server error"
// @Router       /v1/auth/login [post]
func (h *AuthHandler) Login(c echo.Context) error {
	var payload dto.LoginRequestDTO
	if err := bindJSON(c, h.log, "login", &payload); err != nil {
		return err
	}
	if err := h.verifyTurnstile(c, payload.CFTurnstileResponse); err != nil {
		return err
	}

	if payload.Email == "" || payload.Password == "" {
		h.log.Warn("login rejected", "reason", "validation_error")
		return httpresponse.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "Both email and password fields are required")
	}

	ucReq := auth.LoginRequest{
		Email:     payload.Email,
		Password:  payload.Password,
		IPAddress: c.RealIP(),
	}

	resp, err := h.sessionUseCase.Login(c.Request().Context(), ucReq)
	if err != nil {
		switch {
		case errors.Is(err, application.ErrInvalidCredentials):
			return httpcallErrorCustom(c, http.StatusUnauthorized, "INVALID_CREDENTIALS", "Email or password is incorrect")
		case errors.Is(err, application.ErrAccountLocked):
			return httpcallErrorCustom(c, http.StatusLocked, "ACCOUNT_LOCKED", "Account is temporarily locked due to too many failed login attempts. Please try again later.")
		case errors.Is(err, application.ErrEmailNotVerified):
			return httpcallErrorCustom(c, http.StatusForbidden, "EMAIL_NOT_VERIFIED", "Please verify your email address before logging in. Check your inbox or request a new OTP via /auth/resend-email-otp")
		case errors.Is(err, application.ErrRateLimited):
			retryAfter := 0
			if resp != nil {
				retryAfter = resp.RetryAfterSeconds
			}
			return rateLimitedResponse(c, retryAfter, "Too many login attempts from this network. Please wait before trying again")
		default:
			h.log.Error("login failed", "error", err)
			return httpcallError(c, err)
		}
	}

	return httpcallSuccess(c, http.StatusOK, resp, nil)
}

// ChangePassword handles POST /v1/auth/change-password
// @Summary      Change password (authenticated)
// @Description  Changes the password of the currently logged-in account by verifying `old_password`.
// @Description  `retry_new_password` must exactly match `new_password`.
// @Description
// @Description  On success:
// @Description  - ALL existing sessions on every device are revoked (same as `/auth/reset-password`).
// @Description  - A fresh `access_token` + `refresh_token` pair is returned for THIS device so the caller isn't logged out.
// @Description
// @Description  The new password follows the same policy as registration: minimum 10 characters and
// @Description  checked against known breach databases.
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        request body dto.ChangePasswordRequestDTO true "Old password, new password, and confirmation"
// @Success      200 {object} httpresponse.Response{data=auth.ChangePasswordResponse} "Password changed. New session issued for this device."
// @Failure      400 {object} httpresponse.Response "VALIDATION_ERROR | PASSWORD_TOO_SHORT | PASSWORD_BREACHED | PASSWORD_CONFIRMATION_MISMATCH"
// @Failure      401 {object} httpresponse.Response "UNAUTHORIZED | INVALID_CREDENTIALS — access token invalid, or old_password is incorrect"
// @Failure      500 {object} httpresponse.Response "INTERNAL_ERROR — unexpected server error"
// @Router       /v1/auth/change-password [post]
func (h *AuthHandler) ChangePassword(c echo.Context) error {
	var payload dto.ChangePasswordRequestDTO
	if err := bindJSON(c, h.log, "change password", &payload); err != nil {
		return err
	}

	resp, err := h.sessionUseCase.ChangePassword(c.Request().Context(), auth.ChangePasswordRequest{
		UserID:           middleware.UserIDFromContext(c),
		OldPassword:      payload.OldPassword,
		NewPassword:      payload.NewPassword,
		RetryNewPassword: payload.RetryNewPassword,
	})
	if err != nil {
		switch {
		case errors.Is(err, application.ErrInvalidInput):
			return httpresponse.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", unwrapMessage(err))
		case errors.Is(err, application.ErrPasswordConfirmationMismatch):
			return httpcallErrorCustom(c, http.StatusBadRequest, "PASSWORD_CONFIRMATION_MISMATCH", "new_password and retry_new_password do not match")
		case errors.Is(err, application.ErrPasswordTooShort):
			return httpcallErrorCustom(c, http.StatusBadRequest, "PASSWORD_TOO_SHORT", "Password must be at least 10 characters long")
		case errors.Is(err, application.ErrPasswordBreached):
			return httpcallErrorCustom(c, http.StatusBadRequest, "PASSWORD_BREACHED", "This password has appeared in known data breaches. Please choose a different password")
		case errors.Is(err, application.ErrInvalidCredentials):
			return httpcallErrorCustom(c, http.StatusUnauthorized, "INVALID_CREDENTIALS", "Old password is incorrect")
		default:
			h.log.Error("change password failed", "error", err)
			return httpcallError(c, err)
		}
	}

	return httpcallSuccess(c, http.StatusOK, resp, nil)
}

// RefreshToken handles POST /v1/auth/refresh
// @Summary      Refresh session tokens
// @Description  Exchanges a valid `refresh_token` for a brand-new `access_token` + `refresh_token` pair.
// @Description  The presented refresh token is **rotated**: it is revoked immediately after the exchange
// @Description  and cannot be used a second time.
// @Description
// @Description  Returns `TOKEN_VERSION_MISMATCH` when the session was revoked by logout-all or a
// @Description  password reset — the client must redirect the user to login.
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Param        request body dto.RefreshTokenRequestDTO true "Refresh token"
// @Success      200 {object} httpresponse.Response{data=auth.RefreshTokenResponse} "New token pair issued"
// @Failure      400 {object} httpresponse.Response "VALIDATION_ERROR — refresh_token field is missing"
// @Failure      401 {object} httpresponse.Response "UNAUTHORIZED | TOKEN_VERSION_MISMATCH"
// @Failure      500 {object} httpresponse.Response "INTERNAL_ERROR — unexpected server error"
// @Router       /v1/auth/refresh [post]
func (h *AuthHandler) RefreshToken(c echo.Context) error {
	var payload dto.RefreshTokenRequestDTO
	if err := bindJSON(c, h.log, "refresh", &payload); err != nil {
		return err
	}

	resp, err := h.sessionUseCase.RefreshToken(c.Request().Context(), auth.RefreshTokenRequest{
		RefreshToken: payload.RefreshToken,
	})
	if err != nil {
		switch {
		case errors.Is(err, application.ErrInvalidInput):
			return httpresponse.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", unwrapMessage(err))
		case errors.Is(err, application.ErrTokenVersionMismatch):
			return httpcallErrorCustom(c, http.StatusUnauthorized, "TOKEN_VERSION_MISMATCH", "This session has been revoked. Please log in again")
		case errors.Is(err, application.ErrInvalidToken):
			return httpcallErrorCustom(c, http.StatusUnauthorized, "UNAUTHORIZED", "Invalid or expired refresh token")
		default:
			h.log.Error("refresh failed", "error", err)
			return httpcallError(c, err)
		}
	}

	return httpcallSuccess(c, http.StatusOK, resp, nil)
}

// Logout handles POST /v1/auth/logout
// @Summary      Logout (this session)
// @Description  Terminates the CURRENT session: the provided `refresh_token` is revoked and can no longer
// @Description  be exchanged for new access tokens. The current access token simply expires on its own
// @Description  (≤15 minutes). Other devices/sessions stay logged in — use `/auth/logout-all` to revoke everything.
// @Description
// @Description  Logout is idempotent: an already-expired refresh token still returns 200.
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        request body dto.LogoutRequestDTO true "Refresh token of the session being terminated"
// @Success      200 {object} httpresponse.Response "Session terminated"
// @Failure      400 {object} httpresponse.Response "VALIDATION_ERROR — refresh_token field is missing"
// @Failure      401 {object} httpresponse.Response "UNAUTHORIZED | TOKEN_VERSION_MISMATCH — access token invalid, or refresh token belongs to another account"
// @Failure      500 {object} httpresponse.Response "INTERNAL_ERROR — unexpected server error"
// @Router       /v1/auth/logout [post]
func (h *AuthHandler) Logout(c echo.Context) error {
	var payload dto.LogoutRequestDTO
	if err := bindJSON(c, h.log, "logout", &payload); err != nil {
		return err
	}

	err := h.sessionUseCase.Logout(c.Request().Context(), auth.LogoutRequest{
		UserID:       middleware.UserIDFromContext(c),
		RefreshToken: payload.RefreshToken,
	})
	if err != nil {
		switch {
		case errors.Is(err, application.ErrInvalidInput):
			return httpresponse.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", unwrapMessage(err))
		case errors.Is(err, application.ErrInvalidToken):
			return httpcallErrorCustom(c, http.StatusUnauthorized, "UNAUTHORIZED", "The refresh token does not belong to this account")
		default:
			h.log.Error("logout failed", "error", err)
			return httpcallError(c, err)
		}
	}

	return httpcallSuccess(c, http.StatusOK, map[string]string{"message": "Logged out successfully"}, nil)
}

// LogoutAll handles POST /v1/auth/logout-all
// @Summary      Logout from all devices
// @Description  Revokes EVERY session of the account by incrementing the internal token version.
// @Description  All previously issued access AND refresh tokens become invalid on their next use,
// @Description  including the one used to call this endpoint.
// @Tags         Auth
// @Produce      json
// @Security     BearerAuth
// @Success      200 {object} httpresponse.Response "All sessions revoked"
// @Failure      401 {object} httpresponse.Response "UNAUTHORIZED | TOKEN_VERSION_MISMATCH"
// @Failure      500 {object} httpresponse.Response "INTERNAL_ERROR — unexpected server error"
// @Router       /v1/auth/logout-all [post]
func (h *AuthHandler) LogoutAll(c echo.Context) error {
	if err := h.sessionUseCase.LogoutAll(c.Request().Context(), middleware.UserIDFromContext(c)); err != nil {
		h.log.Error("logout-all failed", "error", err)
		return httpcallError(c, err)
	}

	return httpcallSuccess(c, http.StatusOK, map[string]string{"message": "All sessions have been revoked. Please log in again"}, nil)
}

// ForgotPassword handles POST /v1/auth/forgot-password
// @Summary      Request a password reset OTP (step 1 of 3)
// @Description  Sends a 6-digit password reset OTP to the given email address.
// @Description
// @Description  **Anti-enumeration:** this endpoint ALWAYS returns the same generic 200 response,
// @Description  whether or not the email is registered. Do not use it to probe for accounts.
// @Description
// @Description  **Rate limiting:** 60-second cooldown + max 5 requests per 24 hours per email
// @Description  (separate budget from `/auth/resend-email-otp`). When throttled, the response
// @Description  includes `meta.retry_after_seconds`.
// @Description
// @Description  Flow: `/forgot-password` → `/verify-reset-otp` → `/reset-password`.
// @Description
// @Description  **cf_turnstile_response** — required. The token returned by the Cloudflare Turnstile
// @Description  widget after the user completes the challenge.
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Param        request body dto.ForgotPasswordRequestDTO true "Account email"
// @Success      200 {object} httpresponse.Response "Generic response — OTP sent if the email is registered"
// @Failure      400 {object} httpresponse.Response "VALIDATION_ERROR | TURNSTILE_VERIFICATION_FAILED — email missing, or bot verification failed"
// @Failure      429 {object} httpresponse.Response "RATE_LIMITED — check meta.retry_after_seconds"
// @Failure      500 {object} httpresponse.Response "INTERNAL_ERROR — unexpected server error"
// @Router       /v1/auth/forgot-password [post]
func (h *AuthHandler) ForgotPassword(c echo.Context) error {
	var payload dto.ForgotPasswordRequestDTO
	if err := bindJSON(c, h.log, "forgot password", &payload); err != nil {
		return err
	}
	if err := h.verifyTurnstile(c, payload.CFTurnstileResponse); err != nil {
		return err
	}

	resp, err := h.accountUseCase.ForgotPassword(c.Request().Context(), auth.ForgotPasswordRequest{
		Email: payload.Email,
	})
	if err != nil {
		switch {
		case errors.Is(err, application.ErrInvalidInput):
			return httpresponse.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", unwrapMessage(err))
		case errors.Is(err, application.ErrRateLimited):
			retryAfter := 0
			if resp != nil {
				retryAfter = resp.RetryAfterSeconds
			}
			return rateLimitedResponse(c, retryAfter, "Too many reset requests. Please wait before requesting again")
		default:
			h.log.Error("forgot password failed", "error", err)
			return httpcallError(c, err)
		}
	}

	// ALWAYS the same generic response — never reveal whether the email exists.
	return httpcallSuccess(c, http.StatusOK, map[string]string{"message": "If the email is registered, a reset code has been sent"}, nil)
}

// VerifyResetOTP handles POST /v1/auth/verify-reset-otp
// @Summary      Verify password reset OTP (step 2 of 3)
// @Description  Exchanges a valid reset OTP for a short-lived (~15 minutes) single-use `reset_token`.
// @Description  Pass that token to `/auth/reset-password` to actually change the password.
// @Description
// @Description  **Attempt limits:** maximum 5 wrong attempts per OTP; then a new code must be requested
// @Description  via `/auth/forgot-password`. Failed attempts return `meta.attempts_remaining`.
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Param        request body dto.VerifyResetOTPRequestDTO true "Email and reset OTP code"
// @Success      200 {object} httpresponse.Response{data=auth.VerifyResetOTPResponse} "reset_token issued (single-use, ~15 min)"
// @Failure      400 {object} httpresponse.Response "VALIDATION_ERROR | INVALID_OTP | OTP_EXPIRED"
// @Failure      429 {object} httpresponse.Response "OTP_MAX_ATTEMPTS — request a new OTP via /auth/forgot-password"
// @Failure      500 {object} httpresponse.Response "INTERNAL_ERROR — unexpected server error"
// @Router       /v1/auth/verify-reset-otp [post]
func (h *AuthHandler) VerifyResetOTP(c echo.Context) error {
	var payload dto.VerifyResetOTPRequestDTO
	if err := bindJSON(c, h.log, "verify reset otp", &payload); err != nil {
		return err
	}

	resp, err := h.sessionUseCase.VerifyResetOTP(c.Request().Context(), auth.VerifyResetOTPRequest{
		Email: payload.Email,
		OTP:   payload.OTP,
	})
	if err != nil {
		if errors.Is(err, application.ErrInvalidInput) {
			return httpresponse.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", unwrapMessage(err))
		}

		attemptsRemaining := 0
		if resp != nil {
			attemptsRemaining = resp.AttemptsRemaining
		}
		if otpErr := otpVerifyError(c, err, attemptsRemaining, "/auth/forgot-password"); otpErr != nil {
			return otpErr
		}
		h.log.Error("verify reset otp failed", "error", err)
		return httpcallError(c, err)
	}

	return httpcallSuccess(c, http.StatusOK, resp, nil)
}

// ResetPassword handles POST /v1/auth/reset-password
// @Summary      Reset password (step 3 of 3)
// @Description  Changes the account password using the single-use `reset_token` from `/auth/verify-reset-otp`.
// @Description
// @Description  On success:
// @Description  - The `reset_token` is consumed permanently (replaying it returns 401).
// @Description  - ALL existing sessions on every device are revoked.
// @Description  - A fresh `access_token` + `refresh_token` pair is returned (auto-login).
// @Description
// @Description  The new password follows the same policy as registration: minimum 10 characters and
// @Description  checked against known breach databases.
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Param        request body dto.ResetPasswordRequestDTO true "Reset token and new password"
// @Success      200 {object} httpresponse.Response{data=auth.ResetPasswordResponse} "Password changed. New session issued."
// @Failure      400 {object} httpresponse.Response "VALIDATION_ERROR | PASSWORD_TOO_SHORT | PASSWORD_BREACHED"
// @Failure      401 {object} httpresponse.Response "UNAUTHORIZED — reset_token invalid, expired, or already used"
// @Failure      500 {object} httpresponse.Response "INTERNAL_ERROR — unexpected server error"
// @Router       /v1/auth/reset-password [post]
func (h *AuthHandler) ResetPassword(c echo.Context) error {
	var payload dto.ResetPasswordRequestDTO
	if err := bindJSON(c, h.log, "reset password", &payload); err != nil {
		return err
	}

	resp, err := h.sessionUseCase.ResetPassword(c.Request().Context(), auth.ResetPasswordRequest{
		ResetToken:  payload.ResetToken,
		NewPassword: payload.NewPassword,
	})
	if err != nil {
		switch {
		case errors.Is(err, application.ErrInvalidInput):
			return httpresponse.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", unwrapMessage(err))
		case errors.Is(err, application.ErrPasswordTooShort):
			return httpcallErrorCustom(c, http.StatusBadRequest, "PASSWORD_TOO_SHORT", "Password must be at least 10 characters long")
		case errors.Is(err, application.ErrPasswordBreached):
			return httpcallErrorCustom(c, http.StatusBadRequest, "PASSWORD_BREACHED", "This password has appeared in known data breaches. Please choose a different password")
		case errors.Is(err, application.ErrInvalidToken):
			return httpcallErrorCustom(c, http.StatusUnauthorized, "UNAUTHORIZED", "The reset token is invalid, expired, or has already been used")
		default:
			h.log.Error("reset password failed", "error", err)
			return httpcallError(c, err)
		}
	}

	return httpcallSuccess(c, http.StatusOK, resp, nil)
}
