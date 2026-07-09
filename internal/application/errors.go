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

// Token errors (refresh / reset tokens)
var (
	// ErrInvalidToken covers malformed, expired, revoked, and already-consumed
	// refresh/reset tokens. Deliberately one generic error so responses don't
	// leak which specific check failed.
	ErrInvalidToken = errors.New("INVALID_TOKEN")

	// ErrTokenVersionMismatch means the token is cryptographically valid but its
	// token_version claim no longer matches USER.token_version (revoked session).
	ErrTokenVersionMismatch = errors.New("TOKEN_VERSION_MISMATCH")
)
