package handler

import (
	"errors"
	"net/http"

	"github.com/aprxty3/your_persona_controller.git/internal/application"
	"github.com/aprxty3/your_persona_controller.git/internal/application/auth"
	"github.com/aprxty3/your_persona_controller.git/internal/interfaces/http/dto"
	"github.com/aprxty3/your_persona_controller.git/pkg/httpresponse"
	"github.com/aprxty3/your_persona_controller.git/pkg/logger"
	"github.com/labstack/echo/v4"
)

// AuthHandler handles HTTP requests for authentication and account onboarding.
type AuthHandler struct {
	createGuestSessionUseCase *auth.CreateGuestSessionUseCase
	registerUseCase           *auth.RegisterUseCase
	verifyEmailOTPUseCase     *auth.VerifyEmailOTPUseCase
	resendEmailOTPUseCase     *auth.ResendEmailOTPUseCase
	loginUseCase              *auth.LoginUseCase
	log                       logger.Logger
}

// NewAuthHandler is the constructor for Dependency Injection.
func NewAuthHandler(
	createGuestSessionUseCase *auth.CreateGuestSessionUseCase,
	registerUseCase *auth.RegisterUseCase,
	verifyEmailOTPUseCase *auth.VerifyEmailOTPUseCase,
	resendEmailOTPUseCase *auth.ResendEmailOTPUseCase,
	loginUseCase *auth.LoginUseCase,
	log logger.Logger,
) *AuthHandler {
	return &AuthHandler{
		createGuestSessionUseCase: createGuestSessionUseCase,
		registerUseCase:           registerUseCase,
		verifyEmailOTPUseCase:     verifyEmailOTPUseCase,
		resendEmailOTPUseCase:     resendEmailOTPUseCase,
		loginUseCase:              loginUseCase,
		log:                       log.With("handler", "auth"),
	}
}

func unwrapMessage(err error) string {
	msg := err.Error()
	for i := 0; i < len(msg)-2; i++ {
		if msg[i] == ':' && msg[i+1] == ' ' {
			return msg[i+2:]
		}
	}
	return msg
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
	if err := c.Bind(&payload); err != nil {
		h.log.Warn("create guest session rejected", "reason", "bind_error", "error", err)
		return httpresponse.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request body format")
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
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
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
// @Description  If you do not have a referral code, omit the field entirely or set it to `null`.
// @Description  Do NOT send an empty string `""` — that will be treated as a validation error.
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Param        request body dto.RegisterRequestDTO true "Registration Data"
// @Success      201 {object} httpresponse.Response{data=auth.RegisterResponse} "Account created. OTP sent to email. Verify via /auth/verify-email-otp."
// @Failure      400 {object} httpresponse.Response "VALIDATION_ERROR | EMAIL_ALREADY_REGISTERED | PASSWORD_TOO_SHORT | INVALID_REFERRAL_CODE"
// @Failure      409 {object} httpresponse.Response "EMAIL_ALREADY_REGISTERED — email is already in use"
// @Failure      500 {object} httpresponse.Response "INTERNAL_ERROR — unexpected server error"
// @Router       /v1/auth/register [post]
func (h *AuthHandler) Register(c echo.Context) error {
	var payload dto.RegisterRequestDTO
	if err := c.Bind(&payload); err != nil {
		h.log.Warn("register rejected", "reason", "bind_error", "error", err)
		return httpresponse.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request body format")
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
	}

	resp, err := h.registerUseCase.Execute(c.Request().Context(), ucReq)
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
// @Description  **Attempt limits:**
// @Description  - Maximum **5 wrong attempts** per OTP code. After that, you must request a new OTP.
// @Description  - Each failed attempt returns the remaining attempt count in the `meta.attempts_remaining` field.
// @Description
// @Description  **OTP expiry:** OTP codes expire after 10 minutes. Request a new one via `/auth/resend-email-otp`.
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Param        request body dto.VerifyEmailOTPRequestDTO true "Email and OTP code"
// @Success      200 {object} httpresponse.Response "Email verified successfully"
// @Failure      400 {object} httpresponse.Response "VALIDATION_ERROR | INVALID_OTP | OTP_EXPIRED"
// @Failure      429 {object} httpresponse.Response "OTP_MAX_ATTEMPTS — maximum wrong attempts exceeded. Request a new OTP."
// @Failure      500 {object} httpresponse.Response "INTERNAL_ERROR — unexpected server error"
// @Router       /v1/auth/verify-email-otp [post]
func (h *AuthHandler) VerifyEmailOTP(c echo.Context) error {
	var payload dto.VerifyEmailOTPRequestDTO
	if err := c.Bind(&payload); err != nil {
		h.log.Warn("verify email otp rejected", "reason", "bind_error", "error", err)
		return httpresponse.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request body format")
	}

	if payload.Email == "" || payload.OTP == "" {
		h.log.Warn("verify email otp rejected", "reason", "validation_error")
		return httpresponse.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "Both email and otp fields are required")
	}

	ucReq := auth.VerifyEmailOTPRequest{
		Email: payload.Email,
		OTP:   payload.OTP,
	}

	resp, err := h.verifyEmailOTPUseCase.Execute(c.Request().Context(), ucReq)
	if err != nil {
		meta := map[string]interface{}{}
		if resp != nil {
			meta["attempts_remaining"] = resp.AttemptsRemaining
		}

		switch {
		case errors.Is(err, application.ErrInvalidOTP):
			return c.JSON(http.StatusBadRequest, httpresponse.Response{
				Success: false,
				Error:   &httpresponse.ErrorDetail{Code: "INVALID_OTP", Message: "The OTP code is incorrect"},
				Meta:    meta,
			})
		case errors.Is(err, application.ErrOTPExpired):
			return c.JSON(http.StatusBadRequest, httpresponse.Response{
				Success: false,
				Error:   &httpresponse.ErrorDetail{Code: "OTP_EXPIRED", Message: "The OTP code has expired. Please request a new one via /auth/resend-email-otp"},
				Meta:    meta,
			})
		case errors.Is(err, application.ErrOTPMaxAttempts):
			return c.JSON(http.StatusTooManyRequests, httpresponse.Response{
				Success: false,
				Error:   &httpresponse.ErrorDetail{Code: "OTP_MAX_ATTEMPTS", Message: "Maximum verification attempts exceeded. Please request a new OTP via /auth/resend-email-otp"},
				Meta:    meta,
			})
		default:
			h.log.Error("verify email otp failed", "error", err)
			return httpcallError(c, err)
		}
	}

	return httpcallSuccess(c, http.StatusOK, map[string]string{"message": "Email verified successfully"}, nil)
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
	if err := c.Bind(&payload); err != nil {
		h.log.Warn("resend email otp rejected", "reason", "bind_error", "error", err)
		return httpresponse.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request body format")
	}

	if payload.Email == "" {
		h.log.Warn("resend email otp rejected", "reason", "validation_error")
		return httpresponse.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "email field is required")
	}

	ucReq := auth.ResendEmailOTPRequest{
		Email: payload.Email,
	}

	resp, err := h.resendEmailOTPUseCase.Execute(c.Request().Context(), ucReq)
	if err != nil {
		if errors.Is(err, application.ErrRateLimited) {
			meta := map[string]interface{}{}
			if resp != nil {
				meta["retry_after_seconds"] = resp.RetryAfterSeconds
			}
			return c.JSON(http.StatusTooManyRequests, httpresponse.Response{
				Success: false,
				Error:   &httpresponse.ErrorDetail{Code: "RATE_LIMITED", Message: "Too many OTP requests. Please wait before requesting again"},
				Meta:    meta,
			})
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
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Param        request body dto.LoginRequestDTO true "Email and password credentials"
// @Success      200 {object} httpresponse.Response{data=auth.LoginResponse} "Login successful. Use access_token as Bearer token."
// @Failure      400 {object} httpresponse.Response "VALIDATION_ERROR — email or password field is missing"
// @Failure      401 {object} httpresponse.Response "INVALID_CREDENTIALS — email or password is incorrect"
// @Failure      403 {object} httpresponse.Response "EMAIL_NOT_VERIFIED — account email is not yet verified"
// @Failure      423 {object} httpresponse.Response "ACCOUNT_LOCKED — account is temporarily locked. Check meta.locked_until."
// @Failure      500 {object} httpresponse.Response "INTERNAL_ERROR — unexpected server error"
// @Router       /v1/auth/login [post]
func (h *AuthHandler) Login(c echo.Context) error {
	var payload dto.LoginRequestDTO
	if err := c.Bind(&payload); err != nil {
		h.log.Warn("login rejected", "reason", "bind_error", "error", err)
		return httpresponse.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request body format")
	}

	if payload.Email == "" || payload.Password == "" {
		h.log.Warn("login rejected", "reason", "validation_error")
		return httpresponse.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "Both email and password fields are required")
	}

	ucReq := auth.LoginRequest{
		Email:    payload.Email,
		Password: payload.Password,
	}

	resp, err := h.loginUseCase.Execute(c.Request().Context(), ucReq)
	if err != nil {
		switch {
		case errors.Is(err, application.ErrInvalidCredentials):
			return httpcallErrorCustom(c, http.StatusUnauthorized, "INVALID_CREDENTIALS", "Email or password is incorrect")
		case errors.Is(err, application.ErrAccountLocked):
			return httpcallErrorCustom(c, http.StatusLocked, "ACCOUNT_LOCKED", "Account is temporarily locked due to too many failed login attempts. Please try again later.")
		case errors.Is(err, application.ErrEmailNotVerified):
			return httpcallErrorCustom(c, http.StatusForbidden, "EMAIL_NOT_VERIFIED", "Please verify your email address before logging in. Check your inbox or request a new OTP via /auth/resend-email-otp")
		default:
			h.log.Error("login failed", "error", err)
			return httpcallError(c, err)
		}
	}

	return httpcallSuccess(c, http.StatusOK, resp, nil)
}
