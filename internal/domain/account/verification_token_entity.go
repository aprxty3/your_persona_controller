package account

import (
	"time"
)

// TokenType represents the purpose of a verification token.
type TokenType string

const (
	TokenTypeEmailVerification TokenType = "email_verification"
	TokenTypePasswordReset     TokenType = "password_reset"
)

// VerificationToken represents a short-lived OTP code used for email verification or password reset.
type VerificationToken struct {
	ID           string
	UserID       string
	Token        string
	Type         TokenType
	AttemptCount int
	ExpiresAt    time.Time
	UsedAt       *time.Time
	CreatedAt    time.Time
}
