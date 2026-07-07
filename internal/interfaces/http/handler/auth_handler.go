package handler

import (
	"errors"
	"net/http"

	"github.com/aprxty3/your_persona_controller.git/internal/application/auth"
	"github.com/aprxty3/your_persona_controller.git/pkg/httpresponse"
	"github.com/labstack/echo/v4"
)

// AuthHandler handles HTTP requests for authentication and account onboarding.
type AuthHandler struct {
	createGuestSessionUseCase *auth.CreateGuestSessionUseCase
	registerUseCase           *auth.RegisterUseCase
	verifyEmailOTPUseCase     *auth.VerifyEmailOTPUseCase
	resendEmailOTPUseCase     *auth.ResendEmailOTPUseCase
	loginUseCase              *auth.LoginUseCase
}

// NewAuthHandler is the constructor for Dependency Injection.
func NewAuthHandler(
	createGuestSessionUseCase *auth.CreateGuestSessionUseCase,
	registerUseCase *auth.RegisterUseCase,
	verifyEmailOTPUseCase *auth.VerifyEmailOTPUseCase,
	resendEmailOTPUseCase *auth.ResendEmailOTPUseCase,
	loginUseCase *auth.LoginUseCase,
) *AuthHandler {
	return &AuthHandler{
		createGuestSessionUseCase: createGuestSessionUseCase,
		registerUseCase:           registerUseCase,
		verifyEmailOTPUseCase:     verifyEmailOTPUseCase,
		resendEmailOTPUseCase:     resendEmailOTPUseCase,
		loginUseCase:              loginUseCase,
	}
}

// CreateGuestSessionRequestDTO is the input payload for guest session creation.
type CreateGuestSessionRequestDTO struct {
	DisplayName string `json:"display_name" validate:"required"`
	Age         int    `json:"age" validate:"required"`
	Status      string `json:"status" validate:"required"` // e.g., bekerja, mahasiswa
	Locale      string `json:"locale" validate:"required"` // e.g., en, id
}

// CreateGuestSession handles POST /v1/guest-session
// @Summary Create a guest session
// @Description Creates a guest session from onboarding data, sets session_id httpOnly cookie
// @Tags Auth
// @Accept json
// @Produce json
// @Param request body CreateGuestSessionRequestDTO true "Guest Session Onboarding Data"
// @Success 201 {object} httpresponse.Response{data=auth.CreateGuestSessionResponse}
// @Failure 400 {object} httpresponse.Response
// @Failure 500 {object} httpresponse.Response
// @Router /v1/guest-session [post]
func (h *AuthHandler) CreateGuestSession(c echo.Context) error {
	var payload CreateGuestSessionRequestDTO
	if err := c.Bind(&payload); err != nil {
		return httpresponse.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid request body")
	}

	if payload.DisplayName == "" || payload.Age <= 0 || payload.Status == "" || payload.Locale == "" {
		return httpresponse.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "Required fields are missing or invalid")
	}

	ipAddress := c.RealIP()

	ucReq := auth.CreateGuestSessionRequest{
		DisplayName: payload.DisplayName,
		Age:         payload.Age,
		Status:      payload.Status,
		Locale:      payload.Locale,
		IPAddress:   ipAddress,
	}

	resp, err := h.createGuestSessionUseCase.Execute(c.Request().Context(), ucReq)
	if err != nil {
		return httpcallError(c, err)
	}

	// Set HttpOnly Cookie
	cookie := &http.Cookie{
		Name:     "session_id",
		Value:    resp.SessionID,
		Expires:  resp.ExpiresAt,
		Path:     "/",
		HttpOnly: true,
		Secure:   true, // Mandatory for production and Lax/Strict cookies in modern browsers
		SameSite: http.SameSiteLaxMode,
	}
	c.SetCookie(cookie)

	return httpresponse.Success(c, http.StatusCreated, resp, nil)
}

// RegisterRequestDTO holds registration info.
type RegisterRequestDTO struct {
	Email           string  `json:"email" validate:"required,email"`
	Password        string  `json:"password" validate:"required,min=10"`
	PreferredLocale string  `json:"preferred_locale" validate:"required"`
	ReferralCode    *string `json:"referral_code,omitempty"`
}

// Register handles POST /v1/auth/register
// @Summary Register a new user
// @Description Registers a new member account. Claims any valid guest session in cookies.
// @Tags Auth
// @Accept json
// @Produce json
// @Param request body RegisterRequestDTO true "Registration Data"
// @Success 201 {object} httpresponse.Response{data=auth.RegisterResponse}
// @Failure 400 {object} httpresponse.Response
// @Failure 500 {object} httpresponse.Response
// @Router /v1/auth/register [post]
func (h *AuthHandler) Register(c echo.Context) error {
	var payload RegisterRequestDTO
	if err := c.Bind(&payload); err != nil {
		return httpresponse.Error(c, http.StatusBadRequest, "BIND_ERROR", err.Error())
	}

	if payload.Email == "" || payload.Password == "" || payload.PreferredLocale == "" {
		return httpresponse.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "Email, password, and preferred locale are required")
	}

	// Read guest session ID from cookie if present
	var guestSessionID *string
	cookie, err := c.Cookie("session_id")
	if err == nil && cookie != nil {
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
		if errors.Is(err, auth.ErrEmailAlreadyRegistered) {
			return httpcallErrorCustom(c, http.StatusBadRequest, "EMAIL_ALREADY_REGISTERED", "Email is already registered")
		}
		if errors.Is(err, auth.ErrPasswordTooShort) {
			return httpcallErrorCustom(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		}
		return httpcallError(c, err)
	}

	return httpcallSuccess(c, http.StatusCreated, resp, nil)
}

// VerifyEmailOTPRequestDTO holds OTP verification parameters.
type VerifyEmailOTPRequestDTO struct {
	Email string `json:"email" validate:"required,email"`
	OTP   string `json:"otp" validate:"required"`
}

// VerifyEmailOTP handles POST /v1/auth/verify-email-otp
// @Summary Verify email verification OTP
// @Description Verifies registration OTP and marks email as verified
// @Tags Auth
// @Accept json
// @Produce json
// @Param request body VerifyEmailOTPRequestDTO true "Email & OTP"
// @Success 200 {object} httpresponse.Response
// @Failure 400 {object} httpresponse.Response
// @Failure 429 {object} httpresponse.Response
// @Router /v1/auth/verify-email-otp [post]
func (h *AuthHandler) VerifyEmailOTP(c echo.Context) error {
	var payload VerifyEmailOTPRequestDTO
	if err := c.Bind(&payload); err != nil {
		return httpcallError(c, err)
	}

	if payload.Email == "" || payload.OTP == "" {
		return httpresponse.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "Email and OTP are required")
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

		if errors.Is(err, auth.ErrInvalidOTP) {
			return c.JSON(http.StatusBadRequest, httpresponse.Response{
				Success: false,
				Error: &httpresponse.ErrorDetail{
					Code:    "INVALID_OTP",
					Message: "Kode OTP tidak valid",
				},
				Meta: meta,
			})
		}
		if errors.Is(err, auth.ErrOTPExpired) {
			return c.JSON(http.StatusBadRequest, httpresponse.Response{
				Success: false,
				Error: &httpresponse.ErrorDetail{
					Code:    "OTP_EXPIRED",
					Message: "OTP sudah kedaluwarsa",
				},
				Meta: meta,
			})
		}
		if errors.Is(err, auth.ErrOTPMaxAttempts) {
			return c.JSON(http.StatusTooManyRequests, httpresponse.Response{
				Success: false,
				Error: &httpresponse.ErrorDetail{
					Code:    "OTP_MAX_ATTEMPTS",
					Message: "Batas percobaan verifikasi tercapai. Minta OTP baru",
				},
				Meta: meta,
			})
		}
		return httpcallError(c, err)
	}

	return httpcallSuccess(c, http.StatusOK, map[string]string{"message": "Email verified successfully"}, nil)
}

// ResendEmailOTPRequestDTO holds input for resending verification OTP.
type ResendEmailOTPRequestDTO struct {
	Email string `json:"email" validate:"required,email"`
}

// ResendEmailOTP handles POST /v1/auth/resend-email-otp
// @Summary Resend verification OTP
// @Description Generates and sends a new registration OTP code, invalidating previous ones
// @Tags Auth
// @Accept json
// @Produce json
// @Param request body ResendEmailOTPRequestDTO true "Email Address"
// @Success 200 {object} httpresponse.Response
// @Failure 429 {object} httpresponse.Response
// @Router /v1/auth/resend-email-otp [post]
func (h *AuthHandler) ResendEmailOTP(c echo.Context) error {
	var payload ResendEmailOTPRequestDTO
	if err := c.Bind(&payload); err != nil {
		return httpcallError(c, err)
	}

	if payload.Email == "" {
		return httpresponse.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "Email is required")
	}

	ucReq := auth.ResendEmailOTPRequest{
		Email: payload.Email,
	}

	resp, err := h.resendEmailOTPUseCase.Execute(c.Request().Context(), ucReq)
	if err != nil {
		if errors.Is(err, auth.ErrRateLimited) {
			meta := map[string]interface{}{}
			if resp != nil {
				meta["retry_after_seconds"] = resp.RetryAfterSeconds
			}
			return c.JSON(http.StatusTooManyRequests, httpresponse.Response{
				Success: false,
				Error: &httpresponse.ErrorDetail{
					Code:    "RATE_LIMITED",
					Message: "Terlalu banyak permintaan OTP. Silakan tunggu cooldown.",
				},
				Meta: meta,
			})
		}
		return httpcallError(c, err)
	}

	return httpcallSuccess(c, http.StatusOK, map[string]string{"message": "Jika email terdaftar, OTP telah dikirim kembali"}, nil)
}

// LoginRequestDTO holds login credentials.
type LoginRequestDTO struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required"`
}

// Login handles POST /v1/auth/login
// @Summary User login
// @Description Authenticates member and returns JWT tokens
// @Tags Auth
// @Accept json
// @Produce json
// @Param request body LoginRequestDTO true "Credentials"
// @Success 200 {object} httpresponse.Response{data=auth.LoginResponse}
// @Failure 401 {object} httpresponse.Response
// @Failure 423 {object} httpresponse.Response
// @Router /v1/auth/login [post]
func (h *AuthHandler) Login(c echo.Context) error {
	var payload LoginRequestDTO
	if err := c.Bind(&payload); err != nil {
		return httpcallError(c, err)
	}

	if payload.Email == "" || payload.Password == "" {
		return httpresponse.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "Email and password are required")
	}

	ucReq := auth.LoginRequest{
		Email:    payload.Email,
		Password: payload.Password,
	}

	resp, err := h.loginUseCase.Execute(c.Request().Context(), ucReq)
	if err != nil {
		if errors.Is(err, auth.ErrInvalidCredentials) {
			return httpcallErrorCustom(c, http.StatusUnauthorized, "INVALID_CREDENTIALS", "Email atau password salah")
		}
		if errors.Is(err, auth.ErrAccountLocked) {
			return httpcallErrorCustom(c, http.StatusLocked, "ACCOUNT_LOCKED", "Akun terkunci selama 15 menit karena terlalu banyak percobaan gagal")
		}
		return httpcallError(c, err)
	}

	return httpcallSuccess(c, http.StatusOK, resp, nil)
}

// Helpers to satisfy Swaggo DTO representations and unify error handling style.

func httpcallError(c echo.Context, err error) error {
	return httpresponse.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
}

func httpcallErrorCustom(c echo.Context, code int, errCode string, msg string) error {
	return httpcallErrorWithDetail(c, code, errCode, msg)
}

func httpcallErrorWithDetail(c echo.Context, code int, errCode string, msg string) error {
	return httpresponse.Error(c, code, errCode, msg)
}

func httpcallSuccess(c echo.Context, code int, data interface{}, meta interface{}) error {
	return httpresponse.Success(c, code, data, meta)
}
