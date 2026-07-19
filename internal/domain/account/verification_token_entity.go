package account

import (
	"time"
)

// TokenType represents the purpose of a verification token.
type TokenType string

// The two purposes a VerificationToken (OTP) can be issued for.
const (
	TokenTypeEmailVerification TokenType = "email_verification" // #nosec G101 -- enum value, not a credential
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
