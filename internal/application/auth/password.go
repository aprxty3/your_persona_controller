package auth

import (
	"context"
	"fmt"

	"github.com/aprxty3/your_persona_controller.git/internal/application"
	"golang.org/x/crypto/bcrypt"
)

// PasswordMinLength is the NIST-aligned minimum password length (FR-H1a).
const PasswordMinLength = 10

// ValidateNewPassword enforces the single shared password policy (FR-H1a) for
// every flow that accepts a new password (register, reset-password): required,
// minimum length, and HIBP breach check.
//
// The breach check fails open: if the HIBP backend errors, the password is
// accepted so signups/resets are never blocked by a third-party outage.
func ValidateNewPassword(ctx context.Context, checker PasswordBreachChecker, fieldName, password string) error {
	if err := application.ValidateRequired(fieldName, password); err != nil {
		return err
	}
	if err := application.ValidateMinLength(fieldName, password, PasswordMinLength); err != nil {
		return application.ErrPasswordTooShort
	}
	if breached, err := checker.IsBreached(ctx, password); err == nil && breached {
		return application.ErrPasswordBreached
	}
	return nil
}

// HashPassword produces the bcrypt hash used everywhere a password is persisted.
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("password: bcrypt hash: %w", err)
	}
	return string(hash), nil
}
