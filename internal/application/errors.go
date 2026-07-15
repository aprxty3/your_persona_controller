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

// Password change errors
var (
	ErrPasswordConfirmationMismatch = errors.New("PASSWORD_CONFIRMATION_MISMATCH")
)

// Account deletion errors
var (
	ErrDeletionAlreadyRequested  = errors.New("DELETION_ALREADY_REQUESTED")
	ErrNoActiveDeletionRequest   = errors.New("NO_ACTIVE_DELETION_REQUEST")
	ErrDeletionAlreadyProcessing = errors.New("DELETION_ALREADY_PROCESSING")
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
	ErrInvalidToken         = errors.New("INVALID_TOKEN")
	ErrTokenVersionMismatch = errors.New("TOKEN_VERSION_MISMATCH")
)

// Assessment submission errors
var (
	ErrLockNotAcquired      = errors.New("LOCK_NOT_ACQUIRED")
	ErrIdempotencyKeyReused = errors.New("IDEMPOTENCY_KEY_REUSED")
	ErrQuotaExceeded        = errors.New("QUOTA_EXCEEDED")
)

// Result / PDF access errors
var (
	ErrResultNotFound = errors.New("RESULT_NOT_FOUND")
	ErrForbidden      = errors.New("FORBIDDEN")
	ErrPDFNotReady    = errors.New("PDF_NOT_READY")
)

// Bot protection errors
var (
	ErrTurnstileFailed = errors.New("TURNSTILE_VERIFICATION_FAILED")
)
