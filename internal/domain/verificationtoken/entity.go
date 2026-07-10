package verificationtoken

import (
	"time"
)

// TokenType represents the purpose of a verification token.
type TokenType string

const (
	TokenTypeEmailVerification TokenType = "email_verification"
	TokenTypePasswordReset     TokenType = "password_reset"
)

// VerificationToken represents a short-lived OTP code used for email verification
// or password reset. The token field holds a numeric OTP (e.g. 6 digits).
type VerificationToken struct {
	ID           string
	UserID       string
	Token        string // 6-digit numeric OTP
	Type         TokenType
	AttemptCount int
	ExpiresAt    time.Time
	UsedAt       *time.Time
	CreatedAt    time.Time
}
