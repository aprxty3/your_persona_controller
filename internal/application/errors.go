package application

import "errors"

// Input validation errors
var (
	ErrInvalidInput = errors.New("INVALID_INPUT")
)

// Auth / session errors
var (
	ErrInvalidCredentials = errors.New("INVALID_CREDENTIALS")
	ErrAccountLocked      = errors.New("ACCOUNT_LOCKED")
	ErrEmailNotVerified   = errors.New("EMAIL_NOT_VERIFIED")
)

// Registration errors
var (
	ErrEmailAlreadyRegistered = errors.New("EMAIL_ALREADY_REGISTERED")
	ErrPasswordTooShort       = errors.New("PASSWORD_TOO_SHORT")
	ErrPasswordBreached       = errors.New("PASSWORD_BREACHED")
)

// OTP errors
var (
	ErrInvalidOTP     = errors.New("INVALID_OTP")
	ErrOTPExpired     = errors.New("OTP_EXPIRED")
	ErrOTPMaxAttempts = errors.New("OTP_MAX_ATTEMPTS")
	ErrRateLimited    = errors.New("RATE_LIMITED")
)
