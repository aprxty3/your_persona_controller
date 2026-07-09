package dto

type GuestStatus = string

const (
	GuestStatusStudent    GuestStatus = "student"
	GuestStatusWorker     GuestStatus = "worker"
	GuestStatusFreelancer GuestStatus = "freelancer"
	GuestStatusUnemployed GuestStatus = "unemployed"
	GuestStatusOther      GuestStatus = "other"
)

type Locale = string

const (
	LocaleEnglish    Locale = "en"
	LocaleIndonesian Locale = "id"
)

type CreateGuestSessionRequestDTO struct {
	DisplayName string      `json:"display_name" validate:"required"`
	Age         int         `json:"age" validate:"required,min=13"`
	Status      GuestStatus `json:"status" validate:"required,oneof=student worker freelancer unemployed other"`
	Locale      Locale      `json:"locale" validate:"required,oneof=en id"`
}
type RegisterRequestDTO struct {
	Email           string  `json:"email" validate:"required,email"`
	Password        string  `json:"password" validate:"required,min=10"`
	PreferredLocale Locale  `json:"preferred_locale" validate:"required,oneof=en id"`
	ReferralCode    *string `json:"referral_code,omitempty"`
}

type VerifyEmailOTPRequestDTO struct {
	Email string `json:"email" validate:"required,email"`
	OTP   string `json:"otp" validate:"required,len=6"`
}

type ResendEmailOTPRequestDTO struct {
	Email string `json:"email" validate:"required,email"`
}
type LoginRequestDTO struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required"`
}
