package dto

// GuestStatus is a guest's self-reported occupation status at onboarding.
type GuestStatus = string

// The accepted GuestStatus values.
const (
	GuestStatusStudent    GuestStatus = "student"
	GuestStatusWorker     GuestStatus = "worker"
	GuestStatusFreelancer GuestStatus = "freelancer"
	GuestStatusUnemployed GuestStatus = "unemployed"
	GuestStatusOther      GuestStatus = "other"
)

// Locale is a supported UI/content language code.
type Locale = string

// The accepted Locale values.
const (
	LocaleEnglish    Locale = "en"
	LocaleIndonesian Locale = "id"
)

// CreateGuestSessionRequestDTO is the request body for POST /v1/guest-session.
type CreateGuestSessionRequestDTO struct {
	DisplayName string      `json:"display_name" validate:"required"`
	Age         int         `json:"age" validate:"required,min=13"`
	Status      GuestStatus `json:"status" validate:"required,oneof=student worker freelancer unemployed other"`
	Locale      Locale      `json:"locale" validate:"required,oneof=en id"`
}

// RegisterRequestDTO is the request body for POST /v1/auth/register.
type RegisterRequestDTO struct {
	Email               string  `json:"email" validate:"required,email"`
	Password            string  `json:"password" validate:"required,min=10"`
	PreferredLocale     Locale  `json:"preferred_locale" validate:"required,oneof=en id"`
	ReferralCode        *string `json:"referral_code,omitempty"`
	CFTurnstileResponse string  `json:"cf_turnstile_response" validate:"required"`
}

// VerifyEmailOTPRequestDTO is the request body for POST /v1/auth/verify-email-otp.
type VerifyEmailOTPRequestDTO struct {
	Email string `json:"email" validate:"required,email"`
	OTP   string `json:"otp" validate:"required,len=6"`
}

// ResendEmailOTPRequestDTO is the request body for POST /v1/auth/resend-email-otp.
type ResendEmailOTPRequestDTO struct {
	Email string `json:"email" validate:"required,email"`
}

// LoginRequestDTO is the request body for POST /v1/auth/login.
type LoginRequestDTO struct {
	Email               string `json:"email" validate:"required,email"`
	Password            string `json:"password" validate:"required"`
	CFTurnstileResponse string `json:"cf_turnstile_response" validate:"required"`
}

// RefreshTokenRequestDTO is the request body for POST /v1/auth/refresh.
type RefreshTokenRequestDTO struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

// LogoutRequestDTO is the request body for POST /v1/auth/logout.
type LogoutRequestDTO struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

// ForgotPasswordRequestDTO is the request body for POST /v1/auth/forgot-password.
type ForgotPasswordRequestDTO struct {
	Email               string `json:"email" validate:"required,email"`
	CFTurnstileResponse string `json:"cf_turnstile_response" validate:"required"`
}

// VerifyResetOTPRequestDTO is the request body for POST /v1/auth/verify-reset-otp.
type VerifyResetOTPRequestDTO struct {
	Email string `json:"email" validate:"required,email"`
	OTP   string `json:"otp" validate:"required,len=6"`
}

// ResetPasswordRequestDTO is the request body for POST /v1/auth/reset-password.
type ResetPasswordRequestDTO struct {
	ResetToken  string `json:"reset_token" validate:"required"`
	NewPassword string `json:"new_password" validate:"required,min=10"`
}

// ChangePasswordRequestDTO is the request body for POST /v1/auth/change-password.
type ChangePasswordRequestDTO struct {
	OldPassword      string `json:"old_password" validate:"required"`
	NewPassword      string `json:"new_password" validate:"required,min=10"`
	RetryNewPassword string `json:"retry_new_password" validate:"required"`
}

// UpdateProfileRequestDTO is a partial update — omit any field you don't want to change.
type UpdateProfileRequestDTO struct {
	DisplayName     *string      `json:"display_name,omitempty" validate:"omitempty"`
	Age             *int         `json:"age,omitempty" validate:"omitempty,min=13"`
	Status          *GuestStatus `json:"status,omitempty" validate:"omitempty,oneof=student worker freelancer unemployed other"`
	PreferredLocale *Locale      `json:"preferred_locale,omitempty" validate:"omitempty,oneof=en id"`
}
