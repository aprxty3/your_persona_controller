package dto

type CreateGuestSessionRequestDTO struct {
	DisplayName string `json:"display_name" validate:"required"`
	Age         int    `json:"age" validate:"required"`
	Status      string `json:"status" validate:"required"` // e.g., bekerja, mahasiswa
	Locale      string `json:"locale" validate:"required"` // e.g., en, id
}

type RegisterRequestDTO struct {
	Email           string  `json:"email" validate:"required,email"`
	Password        string  `json:"password" validate:"required,min=10"`
	PreferredLocale string  `json:"preferred_locale" validate:"required"`
	ReferralCode    *string `json:"referral_code,omitempty"`
}

type VerifyEmailOTPRequestDTO struct {
	Email string `json:"email" validate:"required,email"`
	OTP   string `json:"otp" validate:"required"`
}

type ResendEmailOTPRequestDTO struct {
	Email string `json:"email" validate:"required,email"`
}

type LoginRequestDTO struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required"`
}
